package mcpbroker

import "github.com/stacklok/mecatl/internal/identitybroker"

// WithIdentityBroker gives the runtime the agent identity broker that MCP
// calls will be authorized through. Nothing calls it yet.
func WithIdentityBroker(broker identitybroker.Service) Option {
	return func(runtime *Runtime) { runtime.identityBroker = broker }
}

// IdentityBroker returns the agent identity broker the runtime was given, or
// nil when none is configured.
func (r *Runtime) IdentityBroker() identitybroker.Service {
	return r.identityBroker
}
