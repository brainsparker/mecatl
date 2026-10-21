package app

import (
	"context"
	"sync"
	"testing"

	"github.com/stacklok/mecatl/engine/session"
	"github.com/stacklok/mecatl/internal/adapter/mcpauthority"
	"github.com/stacklok/mecatl/internal/adapter/mcpbroker"
	"github.com/stacklok/mecatl/internal/adapter/permconfig"
	"github.com/stacklok/mecatl/internal/identitybroker"
	"github.com/stacklok/mecatl/internal/identitybroker/standin"
)

// closableCore is a stand-in core that records the callers it sees and
// whether it was closed.
type closableCore struct {
	*standin.Broker
	mu      sync.Mutex
	callers []identitybroker.Caller
	closed  int
}

func (c *closableCore) FindSession(ctx context.Context, caller identitybroker.Caller, req identitybroker.FindSessionRequest) (identitybroker.Session, bool, error) {
	c.mu.Lock()
	c.callers = append(c.callers, caller)
	c.mu.Unlock()
	return c.Broker.FindSession(ctx, caller, req)
}

func (c *closableCore) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed++
	return nil
}

func echoBrokerCaller(context.Context, mcpbroker.SessionRef, string, session.ToolCall) (session.ToolResult, error) {
	return session.NewToolResult("call", "ok"), nil
}

// identityBrokerConfigs returns one configuration per MCP broker construction
// path, and whether it should select the bundled ToolHive path.
func identityBrokerConfigs(t *testing.T) map[string]struct {
	cfg     Config
	bundled bool
} {
	base := func() Config { return Config{Workspace: t.TempDir(), UseMock: true, NoSoul: true} }
	bundled := base()
	bundled.MCPAuthority = mcpauthority.NewBroker(mcpauthority.BrokerConfig{CallbackURL: "https://broker.example/oauth/callback", Routes: []permconfig.MCPServerProfile{protectedToolHiveRoute("calendar")}})
	configured := base()
	configured.MCPAuthority = mcpauthority.NewBroker(mcpauthority.BrokerConfig{})
	configured.MCPBrokerCaller = echoBrokerCaller
	return map[string]struct {
		cfg     Config
		bundled bool
	}{
		"bundled ToolHive path": {bundled, true},
		"configured path":       {configured, false},
	}
}

func TestIdentityBrokerIsOffByDefault(t *testing.T) {
	for name, c := range identityBrokerConfigs(t) {
		t.Run(name, func(t *testing.T) {
			built, err := buildIsolated(t, t.Context(), c.cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(built.Close)
			if built.Service.IdentityBroker() != nil {
				t.Fatal("the session code received an identity broker with the flag off")
			}
			if built.MCPBroker == nil {
				t.Fatal("the MCP broker was not built")
			}
			if built.MCPBroker.IdentityBroker() != nil {
				t.Fatal("the MCP broker received an identity broker with the flag off")
			}
		})
	}
}

func TestIdentityBrokerReachesSessionCodeAndBothMCPBrokerPaths(t *testing.T) {
	for name, c := range identityBrokerConfigs(t) {
		t.Run(name, func(t *testing.T) {
			core := &closableCore{Broker: standin.New()}
			cfg := c.cfg
			cfg.IdentityBroker = true
			cfg.identityBrokerCore = core
			built, err := buildIsolated(t, t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(built.Close)

			// Enabling the identity broker must not change which MCP broker path
			// is selected; only the bundled ToolHive path mounts handlers.
			if got := !built.MCPBrokerHandlers.Empty(); got != c.bundled {
				t.Fatalf("bundled ToolHive path selected = %v, want %v", got, c.bundled)
			}

			fromSessions := built.Service.IdentityBroker()
			if fromSessions == nil {
				t.Fatal("the session code received no identity broker")
			}
			if fromMCP := built.MCPBroker.IdentityBroker(); fromMCP != fromSessions {
				t.Fatal("the MCP broker and the session code received different identity brokers")
			}
			// Service and Core cannot share an implementation, so Mecatl cannot hold
			// the core; calls go through the boundary, which supplies the caller.
			if _, _, err := fromSessions.FindSession(t.Context(), identitybroker.FindSessionRequest{RootHostSessionID: "hostS42"}); err != nil {
				t.Fatalf("FindSession: %v", err)
			}
			want := identitybroker.Caller{Workload: identityBrokerWorkload}
			core.mu.Lock()
			defer core.mu.Unlock()
			if len(core.callers) != 1 || core.callers[0] != want || core.callers[0].Verified {
				t.Fatalf("core callers = %+v, want one unverified %+v", core.callers, want)
			}
		})
	}
}

func TestIdentityBrokerIsClosedOnShutdown(t *testing.T) {
	core := &closableCore{Broker: standin.New()}
	built, err := buildIsolated(t, t.Context(), Config{Workspace: t.TempDir(), UseMock: true, NoSoul: true, IdentityBroker: true, identityBrokerCore: core})
	if err != nil {
		t.Fatal(err)
	}
	if built.Service.IdentityBroker() == nil {
		t.Fatal("the session code received no identity broker")
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
