package bandcamp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = orig })
	return srv.URL
}

func TestFanIDParsesCollectionSummary(t *testing.T) {
	var gotCookie string
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"fan_id": 1712700664}`))
	})
	id, err := NewClient().FanID(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if id != "1712700664" {
		t.Fatalf("FanID = %q, want 1712700664", id)
	}
	if gotCookie != "identity=abc123" {
		t.Fatalf("Cookie header = %q, want identity=abc123", gotCookie)
	}
}

func TestFanIDInvalidCookieReturnsSentinel(t *testing.T) {
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := NewClient().FanID(context.Background(), "expired")
	if err != ErrInvalidCookie {
		t.Fatalf("FanID error = %v, want ErrInvalidCookie", err)
	}
}

func TestCollectionParsesItemsAndMatchesRedownloadURLs(t *testing.T) {
	var gotBody map[string]any
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"more_available": true,
			"last_token": "tok-2",
			"items": [{
				"sale_item_type": "p",
				"sale_item_id": 123,
				"item_type": "album",
				"band_name": "Pinkfong",
				"item_title": "Baby Shark",
				"album_title": "Pinkfong Animal Songs",
				"purchased": "01 Jan 2021 10:00:00 GMT",
				"item_art_url": "https://example.com/art.jpg"
			}],
			"redownload_urls": {"p123": "https://bandcamp.com/download?x=1"}
		}`))
	})
	page, err := NewClient().Collection(context.Background(), "cookie", "42", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["fan_id"] != "42" || gotBody["count"].(float64) != 10 {
		t.Fatalf("request body = %+v, unexpected fan_id/count", gotBody)
	}
	if !page.MoreAvailable || page.LastToken != "tok-2" {
		t.Fatalf("page = %+v, want more_available=true, last_token=tok-2", page)
	}
	if len(page.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(page.Items))
	}
	item := page.Items[0]
	if item.SaleItemType != "p" || item.SaleItemID != "123" || item.ArtistName != "Pinkfong" ||
		item.ItemTitle != "Baby Shark" || item.RedownloadURL != "https://bandcamp.com/download?x=1" {
		t.Fatalf("item = %+v, unexpected fields", item)
	}
}

func TestResolveDownloadPicksBestFormatByPriority(t *testing.T) {
	url := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><div id="pagedata" data-blob="{&quot;download_items&quot;:[{&quot;downloads&quot;:{&quot;mp3-320&quot;:{&quot;url&quot;:&quot;https://example.com/mp3&quot;,&quot;size_mb&quot;:8.5},&quot;vorbis&quot;:{&quot;url&quot;:&quot;https://example.com/ogg&quot;,&quot;size_mb&quot;:7}}}]}"></div></body></html>`))
	})
	info, err := NewClient().ResolveDownload(context.Background(), "cookie", url)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "mp3-320" || info.URL != "https://example.com/mp3" {
		t.Fatalf("ResolveDownload = %+v, want mp3-320 to win over vorbis", info)
	}
}

func TestResolveDownloadFallsBackWhenPreferredFormatsAbsent(t *testing.T) {
	url := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><div id="pagedata" data-blob="{&quot;download_items&quot;:[{&quot;downloads&quot;:{&quot;wav&quot;:{&quot;url&quot;:&quot;https://example.com/wav&quot;,&quot;size_mb&quot;:40},&quot;aiff-lossless&quot;:{&quot;url&quot;:&quot;https://example.com/aiff&quot;,&quot;size_mb&quot;:42}}}]}"></div></body></html>`))
	})
	info, err := NewClient().ResolveDownload(context.Background(), "cookie", url)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "wav" {
		t.Fatalf("ResolveDownload = %+v, want wav (higher priority than aiff-lossless)", info)
	}
}

func TestResolveDownloadMissingPagedataReturnsDistinctError(t *testing.T) {
	url := withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>no pagedata here</body></html>`))
	})
	_, err := NewClient().ResolveDownload(context.Background(), "cookie", url)
	if err != ErrPagedataNotFound {
		t.Fatalf("ResolveDownload error = %v, want ErrPagedataNotFound", err)
	}
}
