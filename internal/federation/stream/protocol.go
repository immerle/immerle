// Package stream implements the instance-side client of the federation feed
// socket (RFC-socket-federation-client.md): a live replacement for the hourly
// REST feed pull, with the REST path kept as a fallback.
package stream

import "encoding/json"

// Frame types, instance <-> hub. Kept as a hand-written copy of the hub's
// internal/ws/protocol.go (not in the vendored OpenAPI spec, which doesn't
// describe the socket upgrade's frames).
const (
	TypeWelcome        = "welcome"
	TypeResume         = "resume"
	TypeHeartbeat      = "heartbeat"
	TypeHeartbeatAck   = "heartbeat_ack"
	TypePlaylistUpsert = "playlist.upsert"
	TypePlaylistDelete = "playlist.delete"
	TypeReplayRequest  = "replay.request"
	TypeError          = "error"
)

// Frame is the wire shape of every message on the federation feed socket.
// Fields are opaque to the hub (it only routes by Type/publisher/subscriber);
// meaning is defined between publishing and subscribing instances.
type Frame struct {
	Type string `json:"type"`

	Cursors map[string]string `json:"cursors,omitempty"`

	ExternalID string          `json:"externalId,omitempty"`
	Version    string          `json:"version,omitempty"`
	UpdatedAt  string          `json:"updatedAt,omitempty"`
	Image      string          `json:"image,omitempty"`
	Tracks     json.RawMessage `json:"tracks,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	AuthorID   string          `json:"authorId,omitempty"` // set by the hub on relay
	Target     string          `json:"target,omitempty"`   // unicast reply to a replay.request

	ForSubscriberID string `json:"forSubscriberId,omitempty"`
	SinceVersion    string `json:"sinceVersion,omitempty"`

	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
