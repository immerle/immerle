package subsonic

// This file defines the response payloads for the additional Subsonic endpoints
// (file-based browsing, list/search v1, info, lyrics and the stub sections).

// Directory is the getMusicDirectory result (file-based browsing).
type Directory struct {
	ID     string  `xml:"id,attr" json:"id"`
	Name   string  `xml:"name,attr" json:"name"`
	Parent string  `xml:"parent,attr,omitempty" json:"parent,omitempty"`
	Child  []Child `xml:"child" json:"child,omitempty"`
}

// AlbumList is the getAlbumList (non-ID3) result.
type AlbumList struct {
	Album []Child `xml:"album" json:"album,omitempty"`
}

// Songs wraps a flat song list (getSongsByGenre, getRandomSongs, getTopSongs).
type Songs struct {
	Song []Child `xml:"song" json:"song,omitempty"`
}

// Starred is the getStarred (non-ID3) result.
type Starred struct {
	Artist []ArtistItem `xml:"artist" json:"artist,omitempty"`
	Album  []Child      `xml:"album" json:"album,omitempty"`
	Song   []Child      `xml:"song" json:"song,omitempty"`
}

// SearchResult2 is the search2 (non-ID3) result.
type SearchResult2 struct {
	Artist []ArtistItem `xml:"artist" json:"artist,omitempty"`
	Album  []Child      `xml:"album" json:"album,omitempty"`
	Song   []Child      `xml:"song" json:"song,omitempty"`
}

// SimilarSongs is the getSimilarSongs result.
type SimilarSongs struct {
	Song []Child `xml:"song" json:"song,omitempty"`
}

// SimilarSongs2 is the getSimilarSongs2 result.
type SimilarSongs2 struct {
	Song []Child `xml:"song" json:"song,omitempty"`
}

// ArtistInfoBase carries the common artist-info fields.
type ArtistInfoBase struct {
	Biography      string `xml:"biography,omitempty" json:"biography,omitempty"`
	MusicBrainzID  string `xml:"musicBrainzId,omitempty" json:"musicBrainzId,omitempty"`
	LastFmURL      string `xml:"lastFmUrl,omitempty" json:"lastFmUrl,omitempty"`
	SmallImageURL  string `xml:"smallImageUrl,omitempty" json:"smallImageUrl,omitempty"`
	MediumImageURL string `xml:"mediumImageUrl,omitempty" json:"mediumImageUrl,omitempty"`
	LargeImageURL  string `xml:"largeImageUrl,omitempty" json:"largeImageUrl,omitempty"`
}

// ArtistInfo is the getArtistInfo result.
type ArtistInfo struct {
	ArtistInfoBase
	SimilarArtist []ArtistItem `xml:"similarArtist" json:"similarArtist,omitempty"`
}

// ArtistInfo2 is the getArtistInfo2 (ID3) result.
type ArtistInfo2 struct {
	ArtistInfoBase
	SimilarArtist []ArtistID3 `xml:"similarArtist" json:"similarArtist,omitempty"`
}

// AlbumInfo is the getAlbumInfo/getAlbumInfo2 result.
type AlbumInfo struct {
	Notes          string `xml:"notes,omitempty" json:"notes,omitempty"`
	MusicBrainzID  string `xml:"musicBrainzId,omitempty" json:"musicBrainzId,omitempty"`
	SmallImageURL  string `xml:"smallImageUrl,omitempty" json:"smallImageUrl,omitempty"`
	MediumImageURL string `xml:"mediumImageUrl,omitempty" json:"mediumImageUrl,omitempty"`
	LargeImageURL  string `xml:"largeImageUrl,omitempty" json:"largeImageUrl,omitempty"`
}

// Lyrics is the getLyrics result (unsynced).
type Lyrics struct {
	Artist string `xml:"artist,attr,omitempty" json:"artist,omitempty"`
	Title  string `xml:"title,attr,omitempty" json:"title,omitempty"`
	Value  string `xml:",chardata" json:"value,omitempty"`
}

// LyricsList is the OpenSubsonic getLyricsBySongId result.
type LyricsList struct {
	StructuredLyrics []StructuredLyrics `xml:"structuredLyrics" json:"structuredLyrics,omitempty"`
}

// StructuredLyrics is one (possibly synced) lyrics document.
type StructuredLyrics struct {
	Lang   string      `xml:"lang,attr" json:"lang"`
	Synced bool        `xml:"synced,attr" json:"synced"`
	Line   []LyricLine `xml:"line" json:"line,omitempty"`
}

// LyricLine is a single lyric line.
type LyricLine struct {
	Start int64  `xml:"start,attr,omitempty" json:"start,omitempty"`
	Value string `xml:",chardata" json:"value"`
}

// Videos is the getVideos result (empty — no video support).
type Videos struct {
	Video []Child `xml:"video" json:"video,omitempty"`
}

// Bookmarks is the getBookmarks result.
type Bookmarks struct {
	Bookmark []Bookmark `xml:"bookmark" json:"bookmark,omitempty"`
}

// Bookmark is a saved playback position.
type Bookmark struct {
	Position int64  `xml:"position,attr" json:"position"`
	Username string `xml:"username,attr" json:"username"`
	Created  string `xml:"created,attr,omitempty" json:"created,omitempty"`
	Changed  string `xml:"changed,attr,omitempty" json:"changed,omitempty"`
	Entry    *Child `xml:"entry,omitempty" json:"entry,omitempty"`
}

// InternetRadioStations is the getInternetRadioStations result.
type InternetRadioStations struct {
	InternetRadioStation []InternetRadioStation `xml:"internetRadioStation" json:"internetRadioStation,omitempty"`
}

// InternetRadioStation is a single radio station.
type InternetRadioStation struct {
	ID          string `xml:"id,attr" json:"id"`
	Name        string `xml:"name,attr" json:"name"`
	StreamURL   string `xml:"streamUrl,attr" json:"streamUrl"`
	HomePageURL string `xml:"homePageUrl,attr,omitempty" json:"homePageUrl,omitempty"`
}

// ChatMessages is the getChatMessages result.
type ChatMessages struct {
	ChatMessage []ChatMessage `xml:"chatMessage" json:"chatMessage,omitempty"`
}

// ChatMessage is a single chat message.
type ChatMessage struct {
	Username string `xml:"username,attr" json:"username"`
	Time     int64  `xml:"time,attr" json:"time"`
	Message  string `xml:"message,attr" json:"message"`
}
