// Package importer pulls playlists from external sources (e.g. Spotify) and
// resolves each source track against the on-demand content providers to build a
// immerle playlist. Sources are pluggable: register a Factory for a new source
// and it becomes available without touching the import engine.
package importer

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Track is a single track as described by an import source. Only Title and
// Artist are required; the rest help matching/display.
type Track struct {
	Title  string
	Artist string
	Album  string
	// ISRC is the recording identifier, when the source exposes it.
	ISRC string
	// Duration is the track length in seconds (0 if unknown).
	Duration int
}

// Playlist is a playlist as listed by an import source — distinct from both the
// import job and the immerle playlist that will be created from it.
type Playlist struct {
	Name        string
	Description string
	Tracks      []Track
}

// Source fetches a playlist from an external service. Implementations are
// content-agnostic to the rest of the import engine: they only turn a reference
// (an id or URL) into a Playlist of (title, artist) pairs.
type Source interface {
	// Name is the unique source identifier (slug), e.g. "spotify".
	Name() string
	// FetchPlaylist resolves a source reference (playlist id or URL) into its
	// metadata and track list.
	FetchPlaylist(ctx context.Context, ref string) (Playlist, error)
}

// HubFetcher fetches an external playlist through the immerle hub
// (federation). The hub holds the third-party credentials (e.g. Spotify), so a
// source backed by it needs none of its own.
type HubFetcher interface {
	// Available reports whether the hub is currently usable (configured). Read
	// live so a source's "configured" state reflects hot config changes.
	Available() bool
	FetchPlaylist(ctx context.Context, source, ref string) (Playlist, error)
}

// SourceDeps are the dependencies handed to a source factory: free-form settings
// (from the runtime import config) and the hub fetcher (nil when no hub is
// configured). A source uses whichever it needs.
type SourceDeps struct {
	Settings map[string]string
	Hub      HubFetcher
}

// Factory builds a Source from its dependencies.
type Factory func(deps SourceDeps) (Source, error)

var (
	factoriesMu sync.RWMutex
	factories   = map[string]Factory{}
)

// RegisterFactory registers a source factory under a name. Intended for use in
// package init(). Panics on a duplicate registration (programmer error).
func RegisterFactory(name string, f Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	if _, dup := factories[name]; dup {
		panic("importer: duplicate source factory " + name)
	}
	factories[name] = f
}

// HasFactory reports whether a source with the given name is registered.
func HasFactory(name string) bool {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	_, ok := factories[name]
	return ok
}

// Available returns the registered source names, sorted.
func Available() []string {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Build instantiates a registered source from its dependencies.
func Build(name string, deps SourceDeps) (Source, error) {
	factoriesMu.RLock()
	f, ok := factories[name]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("importer: unknown source %q", name)
	}
	return f(deps)
}
