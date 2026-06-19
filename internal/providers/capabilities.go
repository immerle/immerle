package providers

import "context"

// ProtocolVersion is the capabilities protocol HTTP providers must implement.
// A remote's /capabilities must advertise this exact version.
const ProtocolVersion = 1

// Capabilities is the contract a remote HTTP provider advertises at its
// mandatory /capabilities endpoint. It states the protocol version it speaks,
// its slug name (which becomes the provider's name in immerle), and the config
// fields it understands — keyed by field name.
type Capabilities struct {
	Version int                    `json:"version"`
	Name    string                 `json:"name"`
	Config  map[string]ConfigField `json:"config"`
}

// ConfigField declares one config value the remote accepts: its value type
// (free-form, e.g. "string"), where it travels (a request header or query
// param), and whether it must be supplied for the provider to work.
type ConfigField struct {
	Type     string `json:"type"`
	Where    string `json:"where"` // "headers" | "params"
	Required bool   `json:"required"`
}

// CapabilityProvider is an optional capability: a provider that can fetch its
// remote's advertised Capabilities (used by the admin to generate the config
// skeleton and display the live protocol version).
type CapabilityProvider interface {
	Capabilities(ctx context.Context) (Capabilities, error)
}
