package core

import (
	"log/slog"

	"github.com/immerle/immerle/internal/integrations/discover"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/services/ondemand"
	"github.com/immerle/immerle/internal/services/sonic"
)

// Services are the application services shared by the Subsonic and native
// APIs. They are built once so both protocols run on the same instances.
type Services struct {
	Library    *LibraryService
	Playback   *PlaybackService
	Playlists  *PlaylistService
	PlayQueue  *PlayQueueService
	Users      *UserService
	Shares     *ShareService
	HallOfFame *HallOfFameService
	Radio      *RadioService
}

// ServiceOptions are the non-repository collaborators of the shared services.
// Every field but Auth is optional.
type ServiceOptions struct {
	Auth          *AuthService
	OnDemand      *ondemand.CatalogService
	Activity      *ActivityService
	NowPlaying    *NowPlayingTracker
	ScrobbleSync  ScrobbleEnqueuer
	PlaylistSync  PlaylistSyncEnqueuer
	Sonic         *sonic.Service
	Discover      *discover.Client
	UserForgetter UserForgetter
	Logger        *slog.Logger
}

// NewServices builds the shared services over the store's repositories.
func NewServices(store *persistence.Store, o ServiceOptions) *Services {
	return &Services{
		Library:    NewLibraryService(store.Catalog, store.Annotations, store.Playlists, o.OnDemand).WithSonic(o.Sonic).WithDiscover(o.Discover),
		Playback:   NewPlaybackService(store.Catalog, store.Annotations, store.Scrobbles, o.OnDemand, o.Activity, o.NowPlaying, o.ScrobbleSync),
		Playlists:  NewPlaylistService(store.Playlists, store.Annotations, o.Activity, o.PlaylistSync, o.OnDemand),
		PlayQueue:  NewPlayQueueService(store.PlayQueues, store.Catalog, store.Annotations, o.Logger),
		Users:      NewUserService(store.Users, o.Auth).WithForgetter(o.UserForgetter),
		Shares:     NewShareService(store.Shares, store.Catalog, store.Playlists),
		HallOfFame: NewHallOfFameService(store.HallOfFame, o.OnDemand),
		Radio:      NewRadioService(store.Radio),
	}
}
