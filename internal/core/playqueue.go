package core

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/services/ondemand"
)

// PlayQueueEvent is broadcast to a user's connected clients whenever their
// play queue changes — a save (new track/position/playing state) or a
// target change (see SetTarget). This is the real-time channel behind
// cross-device sync and remote control (api's GET /play-queue/events).
type PlayQueueEvent struct {
	Queue PlayQueueResult
	At    time.Time
}

// playQueueHub fans out play-queue events to a user's connected clients
// (SSE), mirroring jamHub's per-key pub/sub but keyed by user id.
type playQueueHub struct {
	mu   sync.Mutex
	subs map[string]map[chan PlayQueueEvent]struct{}
}

func newPlayQueueHub() *playQueueHub {
	return &playQueueHub{subs: map[string]map[chan PlayQueueEvent]struct{}{}}
}

func (h *playQueueHub) subscribe(userID string) chan PlayQueueEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan PlayQueueEvent, 16)
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan PlayQueueEvent]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	return ch
}

func (h *playQueueHub) unsubscribe(userID string, ch chan PlayQueueEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[userID]; ok {
		delete(set, ch)
		close(ch)
		if len(set) == 0 {
			delete(h.subs, userID)
		}
	}
}

// broadcast sends ev to every subscriber and returns how many there were, so
// the caller can log/notice a save that had nobody listening for it.
func (h *playQueueHub) broadcast(userID string, ev PlayQueueEvent) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for ch := range h.subs[userID] {
		n++
		select {
		case ch <- ev:
		default: // drop for slow consumers; they get the next snapshot
		}
	}
	return n
}

// ErrNotActiveDevice is returned by SaveFromDevice when another device holds
// the active-playback role: a late write from a device that was just taken
// over from must not overwrite (and re-broadcast) the new device's state.
var ErrNotActiveDevice = errors.New("not the active playback device")

// presenceTTL is how long a device that isn't holding an SSE stream (a poller)
// still counts as online after its last poll.
const presenceTTL = 15 * time.Second

// devicePresence tracks one device's live connections to the play-queue channel.
type devicePresence struct {
	conns    int
	lastSeen time.Time
}

// PlayQueueService holds the saved play-queue sync logic shared by every
// presentation layer: persisting a user's queue and resolving it back into its
// tracks.
type PlayQueueService struct {
	playQueues  *persistence.PlayQueueRepo
	catalog     *persistence.CatalogRepo
	annotations *persistence.AnnotationRepo
	hub         *playQueueHub
	logger      *slog.Logger

	// ponytail: one global lock serializing every queue write, per-user locks
	// if write throughput ever matters.
	mu sync.Mutex

	presenceMu sync.Mutex
	presence   map[string]map[string]*devicePresence // userID -> deviceID
}

// NewPlayQueueService wires the play-queue application service. logger may
// be nil (tests), in which case notify() logging is skipped.
func NewPlayQueueService(playQueues *persistence.PlayQueueRepo, catalog *persistence.CatalogRepo, annotations *persistence.AnnotationRepo, logger *slog.Logger) *PlayQueueService {
	return &PlayQueueService{playQueues: playQueues, catalog: catalog, annotations: annotations, hub: newPlayQueueHub(), logger: logger, presence: map[string]map[string]*devicePresence{}}
}

// PlayQueueResult is a saved queue resolved into its tracks.
type PlayQueueResult struct {
	Queue   models.PlayQueue
	Entries []TrackEntry
	// TargetOnline reports whether Queue.TargetDeviceID currently has a live
	// connection (see Subscribe/Touch). A spectator uses it to tell "the
	// active device is playing elsewhere" from "the active device is gone",
	// in which case pressing play resumes here instead of sending a command
	// nobody will ever apply.
	TargetOnline bool
}

// Get returns the user's saved queue with its tracks. Returns
// persistence.ErrNotFound when no queue has been saved.
func (s *PlayQueueService) Get(ctx context.Context, userID string) (PlayQueueResult, error) {
	q, err := s.playQueues.Get(ctx, userID)
	if err != nil {
		return PlayQueueResult{}, err
	}
	q, tracks, err := s.resolveQueue(ctx, q)
	if err != nil {
		return PlayQueueResult{}, err
	}
	trackAnn, _ := s.annotations.AnnotationMap(ctx, userID, models.ItemTrack)
	entries := make([]TrackEntry, 0, len(tracks))
	for _, track := range tracks {
		entries = append(entries, TrackEntry{Track: track, Annotation: annPtr(trackAnn, track.ID)})
	}
	return PlayQueueResult{Queue: q, Entries: entries, TargetOnline: s.online(userID, q.TargetDeviceID)}, nil
}

// resolveQueue batch-resolves catalog tracks, removes stale local IDs, and
// retains remote IDs even when the saving client supplied no display metadata.
// A local track missing from the catalog can never be streamed; restoring it
// would leave the player apparently active on an unplayable track.
func (s *PlayQueueService) resolveQueue(ctx context.Context, q models.PlayQueue) (models.PlayQueue, []models.Track, error) {
	entryMeta := make(map[string]models.QueueEntry, len(q.Entries))
	for _, entry := range q.Entries {
		entryMeta[entry.ID] = entry
	}
	catalogTracks, err := s.catalog.TracksByIDs(ctx, q.TrackIDs)
	if err != nil {
		return q, nil, err
	}

	currentIndex := -1
	for i, id := range q.TrackIDs {
		if id == q.Current {
			currentIndex = i
			break
		}
	}

	trackIDs := make([]string, 0, len(q.TrackIDs))
	entries := make([]models.QueueEntry, 0, len(q.TrackIDs))
	tracks := make([]models.Track, 0, len(q.TrackIDs))
	originalIndexes := make([]int, 0, len(q.TrackIDs))
	for i, id := range q.TrackIDs {
		track, found := catalogTracks[id]
		if found {
			trackIDs = append(trackIDs, id)
			tracks = append(tracks, track)
			originalIndexes = append(originalIndexes, i)
			entries = append(entries, models.QueueEntry{
				ID: id, Title: track.Title, Artist: track.ArtistName, Album: track.AlbumName,
				CoverArt: track.CoverArt, Duration: track.Duration, Remote: track.Remote,
			})
			continue
		}

		// Retain remote IDs even when a Subsonic client supplied no display
		// metadata. The ID can still be mirrored/saved; only add a display track
		// when metadata exists. Missing local IDs must be dropped because their
		// streams cannot be resolved.
		meta, hasMeta := entryMeta[id]
		if !ondemand.IsRemoteID(id) && (!hasMeta || !meta.Remote) {
			continue
		}
		trackIDs = append(trackIDs, id)
		originalIndexes = append(originalIndexes, i)
		if !hasMeta {
			continue
		}
		track = models.Track{
			ID: id, Title: meta.Title, ArtistName: meta.Artist, AlbumName: meta.Album,
			CoverArt: meta.CoverArt, Duration: meta.Duration, Remote: true,
		}
		entries = append(entries, models.QueueEntry{
			ID: id, Title: meta.Title, Artist: meta.Artist, Album: meta.Album,
			CoverArt: meta.CoverArt, Duration: meta.Duration, Remote: true,
		})
		tracks = append(tracks, track)
	}

	q.TrackIDs = trackIDs
	q.Entries = entries
	if q.Current != "" && !containsTrackID(trackIDs, q.Current) {
		if len(trackIDs) == 0 {
			q.Current = ""
		} else {
			next := 0
			for i, originalIndex := range originalIndexes {
				if currentIndex < 0 || originalIndex >= currentIndex {
					next = i
					break
				}
			}
			q.Current = trackIDs[next]
		}
		q.PositionMs = 0
		q.Playing = false
	}
	if len(trackIDs) == 0 {
		q.Current = ""
		q.PositionMs = 0
		q.Playing = false
	}
	return q, tracks, nil
}

func containsTrackID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

// Save persists the user's play queue, stamped with the current time, and
// notifies subscribers (see Subscribe). entries is a display-metadata
// snapshot for each of trackIDs (see models.PlayQueue.Entries) — may be nil
// (e.g. from a Subsonic client, which has no rich metadata to offer). shuffle
// and repeat mirror the saving device's transport mode (see
// models.PlayQueue.Shuffle/Repeat) — a Subsonic client passes shuffle=false,
// repeat="" since it has no such concept.
func (s *PlayQueueService) Save(ctx context.Context, userID, current string, positionMs int64, playing bool, changedBy string, trackIDs []string, entries []models.QueueEntry, shuffle bool, repeat string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(ctx, userID, current, positionMs, playing, changedBy, trackIDs, entries, shuffle, repeat)
}

// SaveFromDevice is Save for an app install (changedBy is its device id),
// rejected with ErrNotActiveDevice when another device is the active target.
// The check and the write happen under the same lock as SetTarget, so a
// device's in-flight save can never land after it was taken over from.
func (s *PlayQueueService) SaveFromDevice(ctx context.Context, userID, current string, positionMs int64, playing bool, deviceID string, trackIDs []string, entries []models.QueueEntry, shuffle bool, repeat string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, err := s.playQueues.Get(ctx, userID)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return err
	}
	if q.TargetDeviceID != "" && q.TargetDeviceID != deviceID {
		return ErrNotActiveDevice
	}
	return s.save(ctx, userID, current, positionMs, playing, deviceID, trackIDs, entries, shuffle, repeat)
}

func (s *PlayQueueService) save(ctx context.Context, userID, current string, positionMs int64, playing bool, changedBy string, trackIDs []string, entries []models.QueueEntry, shuffle bool, repeat string) error {
	q, _, err := s.resolveQueue(ctx, models.PlayQueue{
		UserID:     userID,
		Current:    current,
		PositionMs: positionMs,
		Playing:    playing,
		ChangedBy:  changedBy,
		ChangedAt:  time.Now(),
		TrackIDs:   trackIDs,
		Entries:    entries,
		Shuffle:    shuffle,
		Repeat:     repeat,
	})
	if err != nil {
		return err
	}
	if err := s.playQueues.Save(ctx, q); err != nil {
		return err
	}
	s.notify(ctx, userID)
	return nil
}

// SetTarget assigns (deviceID != "") or clears (deviceID == "") the sole
// device that should be actively playing the user's queue, and notifies
// subscribers (see Subscribe).
func (s *PlayQueueService) SetTarget(ctx context.Context, userID, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.playQueues.SetTarget(ctx, userID, deviceID); err != nil {
		return err
	}
	s.notify(ctx, userID)
	return nil
}

// SendCommand records a spectator's remote-control command (see
// models.CommandEnvelope) for the active device to apply itself, and
// notifies subscribers (see Subscribe) so it's delivered promptly over SSE
// in addition to the next plain poll/Get.
func (s *PlayQueueService) SendCommand(ctx context.Context, userID string, cmd models.CommandEnvelope) error {
	if err := s.playQueues.SetCommand(ctx, userID, cmd); err != nil {
		return err
	}
	s.notify(ctx, userID)
	return nil
}

// Subscribe returns an event channel and an unsubscribe func for SSE
// streaming of a user's play-queue changes. deviceID (may be empty, e.g. an
// old client) is marked online for as long as the stream stays open, and
// everyone is notified when it comes and goes so spectators always know
// whether the active device is still there (see PlayQueueResult.TargetOnline).
func (s *PlayQueueService) Subscribe(userID, deviceID string) (<-chan PlayQueueEvent, func()) {
	ch := s.hub.subscribe(userID)
	s.setConnected(userID, deviceID, 1)
	return ch, func() {
		s.hub.unsubscribe(userID, ch)
		s.setConnected(userID, deviceID, -1)
	}
}

// Touch marks deviceID as online for presenceTTL: for clients that poll
// GET /play-queue instead of holding a stream.
func (s *PlayQueueService) Touch(userID, deviceID string) {
	if deviceID == "" {
		return
	}
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()
	s.device(userID, deviceID).lastSeen = time.Now()
}

func (s *PlayQueueService) setConnected(userID, deviceID string, delta int) {
	if deviceID == "" {
		return
	}
	s.presenceMu.Lock()
	d := s.device(userID, deviceID)
	wasOnline := d.conns > 0
	d.conns += delta
	d.lastSeen = time.Now()
	changed := wasOnline != (d.conns > 0)
	s.presenceMu.Unlock()
	if changed {
		// Detached: the request context of a closing stream is already done.
		s.notify(context.Background(), userID)
	}
}

// device returns (creating) the presence entry; callers hold s.presenceMu.
func (s *PlayQueueService) device(userID, deviceID string) *devicePresence {
	if s.presence[userID] == nil {
		s.presence[userID] = map[string]*devicePresence{}
	}
	d := s.presence[userID][deviceID]
	if d == nil {
		d = &devicePresence{}
		s.presence[userID][deviceID] = d
	}
	return d
}

// online reports whether deviceID has an open stream or polled recently.
func (s *PlayQueueService) online(userID, deviceID string) bool {
	if deviceID == "" {
		return false
	}
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()
	d := s.presence[userID][deviceID]
	return d != nil && (d.conns > 0 || time.Since(d.lastSeen) < presenceTTL)
}

func (s *PlayQueueService) notify(ctx context.Context, userID string) {
	res, err := s.Get(ctx, userID)
	if err != nil {
		return
	}
	n := s.hub.broadcast(userID, PlayQueueEvent{Queue: res, At: time.Now()})
	if s.logger != nil {
		s.logger.Info("play-queue notify", "user", userID, "subscribers", n,
			"current", res.Queue.Current, "playing", res.Queue.Playing, "target", res.Queue.TargetDeviceID)
	}
}
