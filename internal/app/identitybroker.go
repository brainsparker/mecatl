package app

import (
	"io"

	"github.com/stacklok/mecatl/internal/identitybroker"
	"github.com/stacklok/mecatl/internal/identitybroker/standin"
)

// identityBrokerWorkload is the caller the identity broker records until it
// authenticates the workload's credential, which will be addressed later. The
// name, and Caller.Verified being false, keep it from reading as a verified
// identity.
const identityBrokerWorkload = "mecak8s-unverified"

// buildIdentityBroker returns the identity broker Mecatl calls, and a function
// that releases it. Both are no-ops when the broker is not enabled. Mecatl gets
// only the authentication boundary, never the core behind it.
func buildIdentityBroker(cfg Config) (identitybroker.Service, func()) {
	if !cfg.IdentityBroker {
		return nil, func() {}
	}
	var core identitybroker.Core = standin.New()
	if cfg.identityBrokerCore != nil {
		core = cfg.identityBrokerCore
	}
	release := func() {}
	if closer, ok := core.(io.Closer); ok {
		release = func() { _ = closer.Close() }
	}
	return identitybroker.NewInProcess(identitybroker.UnverifiedAuthenticator{Workload: identityBrokerWorkload}, core), release
}
