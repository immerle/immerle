// Package subsonic implements the Subsonic / OpenSubsonic API.
package subsonic

import "encoding/xml"

const (
	// apiVersion is the Subsonic protocol version advertised by the server.
	apiVersion = "1.16.1"
	// serverType identifies this implementation.
	serverType = "immerle"
	// serverVersion is the immerle server version.
	serverVersion = "0.1.0"
)

// Response is the root Subsonic response envelope.
type Response struct {
	XMLName       xml.Name `xml:"http://subsonic.org/restapi subsonic-response" json:"-"`
	Status        string   `xml:"status,attr" json:"status"`
	Version       string   `xml:"version,attr" json:"version"`
	Type          string   `xml:"type,attr" json:"type"`
	ServerVersion string   `xml:"serverVersion,attr" json:"serverVersion"`
	OpenSubsonic  bool     `xml:"openSubsonic,attr" json:"openSubsonic"`

	Error                  *Error                  `xml:"error,omitempty" json:"error,omitempty"`
	License                *License                `xml:"license,omitempty" json:"license,omitempty"`
	MusicFolders           *MusicFolders           `xml:"musicFolders,omitempty" json:"musicFolders,omitempty"`
	Indexes                *Indexes                `xml:"indexes,omitempty" json:"indexes,omitempty"`
	Artists                *ArtistsID3             `xml:"artists,omitempty" json:"artists,omitempty"`
	Artist                 *ArtistID3              `xml:"artist,omitempty" json:"artist,omitempty"`
	Album                  *AlbumID3               `xml:"album,omitempty" json:"album,omitempty"`
	Song                   *Child                  `xml:"song,omitempty" json:"song,omitempty"`
	AlbumList2             *AlbumList2             `xml:"albumList2,omitempty" json:"albumList2,omitempty"`
	Genres                 *Genres                 `xml:"genres,omitempty" json:"genres,omitempty"`
	SearchResult3          *SearchResult3          `xml:"searchResult3,omitempty" json:"searchResult3,omitempty"`
	Playlists              *Playlists              `xml:"playlists,omitempty" json:"playlists,omitempty"`
	Playlist               *Playlist               `xml:"playlist,omitempty" json:"playlist,omitempty"`
	User                   *User                   `xml:"user,omitempty" json:"user,omitempty"`
	Users                  *Users                  `xml:"users,omitempty" json:"users,omitempty"`
	NowPlaying             *NowPlaying             `xml:"nowPlaying,omitempty" json:"nowPlaying,omitempty"`
	PlayQueue              *PlayQueue              `xml:"playQueue,omitempty" json:"playQueue,omitempty"`
	Starred2               *Starred2               `xml:"starred2,omitempty" json:"starred2,omitempty"`
	Shares                 *Shares                 `xml:"shares,omitempty" json:"shares,omitempty"`
	ScanStatus             *ScanStatus             `xml:"scanStatus,omitempty" json:"scanStatus,omitempty"`
	OpenSubsonicExtensions []OpenSubsonicExtension `xml:"openSubsonicExtensions,omitempty" json:"openSubsonicExtensions,omitempty"`

	Directory             *Directory             `xml:"directory,omitempty" json:"directory,omitempty"`
	AlbumList             *AlbumList             `xml:"albumList,omitempty" json:"albumList,omitempty"`
	Starred               *Starred               `xml:"starred,omitempty" json:"starred,omitempty"`
	SearchResult2         *SearchResult2         `xml:"searchResult2,omitempty" json:"searchResult2,omitempty"`
	SongsByGenre          *Songs                 `xml:"songsByGenre,omitempty" json:"songsByGenre,omitempty"`
	RandomSongs           *Songs                 `xml:"randomSongs,omitempty" json:"randomSongs,omitempty"`
	TopSongs              *Songs                 `xml:"topSongs,omitempty" json:"topSongs,omitempty"`
	SimilarSongs          *SimilarSongs          `xml:"similarSongs,omitempty" json:"similarSongs,omitempty"`
	SimilarSongs2         *SimilarSongs2         `xml:"similarSongs2,omitempty" json:"similarSongs2,omitempty"`
	ArtistInfo            *ArtistInfo            `xml:"artistInfo,omitempty" json:"artistInfo,omitempty"`
	ArtistInfo2           *ArtistInfo2           `xml:"artistInfo2,omitempty" json:"artistInfo2,omitempty"`
	AlbumInfo             *AlbumInfo             `xml:"albumInfo,omitempty" json:"albumInfo,omitempty"`
	Lyrics                *Lyrics                `xml:"lyrics,omitempty" json:"lyrics,omitempty"`
	LyricsList            *LyricsList            `xml:"lyricsList,omitempty" json:"lyricsList,omitempty"`
	Videos                *Videos                `xml:"videos,omitempty" json:"videos,omitempty"`
	Bookmarks             *Bookmarks             `xml:"bookmarks,omitempty" json:"bookmarks,omitempty"`
	InternetRadioStations *InternetRadioStations `xml:"internetRadioStations,omitempty" json:"internetRadioStations,omitempty"`
	ChatMessages          *ChatMessages          `xml:"chatMessages,omitempty" json:"chatMessages,omitempty"`
}

// Error is a Subsonic error payload.
type Error struct {
	Code    int    `xml:"code,attr" json:"code"`
	Message string `xml:"message,attr" json:"message"`
}

// License describes the server license (always valid here).
type License struct {
	Valid bool `xml:"valid,attr" json:"valid"`
}

// MusicFolders wraps the list of top-level music folders.
type MusicFolders struct {
	MusicFolder []MusicFolder `xml:"musicFolder" json:"musicFolder"`
}

// MusicFolder is a configured library root.
type MusicFolder struct {
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

// Indexes is the legacy (non-ID3) artist index.
type Indexes struct {
	IgnoredArticles string  `xml:"ignoredArticles,attr" json:"ignoredArticles"`
	LastModified    int64   `xml:"lastModified,attr" json:"lastModified"`
	Index           []Index `xml:"index" json:"index,omitempty"`
}

// Index groups artists by first letter.
type Index struct {
	Name   string       `xml:"name,attr" json:"name"`
	Artist []ArtistItem `xml:"artist" json:"artist,omitempty"`
}

// ArtistItem is a legacy artist reference.
type ArtistItem struct {
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

// ArtistsID3 is the ID3 artist index (getArtists).
type ArtistsID3 struct {
	IgnoredArticles string     `xml:"ignoredArticles,attr" json:"ignoredArticles"`
	Index           []IndexID3 `xml:"index" json:"index,omitempty"`
}

// IndexID3 groups ID3 artists by first letter.
type IndexID3 struct {
	Name   string      `xml:"name,attr" json:"name"`
	Artist []ArtistID3 `xml:"artist" json:"artist,omitempty"`
}

// ArtistID3 is an artist in the ID3 model.
type ArtistID3 struct {
	ID         string     `xml:"id,attr" json:"id"`
	Name       string     `xml:"name,attr" json:"name"`
	CoverArt   string     `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	AlbumCount int        `xml:"albumCount,attr" json:"albumCount"`
	Starred    string     `xml:"starred,attr,omitempty" json:"starred,omitempty"`
	Album      []AlbumID3 `xml:"album" json:"album,omitempty"`
}

// AlbumID3 is an album in the ID3 model.
type AlbumID3 struct {
	ID        string  `xml:"id,attr" json:"id"`
	Name      string  `xml:"name,attr" json:"name"`
	Artist    string  `xml:"artist,attr,omitempty" json:"artist,omitempty"`
	ArtistID  string  `xml:"artistId,attr,omitempty" json:"artistId,omitempty"`
	CoverArt  string  `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	SongCount int     `xml:"songCount,attr" json:"songCount"`
	Duration  int     `xml:"duration,attr" json:"duration"`
	Created   string  `xml:"created,attr,omitempty" json:"created,omitempty"`
	Year      int     `xml:"year,attr,omitempty" json:"year,omitempty"`
	Genre     string  `xml:"genre,attr,omitempty" json:"genre,omitempty"`
	Starred   string  `xml:"starred,attr,omitempty" json:"starred,omitempty"`
	Song      []Child `xml:"song" json:"song,omitempty"`
}

// AlbumList2 wraps the getAlbumList2 result.
type AlbumList2 struct {
	Album []AlbumID3 `xml:"album" json:"album,omitempty"`
}

// Child is the universal media entry (song or directory).
type Child struct {
	ID            string `xml:"id,attr" json:"id"`
	Parent        string `xml:"parent,attr,omitempty" json:"parent,omitempty"`
	IsDir         bool   `xml:"isDir,attr" json:"isDir"`
	Title         string `xml:"title,attr" json:"title"`
	Album         string `xml:"album,attr,omitempty" json:"album,omitempty"`
	Artist        string `xml:"artist,attr,omitempty" json:"artist,omitempty"`
	Track         int    `xml:"track,attr,omitempty" json:"track,omitempty"`
	Year          int    `xml:"year,attr,omitempty" json:"year,omitempty"`
	Genre         string `xml:"genre,attr,omitempty" json:"genre,omitempty"`
	CoverArt      string `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	Size          int64  `xml:"size,attr,omitempty" json:"size,omitempty"`
	ContentType   string `xml:"contentType,attr,omitempty" json:"contentType,omitempty"`
	Suffix        string `xml:"suffix,attr,omitempty" json:"suffix,omitempty"`
	Duration      int    `xml:"duration,attr,omitempty" json:"duration,omitempty"`
	BitRate       int    `xml:"bitRate,attr,omitempty" json:"bitRate,omitempty"`
	Path          string `xml:"path,attr,omitempty" json:"path,omitempty"`
	IsVideo       bool   `xml:"isVideo,attr" json:"isVideo"`
	PlayCount     int    `xml:"playCount,attr,omitempty" json:"playCount,omitempty"`
	DiscNumber    int    `xml:"discNumber,attr,omitempty" json:"discNumber,omitempty"`
	Created       string `xml:"created,attr,omitempty" json:"created,omitempty"`
	AlbumID       string `xml:"albumId,attr,omitempty" json:"albumId,omitempty"`
	ArtistID      string `xml:"artistId,attr,omitempty" json:"artistId,omitempty"`
	Type          string `xml:"type,attr,omitempty" json:"type,omitempty"`
	Starred       string `xml:"starred,attr,omitempty" json:"starred,omitempty"`
	UserRating    int    `xml:"userRating,attr,omitempty" json:"userRating,omitempty"`
	MusicBrainzID string `xml:"musicBrainzId,attr,omitempty" json:"musicBrainzId,omitempty"`
	// OpenSubsonic extensions.
	Composer       string        `xml:"composer,attr,omitempty" json:"composer,omitempty"`
	BPM            int           `xml:"bpm,attr,omitempty" json:"bpm,omitempty"`
	Work           string        `xml:"work,attr,omitempty" json:"work,omitempty"`
	MovementName   string        `xml:"movementName,attr,omitempty" json:"movementName,omitempty"`
	MovementNumber int           `xml:"movementNumber,attr,omitempty" json:"movementNumber,omitempty"`
	ReplayGain     *ReplayGain   `xml:"replayGain,omitempty" json:"replayGain,omitempty"`
	Contributors   []Contributor `xml:"contributors,omitempty" json:"contributors,omitempty"`
}

// ReplayGain is the OpenSubsonic per-track/album loudness data, in dB.
type ReplayGain struct {
	TrackGain float64 `xml:"trackGain,attr,omitempty" json:"trackGain,omitempty"`
	AlbumGain float64 `xml:"albumGain,attr,omitempty" json:"albumGain,omitempty"`
}

// Contributor is one OpenSubsonic role credit on a track. The artist carries
// only a name here (participants are not catalog entities), so no id is sent.
type Contributor struct {
	Role   string            `xml:"role,attr" json:"role"`
	Artist ContributorArtist `xml:"artist" json:"artist"`
}

// ContributorArtist is the minimal artist reference inside a Contributor.
type ContributorArtist struct {
	Name string `xml:"name,attr" json:"name"`
}

// Genres wraps the genre list.
type Genres struct {
	Genre []Genre `xml:"genre" json:"genre,omitempty"`
}

// Genre is a genre with counts. The name is element character data in XML.
type Genre struct {
	SongCount  int    `xml:"songCount,attr" json:"songCount"`
	AlbumCount int    `xml:"albumCount,attr" json:"albumCount"`
	Name       string `xml:",chardata" json:"value"`
}

// SearchResult3 is the ID3 search result.
type SearchResult3 struct {
	Artist []ArtistID3 `xml:"artist" json:"artist,omitempty"`
	Album  []AlbumID3  `xml:"album" json:"album,omitempty"`
	Song   []Child     `xml:"song" json:"song,omitempty"`
}

// Playlists wraps the playlist list.
type Playlists struct {
	Playlist []Playlist `xml:"playlist" json:"playlist,omitempty"`
}

// Playlist is a Subsonic playlist.
type Playlist struct {
	ID        string `xml:"id,attr" json:"id"`
	Name      string `xml:"name,attr" json:"name"`
	Comment   string `xml:"comment,attr,omitempty" json:"comment,omitempty"`
	Owner     string `xml:"owner,attr,omitempty" json:"owner,omitempty"`
	Public    bool   `xml:"public,attr" json:"public"`
	SongCount int    `xml:"songCount,attr" json:"songCount"`
	Duration  int    `xml:"duration,attr" json:"duration"`
	Created   string `xml:"created,attr,omitempty" json:"created,omitempty"`
	Changed   string `xml:"changed,attr,omitempty" json:"changed,omitempty"`
	CoverArt  string `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	// CoverArts is a immerle extension: the cover-art ids of the first few
	// tracks (up to 4) for a mosaic thumbnail.
	CoverArts []string `xml:"coverArts,omitempty" json:"coverArts,omitempty"`
	Entry     []Child  `xml:"entry" json:"entry,omitempty"`
}

// Users wraps the user list.
type Users struct {
	User []User `xml:"user" json:"user,omitempty"`
}

// User describes an account and its permissions.
type User struct {
	Username string `xml:"username,attr" json:"username"`
	Email    string `xml:"email,attr,omitempty" json:"email,omitempty"`
	// DisplayName is a immerle extension (not standard Subsonic): a free-text
	// UI name. Always present (empty string when unset) so clients can read it
	// directly and fall back to the username themselves.
	DisplayName       string `xml:"displayName,attr" json:"displayName"`
	AdminRole         bool   `xml:"adminRole,attr" json:"adminRole"`
	SettingsRole      bool   `xml:"settingsRole,attr" json:"settingsRole"`
	DownloadRole      bool   `xml:"downloadRole,attr" json:"downloadRole"`
	UploadRole        bool   `xml:"uploadRole,attr" json:"uploadRole"`
	PlaylistRole      bool   `xml:"playlistRole,attr" json:"playlistRole"`
	CoverArtRole      bool   `xml:"coverArtRole,attr" json:"coverArtRole"`
	CommentRole       bool   `xml:"commentRole,attr" json:"commentRole"`
	PodcastRole       bool   `xml:"podcastRole,attr" json:"podcastRole"`
	StreamRole        bool   `xml:"streamRole,attr" json:"streamRole"`
	JukeboxRole       bool   `xml:"jukeboxRole,attr" json:"jukeboxRole"`
	ShareRole         bool   `xml:"shareRole,attr" json:"shareRole"`
	ScrobblingEnabled bool   `xml:"scrobblingEnabled,attr" json:"scrobblingEnabled"`
}

// NowPlaying wraps now-playing entries.
type NowPlaying struct {
	Entry []NowPlayingEntry `xml:"entry" json:"entry,omitempty"`
}

// NowPlayingEntry is a now-playing item (a Child plus user/minutesAgo).
type NowPlayingEntry struct {
	Child
	Username   string `xml:"username,attr" json:"username"`
	MinutesAgo int    `xml:"minutesAgo,attr" json:"minutesAgo"`
}

// PlayQueue is the saved server-side play queue.
type PlayQueue struct {
	Current   string  `xml:"current,attr,omitempty" json:"current,omitempty"`
	Position  int64   `xml:"position,attr,omitempty" json:"position,omitempty"`
	Username  string  `xml:"username,attr" json:"username"`
	Changed   string  `xml:"changed,attr,omitempty" json:"changed,omitempty"`
	ChangedBy string  `xml:"changedBy,attr,omitempty" json:"changedBy,omitempty"`
	Entry     []Child `xml:"entry" json:"entry,omitempty"`
}

// Starred2 is the ID3 starred result.
type Starred2 struct {
	Artist []ArtistID3 `xml:"artist" json:"artist,omitempty"`
	Album  []AlbumID3  `xml:"album" json:"album,omitempty"`
	Song   []Child     `xml:"song" json:"song,omitempty"`
}

// Shares wraps the share list.
type Shares struct {
	Share []Share `xml:"share" json:"share,omitempty"`
}

// Share is a public share link.
type Share struct {
	ID          string  `xml:"id,attr" json:"id"`
	URL         string  `xml:"url,attr" json:"url"`
	Description string  `xml:"description,attr,omitempty" json:"description,omitempty"`
	Username    string  `xml:"username,attr" json:"username"`
	Created     string  `xml:"created,attr" json:"created"`
	Expires     string  `xml:"expires,attr,omitempty" json:"expires,omitempty"`
	VisitCount  int     `xml:"visitCount,attr" json:"visitCount"`
	Entry       []Child `xml:"entry" json:"entry,omitempty"`
}

// ScanStatus reports library scan progress.
type ScanStatus struct {
	Scanning bool `xml:"scanning,attr" json:"scanning"`
	Count    int  `xml:"count,attr" json:"count"`
}

// OpenSubsonicExtension is one supported extension and its versions.
type OpenSubsonicExtension struct {
	Name     string `xml:"name,attr" json:"name"`
	Versions []int  `xml:"versions" json:"versions"`
}

// newResponse builds a successful base response envelope.
func newResponse() *Response {
	return &Response{
		Status:        "ok",
		Version:       apiVersion,
		Type:          serverType,
		ServerVersion: serverVersion,
		OpenSubsonic:  true,
	}
}

// errorResponse builds a failed response with a Subsonic error code.
func errorResponse(code int, message string) *Response {
	r := newResponse()
	r.Status = "failed"
	r.Error = &Error{Code: code, Message: message}
	return r
}
