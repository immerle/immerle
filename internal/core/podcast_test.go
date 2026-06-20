package core

import (
	"encoding/xml"
	"testing"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]int{"": 0, "90": 90, "1:30": 90, "01:02:03": 3723, "garbage": 0}
	for in, want := range cases {
		if got := parseDuration(in); got != want {
			t.Errorf("parseDuration(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSuffixFor(t *testing.T) {
	cases := []struct{ url, mime, want string }{
		{"https://x.com/ep1.mp3", "", "mp3"},
		{"https://x.com/ep1.m4a?t=1", "", "m4a"},
		{"https://x.com/stream", "audio/mpeg", "mp3"},
		{"https://x.com/stream", "audio/ogg", "ogg"},
	}
	for _, c := range cases {
		if got := suffixFor(c.url, c.mime); got != c.want {
			t.Errorf("suffixFor(%q,%q) = %q, want %q", c.url, c.mime, got, c.want)
		}
	}
}

func TestFeedDecode(t *testing.T) {
	const feed = `<?xml version="1.0"?>
<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
 <channel>
  <title>My Show</title>
  <description>desc</description>
  <itunes:image href="https://x.com/art.jpg"/>
  <item>
   <title>Ep 1</title>
   <guid>abc-1</guid>
   <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
   <itunes:duration>1:02:03</itunes:duration>
   <enclosure url="https://x.com/ep1.mp3" length="123" type="audio/mpeg"/>
  </item>
 </channel>
</rss>`
	var f rssFeed
	if err := xml.Unmarshal([]byte(feed), &f); err != nil {
		t.Fatal(err)
	}
	if f.Channel.Title != "My Show" || f.image() != "https://x.com/art.jpg" {
		t.Fatalf("channel meta wrong: %+v", f.Channel)
	}
	if len(f.Channel.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(f.Channel.Items))
	}
	it := f.Channel.Items[0]
	if it.GUID != "abc-1" || it.Enclosure.URL != "https://x.com/ep1.mp3" || it.Enclosure.Length != 123 {
		t.Fatalf("item fields wrong: %+v", it)
	}
	if parseDuration(it.Duration) != 3723 {
		t.Fatalf("duration parse wrong: %d", parseDuration(it.Duration))
	}
	if parsePubDate(it.PubDate).IsZero() {
		t.Fatal("pubdate did not parse")
	}
}
