package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/providers"
)

func init() { RegisterFactory("deezer", newDeezer) }

// deezerSource imports public Deezer playlists via Deezer's open API
// (api.deezer.com needs no auth for public playlists), so unlike spotify it
// works without a configured hub.
type deezerSource struct {
	client *http.Client
	base   string // overridable in tests
}

func newDeezer(SourceDeps) (Source, error) {
	return &deezerSource{client: providers.NewHTTPClient(30 * time.Second), base: "https://api.deezer.com"}, nil
}

func (d *deezerSource) Name() string { return "deezer" }

type deezerTrack struct {
	Title  string `json:"title"`
	Artist struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
	} `json:"album"`
}

type deezerError struct {
	Message string `json:"message"`
}

func (d *deezerSource) FetchPlaylist(ctx context.Context, ref string) (Playlist, error) {
	id := deezerPlaylistID(ref)
	if id == "" {
		return Playlist{}, fmt.Errorf("could not find a Deezer playlist id in %q", ref)
	}

	var head struct {
		Title  string       `json:"title"`
		Error  *deezerError `json:"error"`
		Tracks struct {
			Data []deezerTrack `json:"data"`
			Next string        `json:"next"`
		} `json:"tracks"`
	}
	if err := d.getJSON(ctx, fmt.Sprintf("%s/playlist/%s", d.base, id), &head); err != nil {
		return Playlist{}, err
	}
	if head.Error != nil {
		return Playlist{}, fmt.Errorf("deezer: %s", head.Error.Message)
	}

	pl := Playlist{Name: head.Title}
	add := func(ts []deezerTrack) {
		for _, t := range ts {
			pl.Tracks = append(pl.Tracks, Track{Title: t.Title, Artist: t.Artist.Name, Album: t.Album.Title})
		}
	}
	add(head.Tracks.Data)

	// Deezer caps tracks.data and hands back a full "next" URL; follow it to the end.
	for next := head.Tracks.Next; next != ""; {
		var page struct {
			Data  []deezerTrack `json:"data"`
			Next  string        `json:"next"`
			Error *deezerError  `json:"error"`
		}
		if err := d.getJSON(ctx, next, &page); err != nil {
			return Playlist{}, err
		}
		if page.Error != nil {
			return Playlist{}, fmt.Errorf("deezer: %s", page.Error.Message)
		}
		add(page.Data)
		next = page.Next
	}
	return pl, nil
}

func (d *deezerSource) getJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deezer: %s returned %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, providers.MaxMetadataBytes)).Decode(dst)
}

var deezerDigits = regexp.MustCompile(`\d+`)

// deezerPlaylistID pulls the playlist id from a playlist URL or a bare id.
// ponytail: does not resolve short links (deezer.page.link) — paste the full
// playlist URL; add redirect-following when someone actually needs it.
func deezerPlaylistID(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "playlist/"); i >= 0 {
		return deezerDigits.FindString(ref[i+len("playlist/"):])
	}
	if strings.ContainsAny(ref, "/ ") {
		return "" // a URL/path without a playlist segment — not ours
	}
	return deezerDigits.FindString(ref) // bare id
}
