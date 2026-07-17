// Command iml is a minimal, UI-less terminal client for an immerle
// server: search songs/albums/playlists over the Subsonic API and play them.
// No queue editing, no casting -- search, pick, play, next.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type scope int

const (
	scopeSong scope = iota
	scopeAlbum
	scopePlaylist
)

func (s scope) String() string {
	return [...]string{"song", "album", "playlist"}[s]
}

var scopePrefixes = map[string]scope{"song": scopeSong, "album": scopeAlbum, "playlist": scopePlaylist}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#7D56F4")).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#3A3A3A"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#333333")).Padding(0, 1)
)

// defaultWidth/defaultHeight are used for the first frame or two, before
// bubbletea's initial tea.WindowSizeMsg arrives.
const defaultWidth, defaultHeight = 80, 24

// truncate cuts s to at most max runes, replacing the last one with an
// ellipsis when it had to cut -- keeps result rows from wrapping past the
// terminal width.
func truncate(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	if max == 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}

// parseQueryPrefix splits a leading "/song", "/album" or "/playlist" off the
// query text and returns the scope it selects. Typing "/playlist" switches
// scope immediately (term is whatever follows, often still empty).
func parseQueryPrefix(raw string) (sc scope, term string, ok bool) {
	rest, found := strings.CutPrefix(raw, "/")
	if !found {
		return 0, raw, false
	}
	word, remainder, _ := strings.Cut(rest, " ")
	sc, ok = scopePrefixes[word]
	if !ok {
		return 0, raw, false
	}
	return sc, remainder, true
}

type repeatMode int

const (
	repeatOff repeatMode = iota
	repeatAll
	repeatOne
)

func (r repeatMode) String() string {
	return [...]string{"off", "all", "one"}[r]
}

// result is a unified, playable search hit: a song plays itself; an album or
// playlist expands to its track list (fetched lazily on selection).
type result struct {
	kind     scope
	id       string
	title    string
	subtitle string
	tracks   []Song // pre-filled only for kind == scopeSong
}

type model struct {
	client *Client
	player *Player

	query   string
	scope   scope
	results []result
	cursor  int
	status  string

	queue           []Song
	queuePlaylistID string // set only when queue came from a playlist -- needed to resolve unresolved (federated) entries
	queuePos        int
	playing         bool
	repeat          repeatMode
	shuffle         bool

	width, height int
}

func initialModel(c *Client, p *Player) model {
	return model{client: c, player: p, status: "type to search, tab to change scope, enter to play"}
}

func (m model) Init() tea.Cmd { return nil }

type searchDoneMsg struct {
	results []result
	err     error
}

type tracksLoadedMsg struct {
	tracks     []Song
	playlistID string // set only when tracks came from a playlist
	err        error
}

func (m model) searchCmd() tea.Cmd {
	query, sc := m.query, m.scope
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if query == "" {
			return searchDoneMsg{}
		}

		var out []result
		songs, albums, playlists, err := m.client.Search(ctx, query, sc.String())
		if err != nil {
			return searchDoneMsg{err: err}
		}
		switch sc {
		case scopeSong:
			for _, s := range songs {
				out = append(out, result{kind: scopeSong, id: s.ID, title: s.Title, subtitle: s.Artist, tracks: []Song{s}})
			}
		case scopeAlbum:
			for _, a := range albums {
				out = append(out, result{kind: scopeAlbum, id: a.ID, title: a.Name, subtitle: a.Artist})
			}
		case scopePlaylist:
			for _, p := range playlists {
				out = append(out, result{kind: scopePlaylist, id: p.ID, title: p.Name, subtitle: fmt.Sprintf("%d songs", p.SongCount)})
			}
		}
		return searchDoneMsg{results: out}
	}
}

func (m model) loadTracksCmd(r result) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if len(r.tracks) > 0 {
			return tracksLoadedMsg{tracks: r.tracks}
		}
		var tracks []Song
		var err error
		var playlistID string
		switch r.kind {
		case scopeAlbum:
			tracks, err = m.client.AlbumTracks(ctx, r.id)
		case scopePlaylist:
			tracks, err = m.client.PlaylistTracks(ctx, r.id)
			playlistID = r.id
		}
		return tracksLoadedMsg{tracks: tracks, playlistID: playlistID, err: err}
	}
}

// streamReadyMsg carries the signed stream URL resolved for the track at
// queuePos when it was requested (see playCurrentCmd) -- fetching it is a
// network call, so it can't happen synchronously inside Update.
type streamReadyMsg struct {
	track Song
	url   string
	err   error
}

// playCurrentCmd resolves the stream URL for the track at queuePos, or nil
// (nothing to play) once the queue is exhausted -- the caller sets the
// "queue finished" status in that case. A federated-playlist entry that
// hasn't been matched to a real track yet (Unresolved, empty id) is resolved
// first -- the server checks the local catalog, then on-demand providers.
func (m model) playCurrentCmd() tea.Cmd {
	if m.queuePos >= len(m.queue) {
		return nil
	}
	t := m.queue[m.queuePos]
	client := m.client
	playlistID := m.queuePlaylistID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if t.Unresolved {
			resolved, err := client.ResolvePlaylistTrack(ctx, playlistID, t.Position)
			if err != nil {
				return streamReadyMsg{track: t, err: fmt.Errorf("resolve: %w", err)}
			}
			t = resolved
		}
		url, err := client.StreamURL(ctx, t.ID)
		return streamReadyMsg{track: t, url: url, err: err}
	}
}

// applyPrefix switches scope and consumes the "/type" prefix from the query
// once it's recognized, so the search itself only sees the actual term.
func (m *model) applyPrefix() {
	if sc, term, ok := parseQueryPrefix(m.query); ok {
		m.scope = sc
		m.query = term
		m.cursor = 0
	}
}

// advance moves to the next queue slot after the caller has already
// incremented queuePos (a manual skip, or trackFinished below): with
// repeat-all it wraps back to the start instead of stopping. A manual skip
// always moves forward regardless of repeat-one -- only a track finishing on
// its own (trackFinished) replays it.
func (m *model) advance() tea.Cmd {
	if m.queuePos >= len(m.queue) {
		if m.repeat == repeatAll && len(m.queue) > 0 {
			m.queuePos = 0
		} else {
			m.playing = false
			m.status = "queue finished"
			return nil
		}
	}
	return m.playCurrentCmd()
}

// trackFinished handles the queue advancing after a track completes on its
// own: repeat-one replays the same slot, everything else defers to advance.
func (m *model) trackFinished() tea.Cmd {
	if m.repeat == repeatOne {
		return m.playCurrentCmd()
	}
	m.queuePos++
	return m.advance()
}

// shuffleFrom shuffles m.queue[from:] in place.
func shuffleFrom(queue []Song, from int) {
	rest := queue[from:]
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyTab:
			m.scope = (m.scope + 1) % 3
			m.cursor = 0
			return m, m.searchCmd()
		case tea.KeyEnter:
			if m.cursor < len(m.results) {
				r := m.results[m.cursor]
				return m, m.loadTracksCmd(r)
			}
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
		case tea.KeyBackspace:
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				return m, m.searchCmd()
			}
		case tea.KeySpace:
			if m.playing || len(m.queue) > 0 {
				m.player.TogglePause()
				return m, nil
			}
			m.query += " "
			m.applyPrefix()
			return m, m.searchCmd()
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "n":
				if len(m.queue) > 0 {
					m.player.Stop()
					m.queuePos++
					return m, m.advance()
				}
			case "q":
				return m, tea.Quit
			case "+", "=":
				if m.playing || len(m.queue) > 0 {
					vol := m.player.AdjustVolume(volumeStep)
					m.status = fmt.Sprintf("volume %d%%", int(vol*100+0.5))
					return m, nil
				}
				m.query += string(msg.Runes)
				m.applyPrefix()
				return m, m.searchCmd()
			case "-":
				if m.playing || len(m.queue) > 0 {
					vol := m.player.AdjustVolume(-volumeStep)
					m.status = fmt.Sprintf("volume %d%%", int(vol*100+0.5))
					return m, nil
				}
				m.query += string(msg.Runes)
				m.applyPrefix()
				return m, m.searchCmd()
			case "r":
				if m.playing || len(m.queue) > 0 {
					m.repeat = (m.repeat + 1) % 3
					m.status = "repeat: " + m.repeat.String()
					return m, nil
				}
				m.query += string(msg.Runes)
				m.applyPrefix()
				return m, m.searchCmd()
			case "s":
				if m.playing || len(m.queue) > 0 {
					m.shuffle = !m.shuffle
					if m.shuffle && m.queuePos+1 < len(m.queue) {
						shuffleFrom(m.queue, m.queuePos+1)
					}
					m.status = fmt.Sprintf("shuffle: %v", m.shuffle)
					return m, nil
				}
				m.query += string(msg.Runes)
				m.applyPrefix()
				return m, m.searchCmd()
			default:
				m.query += string(msg.Runes)
				m.applyPrefix()
				return m, m.searchCmd()
			}
		}

	case searchDoneMsg:
		if msg.err != nil {
			m.status = "search error: " + msg.err.Error()
			return m, nil
		}
		m.results = msg.results
		m.cursor = 0

	case tracksLoadedMsg:
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			return m, nil
		}
		m.queue = msg.tracks
		m.queuePlaylistID = msg.playlistID
		if m.shuffle {
			shuffleFrom(m.queue, 0)
		}
		m.queuePos = 0
		return m, m.advance()

	case streamReadyMsg:
		if msg.err != nil {
			m.status = "stream error: " + msg.err.Error()
			return m, nil
		}
		if m.queuePos < len(m.queue) {
			m.queue[m.queuePos] = msg.track // cache the resolved track so repeat/replay skips re-resolving
		}
		m.player.Play(msg.url)
		m.playing = true
		m.status = fmt.Sprintf("playing %s - %s (%d/%d)", msg.track.Artist, msg.track.Title, m.queuePos+1, len(m.queue))

	case playerMsg:
		if msg.err != nil {
			m.status = "playback error: " + msg.err.Error()
			return m, nil
		}
		if msg.finished && len(m.queue) > 0 {
			return m, m.trackFinished()
		}
	}
	return m, nil
}

func (m model) View() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}

	header := headerStyle.Width(width - 2).Render(fmt.Sprintf("iml  [%s]  %s   repeat:%s shuffle:%v", m.scope, m.query, m.repeat, m.shuffle))
	status := statusStyle.Width(width - 2).Render(truncate(m.status, width-2))
	help := dimStyle.Render("tab or /song /album /playlist: scope  enter: play  space: pause  n: next  +/-: volume  r: repeat  s: shuffle  q: quit")

	// Results get whatever rows are left once the header, status and help
	// bars (plus their blank-line spacers) are accounted for.
	maxRows := height - 6
	if maxRows < 1 {
		maxRows = 1
	}
	shown := m.results
	var more string
	if len(shown) > maxRows {
		shown = shown[:maxRows]
		more = dimStyle.Render(fmt.Sprintf("  … %d more", len(m.results)-maxRows)) + "\n"
	}

	var list strings.Builder
	for i, r := range shown {
		line := truncate(fmt.Sprintf("[%-8s] %s  (%s)", r.kind, r.title, r.subtitle), width-4)
		if i == m.cursor {
			list.WriteString(selectedStyle.Width(width - 2).Render("> " + line))
		} else {
			list.WriteString("  " + line)
		}
		list.WriteString("\n")
	}
	list.WriteString(more)

	return lipgloss.JoinVertical(lipgloss.Left, header, "", list.String(), status, help)
}

// session is what's persisted to ~/.immerle/config.json: the server and the
// device-session JWT from the last login, so it only runs again once the
// token expires (there's no refresh endpoint -- a fresh login is how you get
// a new one).
type session struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".immerle", "config.json"), nil
}

func loadSession() (session, bool) {
	path, err := configPath()
	if err != nil {
		return session{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return session{}, false
	}
	var s session
	if err := json.Unmarshal(data, &s); err != nil {
		return session{}, false
	}
	return s, s.Server != "" && s.Token != ""
}

// saveSession writes the config 0600: it holds a live auth token.
func saveSession(s session) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// loginInput is what promptLogin collects -- only Password never gets
// persisted (session stores the resulting token instead).
type loginInput struct {
	Server   string
	Username string
	Password string
}

// promptLogin asks for username/password, and for the server URL too unless
// defaultServer is already known (a re-login after the saved token expired).
func promptLogin(defaultServer string) (loginInput, error) {
	reader := bufio.NewReader(os.Stdin)

	server := defaultServer
	if server == "" {
		fmt.Print("server URL: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return loginInput{}, err
		}
		server = strings.TrimSpace(line)
	}

	fmt.Print("username: ")
	user, err := reader.ReadString('\n')
	if err != nil {
		return loginInput{}, err
	}

	fmt.Print("password: ")
	passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return loginInput{}, err
	}

	return loginInput{
		Server:   server,
		Username: strings.TrimSpace(user),
		Password: string(passBytes),
	}, nil
}

// login prompts for credentials (pre-filling the server when already known),
// authenticates and persists the resulting token. The network timeout is
// created here, after promptLogin returns -- it must not start ticking while
// still waiting on the human to type, or a careful password entry can burn
// through the whole budget before the request is even sent.
func login(defaultServer string) (*Client, error) {
	in, err := promptLogin(defaultServer)
	if err != nil {
		return nil, fmt.Errorf("input error: %w", err)
	}
	client := NewClient(in.Server)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Login(ctx, in.Username, in.Password); err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}
	if err := saveSession(session{Server: in.Server, Token: client.token}); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not save session:", err)
	}
	return client, nil
}

func logout() {
	path, err := configPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "logout failed:", err)
		os.Exit(1)
	}
	fmt.Println("logged out")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "logout" {
		logout()
		return
	}

	var client *Client
	if sess, ok := loadSession(); ok {
		client = NewClient(sess.Server)
		client.token = sess.Token
		meCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := client.Me(meCtx)
		cancel()
		switch {
		case err == nil:
			// saved token still valid
		case errors.Is(err, ErrUnauthorized):
			client, err = login(sess.Server)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		default:
			fmt.Fprintln(os.Stderr, "connection failed:", err)
			os.Exit(1)
		}
	} else {
		var err error
		client, err = login("")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	var program *tea.Program
	player, err := NewPlayer(func(msg playerMsg) {
		if program != nil {
			program.Send(msg)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "audio init failed:", err)
		os.Exit(1)
	}

	program = tea.NewProgram(initialModel(client, player), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
