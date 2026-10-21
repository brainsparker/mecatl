package conformance

import (
	"context"
	"testing"

	"github.com/stacklok/mecatl/internal/identitybroker"
)

// runWorkloads calls the Core directly, so the cases can choose the workload.
func runWorkloads(t *testing.T, factory Factory) {
	ctx := context.Background()

	open := func(t *testing.T, core identitybroker.Core, workload identitybroker.Workload, host string) identitybroker.Session {
		t.Helper()
		s, err := core.OpenSession(ctx, workload, openRequest(host, alice))
		if err != nil {
			t.Fatalf("OpenSession(%s) as %s: %v", host, workload.Name, err)
		}
		return s
	}

	t.Run("a session refuses another workload", func(t *testing.T) {
		core := factory(t)
		s := open(t, core, workloadA, "hostS42")
		_, err := core.OpenSession(ctx, workloadB, openRequest("hostS42", alice))
		wantError(t, "OpenSession by another workload", err, identitybroker.CodePermissionDenied, "")
		_, _, err = core.FindSession(ctx, workloadB, findRequest("hostS42"))
		wantError(t, "FindSession by another workload", err, identitybroker.CodePermissionDenied, "")
		_, err = core.RegisterAgent(ctx, workloadB, registerRequest(s))
		wantError(t, "RegisterAgent by another workload", err, identitybroker.CodePermissionDenied, "")
		_, err = core.AuthorizeCall(ctx, workloadB, authorizeRequest(s.ID))
		wantError(t, "AuthorizeCall by another workload", err, identitybroker.CodePermissionDenied, "")
		err = core.CloseSession(ctx, workloadB, identitybroker.CloseSessionRequest{SessionID: s.ID, Reason: identitybroker.ReasonSessionClosed})
		wantError(t, "CloseSession by another workload", err, identitybroker.CodePermissionDenied, "")
		if got, ok, err := core.FindSession(ctx, workloadA, findRequest("hostS42")); err != nil || !ok || got != s {
			t.Fatalf("after refusals, FindSession by its workload = %+v, %v, %v; want %+v, true, nil", got, ok, err, s)
		}
	})

	t.Run("another workload is refused before a closed session's reason", func(t *testing.T) {
		core := factory(t)
		s := open(t, core, workloadA, "hostS42")
		if err := core.CloseSession(ctx, workloadA, identitybroker.CloseSessionRequest{SessionID: s.ID, Reason: identitybroker.ReasonSessionClosed}); err != nil {
			t.Fatalf("CloseSession: %v", err)
		}
		_, err := core.RegisterAgent(ctx, workloadB, registerRequest(s))
		wantError(t, "RegisterAgent by another workload in a closed session", err, identitybroker.CodePermissionDenied, "")
	})

	t.Run("workloads open their own sessions", func(t *testing.T) {
		core := factory(t)
		a := open(t, core, workloadA, "hostS42")
		b := open(t, core, workloadB, "hostS43")
		if a.ID == b.ID {
			t.Fatalf("two workloads share broker session %q", a.ID)
		}
	})

	t.Run("an unnamed workload is refused before the request is checked", func(t *testing.T) {
		core := factory(t)
		s := open(t, core, workloadA, "hostS42")
		for name, workload := range map[string]identitybroker.Workload{
			"no name":     {},
			"spaced name": {Name: "system:serviceaccount:mecatl: mecak8s"},
		} {
			_, err := core.OpenSession(ctx, workload, identitybroker.OpenSessionRequest{})
			wantError(t, "OpenSession with "+name, err, identitybroker.CodeUnauthenticated, "")
			_, _, err = core.FindSession(ctx, workload, findRequest("hostS42"))
			wantError(t, "FindSession with "+name, err, identitybroker.CodeUnauthenticated, "")
			err = core.CloseSession(ctx, workload, identitybroker.CloseSessionRequest{SessionID: s.ID, Reason: identitybroker.ReasonSessionClosed})
			wantError(t, "CloseSession with "+name, err, identitybroker.CodeUnauthenticated, "")
			_, err = core.RegisterAgent(ctx, workload, registerRequest(s))
			wantError(t, "RegisterAgent with "+name, err, identitybroker.CodeUnauthenticated, "")
			_, err = core.AuthorizeCall(ctx, workload, authorizeRequest(s.ID))
			wantError(t, "AuthorizeCall with "+name, err, identitybroker.CodeUnauthenticated, "")
		}
	})
}
