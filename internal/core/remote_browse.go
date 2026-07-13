package core

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/providers"
)

// Remote browse id prefixes. Remote artists/albums surfaced in search are not in
// the local catalog, so they carry self-describing ids (provider + keys) that the
// browsing handlers decode to re-query the provider on demand.
const (
	remoteArtistPrefix = "rart:"
	remoteAlbumPrefix  = "ralb:"  // derived album (provider|artistID|name|albumName)
	remoteAlbumByID    = "ralbp:" // provider-album (provider|providerAlbumID|title)
	idSep              = "\x1f"
)

// IsRemoteArtistID reports whether id is a remote (provider) artist id.
func IsRemoteArtistID(id string) bool { return strings.HasPrefix(id, remoteArtistPrefix) }

// IsRemoteAlbumID reports whether id is a remote (provider) album id (either form).
func IsRemoteAlbumID(id string) bool {
	return strings.HasPrefix(id, remoteAlbumByID) || strings.HasPrefix(id, remoteAlbumPrefix)
}

func encodeRemoteAlbumByID(provider, providerAlbumID, title string) string {
	return remoteAlbumByID + b64(strings.Join([]string{provider, providerAlbumID, title}, idSep))
}

func decodeRemoteAlbumByID(id string) (provider, providerAlbumID, title string, ok bool) {
	raw, err := unb64(strings.TrimPrefix(id, remoteAlbumByID))
	if err != nil {
		return "", "", "", false
	}
	parts := strings.SplitN(raw, idSep, 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func encodeRemoteArtistID(provider, artistID, name string) string {
	return remoteArtistPrefix + b64(strings.Join([]string{provider, artistID, name}, idSep))
}

func decodeRemoteArtistID(id string) (provider, artistID, name string, ok bool) {
	raw, err := unb64(strings.TrimPrefix(id, remoteArtistPrefix))
	if err != nil {
		return "", "", "", false
	}
	parts := strings.SplitN(raw, idSep, 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func encodeRemoteAlbumID(provider, artistID, name, album string) string {
	return remoteAlbumPrefix + b64(strings.Join([]string{provider, artistID, name, album}, idSep))
}

func decodeRemoteAlbumID(id string) (provider, artistID, name, album string, ok bool) {
	raw, err := unb64(strings.TrimPrefix(id, remoteAlbumPrefix))
	if err != nil {
		return "", "", "", "", false
	}
	parts := strings.SplitN(raw, idSep, 4)
	if len(parts) != 4 {
		return "", "", "", "", false
	}
	return parts[0], parts[1], parts[2], parts[3], true
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
func unb64(s string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return string(b), err
}

// toRemoteTrack maps a provider result to a remote (not-yet-local) track.
func toRemoteTrack(provider string, res providers.Result) models.Track {
	return models.Track{
		ID:         encodeRemoteID(provider, res.ProviderTrackID),
		Title:      res.Title,
		ArtistName: res.Artist,
		AlbumName:  res.Album,
		TrackNo:    res.TrackNo,
		DiscNo:     res.DiscNo,
		Year:       res.Year,
		Genre:      res.Genre,
		Duration:   res.Duration,
		MBID:       res.MBID,
		CoverArt:   models.RemoteCoverID(res.CoverImageURL),
		Suffix:     res.Suffix,
		Remote:     true,
		Provider:   provider,
	}
}

// ResolveBestRemoteMatch searches every active provider for the track that
// best matches artist/title and returns it, re-ranked the same way normal
// search results are (see relevance in library.go). Unlike RemoteSearch —
// which intentionally queries a single provider for a single top result (see
// searchProvider) — this is for callers with no specific provider in mind,
// e.g. resolving a hub-federated playlist entry that could be catalogued
// under any of them, and for whom a single provider's #1 guess isn't good
// enough to declare the track unresolvable.
//
// A candidate whose title shares nothing with the wanted title is rejected
// outright (ok=false), even if it's the only/best-ranked result a provider's
// free-text search returned: providers like Internet Archive or Free Music
// Archive always return *something* for a query, and silently playing an
// unrelated track is worse than reporting the entry unresolvable. Artist is
// used only to break ties among candidates that already pass the title gate,
// since hub/provider artist metadata (aliases, "feat." credits, ...) is less
// reliable than title.
func (s *CatalogService) ResolveBestRemoteMatch(ctx context.Context, artist, title string) (models.Track, bool) {
	if s == nil || s.state == nil {
		return models.Track{}, false
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return models.Track{}, false
	}
	query := strings.TrimSpace(artist + " " + title)
	_, _, tracks := s.RemoteSearch3(ctx, query, 1, 1, 20)

	var best models.Track
	bestScore := -1
	for _, t := range tracks {
		titleScore := titleOverlap(title, t.Title)
		if titleScore >= 3 {
			continue // shares nothing with the wanted title: never an acceptable match
		}
		score := titleScore*10 + relevance(artist, t.ArtistName)
		if bestScore == -1 || score < bestScore {
			best, bestScore = t, score
		}
	}
	return best, bestScore != -1
}

// titleOverlap is relevance checked in both directions and takes the better
// (lower) score: a hub/federated title routinely carries extra qualifiers a
// provider's canonical title doesn't ("Get Lucky (Radio Edit - feat. ...)" vs
// "Get Lucky"), so plain one-directional containment (does the wanted title
// contain the candidate, or the reverse) misses the common case where one is
// simply a longer superstring of the other.
func titleOverlap(wanted, candidate string) int {
	score := relevance(wanted, candidate)
	if r := relevance(candidate, wanted); r < score {
		score = r
	}
	return score
}

// RemoteSearch3 gathers remote artists, albums and tracks for search3/search2.
// It queries every active provider in parallel and merges their results into
// three deduplicated lists (artists by name, albums by artist+name, tracks by
// id), capped per provider by the given limits. Ordering and the final result
// caps (combined with the local results) are applied by the caller.
func (s *CatalogService) RemoteSearch3(ctx context.Context, query string, artistLimit, albumLimit, songLimit int) ([]models.Artist, []models.Album, []models.Track) {
	if s == nil || s.state == nil {
		return nil, nil, nil
	}
	if artistLimit <= 0 {
		artistLimit = 20
	}
	if albumLimit <= 0 {
		albumLimit = 20
	}
	if songLimit <= 0 {
		songLimit = 20
	}
	provs := s.state.registry.All()
	if len(provs) == 0 {
		return nil, nil, nil
	}
	ctx, cancel := s.searchCtx(ctx)
	defer cancel()

	type provResult struct {
		artists []models.Artist
		albums  []models.Album
		tracks  []models.Track
	}
	results := make([]provResult, len(provs))
	var wg sync.WaitGroup
	for i, prov := range provs {
		wg.Add(1)
		go func(i int, prov providers.Provider) {
			defer wg.Done()
			// Within a provider, derive in order: artists, then albums (widest
			// track fetch), then tracks. All three reuse the single cached track
			// search, so this is one network call per provider.
			results[i] = provResult{
				artists: s.remoteArtistsFrom(ctx, prov, query, artistLimit),
				albums:  s.remoteAlbumsFrom(ctx, prov, query, albumLimit),
				tracks:  s.remoteTracksFrom(ctx, prov, query, songLimit),
			}
		}(i, prov)
	}
	wg.Wait()

	// Merge in provider (admin) order; the caller re-sorts by relevance.
	var artists []models.Artist
	var albums []models.Album
	var tracks []models.Track
	seenArtist := map[string]bool{}
	seenAlbum := map[string]bool{}
	seenTrack := map[string]bool{}
	for _, r := range results {
		for _, a := range r.artists {
			k := strings.ToLower(a.Name)
			if k == "" || seenArtist[k] {
				continue
			}
			seenArtist[k] = true
			artists = append(artists, a)
		}
		for _, al := range r.albums {
			k := strings.ToLower(al.ArtistName + idSep + al.Name)
			if seenAlbum[k] {
				continue
			}
			seenAlbum[k] = true
			albums = append(albums, al)
		}
		for _, t := range r.tracks {
			if seenTrack[t.ID] {
				continue
			}
			seenTrack[t.ID] = true
			tracks = append(tracks, t)
		}
	}
	return artists, albums, tracks
}

// remoteAlbumsFrom derives remote albums from a single provider's track results,
// grouped by (artist, album). Each carries a browsable derived remote album id.
// The context/timeout is set by the caller; it reuses the cached track search.
func (s *CatalogService) remoteAlbumsFrom(ctx context.Context, prov providers.Provider, query string, limit int) []models.Album {
	results, err := s.cachedTrackSearch(ctx, prov, query, limit*4)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]models.Album, 0, limit)
	for _, res := range results {
		album := strings.TrimSpace(res.Album)
		artist := strings.TrimSpace(res.Artist)
		if album == "" {
			continue
		}
		key := strings.ToLower(artist + idSep + album)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, models.Album{
			ID:         encodeRemoteAlbumID(prov.Name(), res.ProviderArtistID, artist, album),
			Name:       album,
			ArtistID:   encodeRemoteArtistID(prov.Name(), res.ProviderArtistID, artist),
			ArtistName: artist,
			Year:       res.Year,
			CoverArt:   models.RemoteCoverID(res.CoverImageURL),
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// remoteArtistsFrom searches a single provider for artists, deduplicated against
// the local library (by name). The context/timeout is set by the caller.
func (s *CatalogService) remoteArtistsFrom(ctx context.Context, prov providers.Provider, query string, limit int) []models.Artist {
	st := s.state
	out := make([]models.Artist, 0, limit)
	seen := make(map[string]bool)
	add := func(a models.Artist) bool {
		key := strings.ToLower(a.Name)
		if a.Name == "" || seen[key] {
			return len(out) < limit
		}
		// Skip artists already present locally (by name).
		if _, err := st.catalog.FindArtistByName(ctx, a.Name); err == nil {
			return len(out) < limit
		}
		seen[key] = true
		out = append(out, a)
		return len(out) < limit
	}

	// Preferred: a provider that searches artists directly carries accurate album
	// counts and images.
	if as, ok := prov.(providers.ArtistSearcher); ok {
		if arts, err := s.cachedArtistSearch(ctx, as, prov.Name(), query, limit); err == nil {
			for _, ar := range arts {
				name := strings.TrimSpace(ar.Name)
				if !add(models.Artist{
					ID:         encodeRemoteArtistID(prov.Name(), ar.ProviderArtistID, name),
					Name:       name,
					AlbumCount: ar.AlbumCount,
					CoverArt:   models.RemoteCoverID(ar.ImageURL),
				}) {
					break
				}
			}
			return out
		}
	}

	// Fallback: infer artists from the (cached) track results, counting distinct
	// albums. This reuses the song search's cache entry — no extra network call.
	results, err := s.cachedTrackSearch(ctx, prov, query, limit*4)
	if err != nil {
		return out
	}
	albumsByArtist := map[string]map[string]bool{}
	var order []providers.Result
	for _, res := range results {
		name := strings.TrimSpace(res.Artist)
		if name == "" {
			continue
		}
		if albumsByArtist[name] == nil {
			albumsByArtist[name] = map[string]bool{}
			order = append(order, res)
		}
		if res.Album != "" {
			albumsByArtist[name][res.Album] = true
		}
	}
	for _, res := range order {
		name := strings.TrimSpace(res.Artist)
		if !add(models.Artist{
			ID:         encodeRemoteArtistID(prov.Name(), res.ProviderArtistID, name),
			Name:       name,
			AlbumCount: len(albumsByArtist[name]),
			CoverArt:   models.RemoteCoverID(res.ArtistImageURL),
		}) {
			break
		}
	}
	return out
}

// RemoteArtist resolves a remote artist id into an artist plus its albums
// (grouped from the provider's tracks), each with a browsable remote album id.
func (s *CatalogService) RemoteArtist(ctx context.Context, remoteArtistID string) (models.Artist, []models.Album, error) {
	if s == nil || s.state == nil {
		return models.Artist{}, nil, nil
	}
	provName, artistID, name, ok := decodeRemoteArtistID(remoteArtistID)
	if !ok {
		return models.Artist{}, nil, nil
	}
	prov, ok := s.state.registry.Get(provName)
	if !ok {
		return models.Artist{}, nil, nil
	}

	// Preferred: a provider that lists an artist's discography directly.
	if lister, ok := prov.(providers.ArtistAlbumLister); ok && artistID != "" {
		if pas, err := lister.ArtistAlbums(ctx, artistID, 100); err == nil {
			albums := make([]models.Album, 0, len(pas))
			for _, pa := range pas {
				albums = append(albums, models.Album{
					ID:         encodeRemoteAlbumByID(provName, pa.ProviderAlbumID, pa.Title),
					Name:       pa.Title,
					ArtistID:   remoteArtistID,
					ArtistName: name,
					Year:       pa.Year,
					CoverArt:   models.RemoteCoverID(pa.CoverImageURL),
				})
			}
			artist := models.Artist{ID: remoteArtistID, Name: name, AlbumCount: len(albums)}
			return artist, albums, nil
		}
	}

	// Fallback: group the provider's top tracks into albums.
	tracks, artistImage := s.providerArtistTracks(ctx, provName, artistID, name)
	var order []string
	byAlbum := map[string]int{}
	albumCover := map[string]string{}
	for _, t := range tracks {
		alb := t.AlbumName
		if _, seen := byAlbum[alb]; !seen {
			order = append(order, alb)
			albumCover[alb] = t.CoverArt
		}
		byAlbum[alb]++
	}
	albums := make([]models.Album, 0, len(order))
	for _, alb := range order {
		albums = append(albums, models.Album{
			ID:         encodeRemoteAlbumID(provName, artistID, name, alb),
			Name:       alb,
			ArtistID:   remoteArtistID,
			ArtistName: name,
			SongCount:  byAlbum[alb],
			CoverArt:   albumCover[alb],
		})
	}
	artist := models.Artist{
		ID:         remoteArtistID,
		Name:       name,
		AlbumCount: len(albums),
		CoverArt:   models.RemoteCoverID(artistImage),
	}
	return artist, albums, nil
}

// providerImageLookup is an ArtistImageLookup backed by the on-demand provider
// registry: avatars come from wherever artists do.
type providerImageLookup struct{ svc *CatalogService }

// NewProviderImageLookup builds an ArtistImageLookup over the catalog service.
func NewProviderImageLookup(svc *CatalogService) ArtistImageLookup {
	return providerImageLookup{svc: svc}
}

// Available reports whether the primary provider can supply artist images.
func (p providerImageLookup) Available() bool {
	if p.svc == nil || p.svc.state == nil {
		return false
	}
	prov, ok := p.svc.searchProvider()
	if !ok {
		return false
	}
	_, ok = prov.(providers.ArtistImageSearcher)
	return ok
}

// Lookup resolves an artist's avatar URL via the provider.
func (p providerImageLookup) Lookup(ctx context.Context, name string) (string, error) {
	if p.svc == nil || p.svc.state == nil {
		return "", nil
	}
	prov, ok := p.svc.searchProvider()
	if !ok {
		return "", nil
	}
	is, ok := prov.(providers.ArtistImageSearcher)
	if !ok {
		return "", nil
	}
	ctx, cancel := p.svc.searchCtx(ctx)
	defer cancel()
	return is.ArtistImage(ctx, name)
}

// RemoteAlbumsForArtist returns the provider discography for a (local) artist
// name, as remote albums — used to enrich a local artist page.
func (s *CatalogService) RemoteAlbumsForArtist(ctx context.Context, artistName string) ([]models.Album, error) {
	if s == nil || s.state == nil {
		return nil, nil
	}
	prov, ok := s.searchProvider()
	if !ok {
		return nil, nil
	}
	lister, lok := prov.(providers.ArtistAlbumLister)
	as, aok := prov.(providers.ArtistSearcher)
	if !lok || !aok {
		s.state.logger.Debug("artist enrichment skipped: provider lacks browse capability",
			"provider", prov.Name(), "artistSearcher", aok, "artistAlbumLister", lok)
		return nil, nil
	}
	ctx, cancel := s.searchCtx(ctx)
	defer cancel()

	arts, err := s.cachedArtistSearch(ctx, as, prov.Name(), artistName, 5)
	if err != nil {
		s.state.logger.Warn("artist enrichment: provider artist search failed",
			"provider", prov.Name(), "artist", artistName, "error", err)
		return nil, nil
	}
	artistID := ""
	for _, a := range arts {
		if strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(artistName)) {
			artistID = a.ProviderArtistID
			break
		}
	}
	if artistID == "" {
		// No artist matched by name: log the candidates so a spelling/tag mismatch
		// is diagnosable, but don't guess (a wrong match would graft an unrelated
		// discography onto the page).
		names := make([]string, 0, len(arts))
		for _, a := range arts {
			names = append(names, a.Name)
		}
		s.state.logger.Debug("artist enrichment: no name match",
			"provider", prov.Name(), "artist", artistName, "candidates", names)
		return nil, nil
	}
	pas, err := lister.ArtistAlbums(ctx, artistID, 100)
	if err != nil {
		s.state.logger.Warn("artist enrichment: provider album list failed",
			"provider", prov.Name(), "artistId", artistID, "error", err)
		return nil, nil
	}
	s.state.logger.Debug("artist enrichment: provider albums",
		"provider", prov.Name(), "artist", artistName, "count", len(pas))
	out := make([]models.Album, 0, len(pas))
	for _, pa := range pas {
		out = append(out, models.Album{
			ID:         encodeRemoteAlbumByID(prov.Name(), pa.ProviderAlbumID, pa.Title),
			Name:       pa.Title,
			ArtistName: artistName,
			Year:       pa.Year,
			CoverArt:   models.RemoteCoverID(pa.CoverImageURL),
		})
	}
	return out, nil
}

// RemoteTracksForAlbum returns an album's full tracklist from the provider,
// matched by artist + album name — used to enrich a partially-owned local album
// (one that only has the tracks the user has actually downloaded so far) with
// the remaining tracks, as remote, play-on-demand ones. Prefers the precise
// artist/album-browse capabilities when the provider has them, but always
// falls back to a plain search+filter (remoteTracksForAlbumBySearch) — most
// on-demand HTTP providers only implement the three mandatory endpoints
// (search/resolve/download), and without this fallback such a provider would
// silently enrich nothing, forever, however many times the album is reopened.
func (s *CatalogService) RemoteTracksForAlbum(ctx context.Context, artistName, albumName string) ([]models.Track, error) {
	if s == nil || s.state == nil || strings.TrimSpace(albumName) == "" {
		return nil, nil
	}
	prov, ok := s.searchProvider()
	if !ok {
		return nil, nil
	}
	ctx, cancel := s.searchCtx(ctx)
	defer cancel()

	as, aok := prov.(providers.ArtistSearcher)
	lister, lok := prov.(providers.ArtistAlbumLister)
	browser, bok := prov.(providers.AlbumBrowser)
	if !aok || !lok || !bok {
		s.state.logger.Debug("album enrichment: provider lacks browse capability, falling back to search",
			"provider", prov.Name(), "artistSearcher", aok, "artistAlbumLister", lok, "albumBrowser", bok)
		return s.remoteTracksForAlbumBySearch(ctx, prov, artistName, albumName)
	}

	arts, err := s.cachedArtistSearch(ctx, as, prov.Name(), artistName, 5)
	if err != nil {
		return s.remoteTracksForAlbumBySearch(ctx, prov, artistName, albumName)
	}
	artistID := ""
	for _, a := range arts {
		if strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(artistName)) {
			artistID = a.ProviderArtistID
			break
		}
	}
	if artistID == "" {
		return s.remoteTracksForAlbumBySearch(ctx, prov, artistName, albumName)
	}
	pas, err := lister.ArtistAlbums(ctx, artistID, 100)
	if err != nil {
		return s.remoteTracksForAlbumBySearch(ctx, prov, artistName, albumName)
	}
	albumID := ""
	for _, pa := range pas {
		if strings.EqualFold(strings.TrimSpace(pa.Title), strings.TrimSpace(albumName)) {
			albumID = pa.ProviderAlbumID
			break
		}
	}
	if albumID == "" {
		s.state.logger.Debug("album enrichment: no album-name match, falling back to search",
			"provider", prov.Name(), "artist", artistName, "album", albumName)
		return s.remoteTracksForAlbumBySearch(ctx, prov, artistName, albumName)
	}
	results, err := browser.AlbumTracks(ctx, albumID, 200)
	if err != nil {
		s.state.logger.Warn("album enrichment: provider album tracks failed, falling back to search",
			"provider", prov.Name(), "albumId", albumID, "error", err)
		return s.remoteTracksForAlbumBySearch(ctx, prov, artistName, albumName)
	}
	out := make([]models.Track, 0, len(results))
	for _, res := range results {
		out = append(out, toRemoteTrack(prov.Name(), res))
	}
	return out, nil
}

// remoteTracksForAlbumBySearch is RemoteTracksForAlbum's fallback for a
// provider that only implements the mandatory Search capability: it searches
// by artist+album and keeps results whose album (and, when known, artist)
// name matches exactly — the same exact-match filtering
// providerArtistTracks's fallback already uses. ctx must already carry the
// caller's search timeout (searchCtx).
func (s *CatalogService) remoteTracksForAlbumBySearch(ctx context.Context, prov providers.Provider, artistName, albumName string) ([]models.Track, error) {
	query := strings.TrimSpace(artistName + " " + albumName)
	results, err := s.cachedTrackSearch(ctx, prov, query, 100)
	if err != nil {
		return nil, nil
	}
	out := make([]models.Track, 0, len(results))
	for _, res := range results {
		if !strings.EqualFold(strings.TrimSpace(res.Album), strings.TrimSpace(albumName)) {
			continue
		}
		if artistName != "" && !strings.EqualFold(strings.TrimSpace(res.Artist), strings.TrimSpace(artistName)) {
			continue
		}
		out = append(out, toRemoteTrack(prov.Name(), res))
	}
	return out, nil
}

// RemoteAlbum resolves a remote album id into an album plus its tracks.
func (s *CatalogService) RemoteAlbum(ctx context.Context, remoteAlbumID string) (models.Album, []models.Track, error) {
	if s == nil || s.state == nil {
		return models.Album{}, nil, nil
	}

	// Provider-album form: fetch the album's tracks directly via AlbumBrowser.
	if strings.HasPrefix(remoteAlbumID, remoteAlbumByID) {
		return s.remoteAlbumByID(ctx, remoteAlbumID)
	}

	provName, artistID, name, albName, ok := decodeRemoteAlbumID(remoteAlbumID)
	if !ok {
		return models.Album{}, nil, nil
	}
	all, _ := s.providerArtistTracks(ctx, provName, artistID, name)
	var tracks []models.Track
	duration := 0
	cover := ""
	for _, t := range all {
		if t.AlbumName == albName {
			t.AlbumID = remoteAlbumID
			if cover == "" {
				cover = t.CoverArt
			}
			tracks = append(tracks, t)
			duration += t.Duration
		}
	}
	album := models.Album{
		ID:         remoteAlbumID,
		Name:       albName,
		ArtistID:   encodeRemoteArtistID(provName, artistID, name),
		ArtistName: name,
		SongCount:  len(tracks),
		Duration:   duration,
		CoverArt:   cover,
	}
	return album, tracks, nil
}

// remoteAlbumByID resolves a provider-album-id remote album via AlbumBrowser.
func (s *CatalogService) remoteAlbumByID(ctx context.Context, remoteAlbumID string) (models.Album, []models.Track, error) {
	provName, providerAlbumID, title, ok := decodeRemoteAlbumByID(remoteAlbumID)
	if !ok {
		return models.Album{}, nil, nil
	}
	prov, ok := s.state.registry.Get(provName)
	if !ok {
		return models.Album{}, nil, nil
	}
	browser, ok := prov.(providers.AlbumBrowser)
	if !ok {
		return models.Album{ID: remoteAlbumID, Name: title}, nil, nil
	}
	results, err := browser.AlbumTracks(ctx, providerAlbumID, 200)
	if err != nil {
		return models.Album{}, nil, err
	}
	var tracks []models.Track
	duration, cover, artist := 0, "", ""
	for _, res := range results {
		tr := toRemoteTrack(provName, res)
		tr.AlbumID = remoteAlbumID
		if cover == "" {
			cover = tr.CoverArt
		}
		if artist == "" {
			artist = res.Artist
		}
		duration += tr.Duration
		tracks = append(tracks, tr)
	}
	return models.Album{
		ID:         remoteAlbumID,
		Name:       title,
		ArtistName: artist,
		SongCount:  len(tracks),
		Duration:   duration,
		CoverArt:   cover,
	}, tracks, nil
}

// providerArtistTracks returns a remote artist's tracks (via the provider's
// ArtistBrowser capability when available, otherwise by searching the name) and
// the artist's image URL.
func (s *CatalogService) providerArtistTracks(ctx context.Context, provName, artistID, name string) ([]models.Track, string) {
	prov, ok := s.state.registry.Get(provName)
	if !ok {
		return nil, ""
	}
	var results []providers.Result
	if browser, ok := prov.(providers.ArtistBrowser); ok && artistID != "" {
		if r, err := browser.ArtistTracks(ctx, artistID, 100); err == nil {
			results = r
		}
	}
	if len(results) == 0 {
		if r, err := s.cachedTrackSearch(ctx, prov, name, 50); err == nil {
			for _, res := range r {
				if strings.EqualFold(strings.TrimSpace(res.Artist), name) {
					results = append(results, res)
				}
			}
		}
	}
	artistImage := ""
	out := make([]models.Track, 0, len(results))
	for _, res := range results {
		if artistImage == "" {
			artistImage = res.ArtistImageURL
		}
		out = append(out, toRemoteTrack(provName, res))
	}
	return out, artistImage
}
