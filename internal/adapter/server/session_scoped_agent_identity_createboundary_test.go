package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stacklok/mecatl/engine/adapter/memstore"
	"github.com/stacklok/mecatl/engine/adapter/mockllm"
	"github.com/stacklok/mecatl/engine/adapter/permpolicy"
	"github.com/stacklok/mecatl/engine/agent"
	"github.com/stacklok/mecatl/engine/governance"
	"github.com/stacklok/mecatl/engine/session"
	"github.com/stacklok/mecatl/engine/tool"
	"github.com/stacklok/mecatl/internal/adapter/mcp"
)

// boundAuthorityWithCeiling is the minimal Authority an agent-bound create must
// persist: a capability set plus a Ceiling equal to it, as the real factory
// (buildAgentDefRootEngine) mints.
func boundAuthorityWithCeiling() session.Authority {
	ceiling := governance.CapabilitySet{Tools: []string{"Read"}}
	return session.Authority{
		CapabilitySet: governance.CapabilitySet{Tools: []string{"Read"}},
		Provenance:    "agent-def:test",
		Ceiling:       &ceiling,
	}
}

func agentBoundTestService(t *testing.T, store *memstore.Store, factory AgentDefSessionEngineFactory) *Service {
	t.Helper()
	svc, err := NewService(Config{
		Engine:                brokerEngineResult().Engine,
		Store:                 store,
		PlacementProvider:     brokerPlacementProvider{},
		PlacementScope:        "test",
		AgentDefSessionEngine: factory,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(svc.Close)
	return svc
}

func restrictedTestEngine() *agent.Engine {
	return agent.NewEngine(agent.Deps{
		LLM:     mockllm.New(mockllm.TextTurn("restricted")),
		Catalog: tool.NewCatalog(),
		Policy:  permpolicy.NewPolicy(nil, nil),
	})
}

// TestSessionScopedAgentIdentity_Scenario1_CreateRejectsIncompatibleOptions
// pins AC1.2 and AC1.5: agent_definition_name cannot be combined with a debug
// target, debug MCP servers, or client-supplied MCP servers, and is rejected as
// InvalidArgument before any engine factory runs. An empty name is a no-op.
func TestSessionScopedAgentIdentity_Scenario1_CreateRejectsIncompatibleOptions(t *testing.T) {
	clientMCP := ClientMCPGrant{specs: []mcp.ServerConfig{{Name: "extra"}}}

	t.Run("validateAgentDefCreate", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			opts    createSessionOpts
			wantErr bool
		}{
			{"no agent definition is a no-op even with a debug target", createSessionOpts{debugTargetID: "t"}, false},
			{"agent definition alone is allowed", createSessionOpts{agentDefinitionName: "a"}, false},
			{"debug target (AC1.5)", createSessionOpts{agentDefinitionName: "a", debugTargetID: "t"}, true},
			{"client MCP servers (AC1.2)", createSessionOpts{agentDefinitionName: "a", clientMCP: clientMCP.specs}, true},
			{"debug MCP servers (AC1.2)", createSessionOpts{agentDefinitionName: "a", debugMCPServers: []string{"s"}}, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := validateAgentDefCreate(tc.opts)
				if tc.wantErr != (err != nil) {
					t.Fatalf("validateAgentDefCreate err = %v, wantErr %v", err, tc.wantErr)
				}
				if tc.wantErr && !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("err = %v, want ErrInvalidArgument", err)
				}
			})
		}
	})

	t.Run("CreateSession rejects before the factory runs", func(t *testing.T) {
		var factoryCalls int32
		svc := agentBoundTestService(t, memstore.New(), func(context.Context, ProviderSelector, SessionProfile, session.PermissionMode, session.Limits, string) (SessionEngineResult, error) {
			atomic.AddInt32(&factoryCalls, 1)
			return SessionEngineResult{Engine: restrictedTestEngine(), Authority: boundAuthorityWithCeiling(), Close: func() error { return nil }}, nil
		})
		for name, opt := range map[string]CreateSessionOption{
			"client MCP":   WithClientMCP(clientMCP),
			"debug target": WithDebugTarget("some-target"),
		} {
			_, err := svc.CreateSessionWithProfile(context.Background(), session.ModeDefault, session.Limits{},
				ProviderSelector{}, ProfileDefault, WithAgentDefinitionName("release-reviewer"), opt)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("%s: err = %v, want ErrInvalidArgument", name, err)
			}
		}
		if n := atomic.LoadInt32(&factoryCalls); n != 0 {
			t.Fatalf("factory ran %d times for a rejected create, want 0", n)
		}
	})
}

// TestSessionScopedAgentIdentity_Scenario3_CreatePersistsBoundCeiling pins the
// create-to-persisted path for AC3.1: the Ceiling the factory mints is bound on
// the stored session (a nil Ceiling means unrestricted to the grant paths), and
// a factory result without one fails the create instead of persisting a
// widenable agent-bound session.
func TestSessionScopedAgentIdentity_Scenario3_CreatePersistsBoundCeiling(t *testing.T) {
	t.Run("minted Ceiling is persisted on the bound authority", func(t *testing.T) {
		store := memstore.New()
		svc := agentBoundTestService(t, store, func(context.Context, ProviderSelector, SessionProfile, session.PermissionMode, session.Limits, string) (SessionEngineResult, error) {
			return SessionEngineResult{Engine: restrictedTestEngine(), Authority: boundAuthorityWithCeiling(), Close: func() error { return nil }}, nil
		})
		created, err := svc.CreateSessionWithProfile(context.Background(), session.ModeDefault, session.Limits{},
			ProviderSelector{}, ProfileDefault, WithAgentDefinitionName("release-reviewer"))
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		stored, err := store.Load(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		authority, bound := stored.BoundAuthority()
		if !bound || authority.Ceiling == nil {
			t.Fatalf("bound=%v Ceiling=%v, want a bound authority with a non-nil Ceiling", bound, authority.Ceiling)
		}
		if got := authority.Ceiling.Tools; len(got) != 1 || got[0] != "Read" {
			t.Fatalf("Ceiling.Tools = %v, want [Read]", got)
		}
	})

	t.Run("a factory result without a Ceiling fails the create", func(t *testing.T) {
		svc := agentBoundTestService(t, memstore.New(), func(context.Context, ProviderSelector, SessionProfile, session.PermissionMode, session.Limits, string) (SessionEngineResult, error) {
			noCeiling := boundAuthorityWithCeiling()
			noCeiling.Ceiling = nil
			return SessionEngineResult{Engine: restrictedTestEngine(), Authority: noCeiling, Close: func() error { return nil }}, nil
		})
		if _, err := svc.CreateSessionWithProfile(context.Background(), session.ModeDefault, session.Limits{},
			ProviderSelector{}, ProfileDefault, WithAgentDefinitionName("release-reviewer")); err == nil {
			t.Fatal("an agent-bound create whose factory minted no Ceiling succeeded; it would be widenable")
		}
	})
}

// TestSessionScopedAgentIdentity_Scenario2_FactoryReceivesPlacementRoot pins the
// plumbing AC2.5 depends on: both factory call sites (create, and the Fork
// successor rebuild) attach the session's own governance root to the context.
// Without it placement-scoped project memory silently turns off.
func TestSessionScopedAgentIdentity_Scenario2_FactoryReceivesPlacementRoot(t *testing.T) {
	var roots []string
	store := memstore.New()
	svc := agentBoundTestService(t, store, func(ctx context.Context, _ ProviderSelector, _ SessionProfile, _ session.PermissionMode, _ session.Limits, _ string) (SessionEngineResult, error) {
		roots = append(roots, AgentDefSessionRootFromContext(ctx))
		return SessionEngineResult{Engine: restrictedTestEngine(), Authority: boundAuthorityWithCeiling(), Close: func() error { return nil }}, nil
	})

	source, err := svc.CreateSessionWithProfile(context.Background(), session.ModeDefault, session.Limits{},
		ProviderSelector{}, ProfileDefault, WithAgentDefinitionName("release-reviewer"))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	stored, err := store.Load(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	want, err := svc.privateGovernanceRoot(context.Background(), stored)
	if err != nil || want == "" {
		t.Fatalf("privateGovernanceRoot = %q, %v; the fixture must bind a non-empty governance root", want, err)
	}

	if _, err := svc.ForkSessionSuccessor(context.Background(), ForkSuccessorRequest{Source: source.ID}); err != nil {
		t.Fatalf("fork successor: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("factory called %d times, want 2 (create, fork)", len(roots))
	}
	for i, site := range []string{"create", "fork successor"} {
		if roots[i] != want {
			t.Fatalf("%s: factory ctx root = %q, want the session's governance root %q", site, roots[i], want)
		}
	}
}
