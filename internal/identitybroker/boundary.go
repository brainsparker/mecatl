package identitybroker

import (
	"context"
	"errors"
)

// Caller is the workload making a call, as the broker's authenticator
// determined it. It is never a request field: Mecatl presents no identity of
// its own, and the broker decides who is calling.
type Caller struct {
	// Workload identifies the calling service, such as a Kubernetes
	// ServiceAccount.
	Workload string
	// Verified reports that an authenticator verified the workload's
	// credential. The zero value is false, so only an authenticator that
	// verifies a credential sets it, and records can tell an unverified
	// placeholder from a verified identity.
	Verified bool
}

// Validate reports whether c names a workload. The error is an *Error with
// CodeUnauthenticated.
func (c Caller) Validate() error {
	if !isToken(c.Workload) {
		return NewError(CodeUnauthenticated, "", "the caller names no workload")
	}
	return nil
}

// Core is the broker side of the contract: Service's calls, each with the
// caller the authenticator determined. Implementations provide the behavior
// documented on Service, and also:
//
//   - A caller that fails Validate is refused with CodeUnauthenticated, before
//     any other check.
//   - A broker session is bound to the workload that opened it. Opening,
//     finding, closing, registering into, or calling through a session bound
//     to another workload is refused with CodePermissionDenied, before its
//     closing reason is revealed.
type Core interface {
	OpenSession(ctx context.Context, caller Caller, req OpenSessionRequest) (Session, error)
	FindSession(ctx context.Context, caller Caller, req FindSessionRequest) (Session, bool, error)
	CloseSession(ctx context.Context, caller Caller, req CloseSessionRequest) error
	RegisterAgent(ctx context.Context, caller Caller, req RegisterAgentRequest) (Registration, error)
	AuthorizeCall(ctx context.Context, caller Caller, req AuthorizeCallRequest) (Authorization, error)
}

// Authenticator decides who is calling. A failure is an *Error, normally with
// CodeUnauthenticated or CodePermissionDenied.
type Authenticator interface {
	Authenticate(ctx context.Context) (Caller, error)
}

// UnverifiedAuthenticator returns the same unverified caller for every call.
// It stands in until the broker authenticates the workload's credential,
// which will be addressed later.
type UnverifiedAuthenticator struct {
	Workload string
}

// Authenticate implements Authenticator. The caller is never verified.
func (a UnverifiedAuthenticator) Authenticate(context.Context) (Caller, error) {
	return Caller{Workload: a.Workload}, nil
}

// NewInProcess returns the Service Mecatl calls when the broker runs in its
// process. Each call asks authn who is calling, then passes that caller to
// core, so the in-process path takes the same authentication step as a
// remote one.
func NewInProcess(authn Authenticator, core Core) Service {
	return inProcess{authn: authn, core: core}
}

type inProcess struct {
	authn Authenticator
	core  Core
}

func (p inProcess) authenticate(ctx context.Context) (Caller, error) {
	caller, err := p.authn.Authenticate(ctx)
	if err == nil {
		return caller, nil
	}
	var brokerErr *Error
	if errors.As(err, &brokerErr) {
		return Caller{}, err
	}
	return Caller{}, NewError(CodeUnauthenticated, "", "the caller could not be authenticated")
}

func (p inProcess) OpenSession(ctx context.Context, req OpenSessionRequest) (Session, error) {
	caller, err := p.authenticate(ctx)
	if err != nil {
		return Session{}, err
	}
	return p.core.OpenSession(ctx, caller, req)
}

func (p inProcess) FindSession(ctx context.Context, req FindSessionRequest) (Session, bool, error) {
	caller, err := p.authenticate(ctx)
	if err != nil {
		return Session{}, false, err
	}
	return p.core.FindSession(ctx, caller, req)
}

func (p inProcess) CloseSession(ctx context.Context, req CloseSessionRequest) error {
	caller, err := p.authenticate(ctx)
	if err != nil {
		return err
	}
	return p.core.CloseSession(ctx, caller, req)
}

func (p inProcess) RegisterAgent(ctx context.Context, req RegisterAgentRequest) (Registration, error) {
	caller, err := p.authenticate(ctx)
	if err != nil {
		return Registration{}, err
	}
	return p.core.RegisterAgent(ctx, caller, req)
}

func (p inProcess) AuthorizeCall(ctx context.Context, req AuthorizeCallRequest) (Authorization, error) {
	caller, err := p.authenticate(ctx)
	if err != nil {
		return Authorization{}, err
	}
	return p.core.AuthorizeCall(ctx, caller, req)
}
