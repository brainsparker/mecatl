package app

import (
	"context"
	"sync"
	"testing"

	"github.com/stacklok/mecatl/internal/identitybroker"
	"github.com/stacklok/mecatl/internal/identitybroker/standin"
)

// closableCore is a stand-in core that records the workloads it sees and
// whether it was closed.
type closableCore struct {
	*standin.Broker
	mu        sync.Mutex
	workloads []identitybroker.Workload
	closed    int
}

func (c *closableCore) FindSession(ctx context.Context, workload identitybroker.Workload, req identitybroker.FindSessionRequest) (identitybroker.Session, bool, error) {
	c.mu.Lock()
	c.workloads = append(c.workloads, workload)
	c.mu.Unlock()
	return c.Broker.FindSession(ctx, workload, req)
}

func (c *closableCore) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed++
	return nil
}

func TestIdentityBrokerIsOffByDefault(t *testing.T) {
	built, err := buildIsolated(t, t.Context(), Config{Workspace: t.TempDir(), UseMock: true, NoSoul: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(built.Close)
	if built.IdentityBroker != nil {
		t.Fatal("an identity broker was constructed with the flag off")
	}
}

func TestIdentityBrokerCallsCarryTheUnverifiedWorkload(t *testing.T) {
	core := &closableCore{Broker: standin.New()}
	built, err := buildIsolated(t, t.Context(), Config{Workspace: t.TempDir(), UseMock: true, NoSoul: true, IdentityBroker: true, identityBrokerCore: core})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(built.Close)
	if built.IdentityBroker == nil {
		t.Fatal("no identity broker was constructed with the flag on")
	}
	// Service and Core cannot share an implementation, so Mecatl cannot hold
	// the core; calls go through the boundary, which supplies the workload.
	if _, _, err := built.IdentityBroker.FindSession(t.Context(), identitybroker.FindSessionRequest{RootHostSessionID: "hostS42"}); err != nil {
		t.Fatalf("FindSession: %v", err)
	}
	want := identitybroker.Workload{Name: identityBrokerWorkload}
	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.workloads) != 1 || core.workloads[0] != want || core.workloads[0].Verified {
		t.Fatalf("core workloads = %+v, want one unverified %+v", core.workloads, want)
	}
}

func TestIdentityBrokerIsClosedOnShutdown(t *testing.T) {
	core := &closableCore{Broker: standin.New()}
	built, err := buildIsolated(t, t.Context(), Config{Workspace: t.TempDir(), UseMock: true, NoSoul: true, IdentityBroker: true, identityBrokerCore: core})
	if err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	closedBeforeShutdown := core.closed
	core.mu.Unlock()
	if closedBeforeShutdown != 0 {
		t.Fatal("the identity broker was closed before shutdown")
	}
	built.Close()
	built.Close()
	core.mu.Lock()
	defer core.mu.Unlock()
	if core.closed != 1 {
		t.Fatalf("identity broker closed %d times, want once", core.closed)
	}
}
