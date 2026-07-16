package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

// sampleRate is fixed rather than read per-track from the mp3 decoder.
// ponytail: oto needs one sample rate for its whole process lifetime; a
// library with mixed sample rates would play some tracks pitch-shifted.
// Fix: recreate the oto context per track if that ever shows up.
const sampleRate = 44100

// Player streams one track at a time through go-mp3 -> oto. It reports
// progress and completion to the TUI via onEvent, called from its own
// goroutine (never from Update), so callers must bridge it with
// tea.Program.Send.
type Player struct {
	ctx     *oto.Context
	onEvent func(playerMsg)

	mu      sync.Mutex
	current *oto.Player
	closer  func() error
	paused  bool
	volume  float64 // [0,1], carried over across tracks since each gets a new oto.Player
	gen     int     // bumps on every Play/Stop, so a stale goroutine's events are ignored
}

// volumeStep is how much each key press changes the volume by.
const volumeStep = 0.1

type playerMsg struct {
	gen      int
	finished bool
	err      error
}

func NewPlayer(onEvent func(playerMsg)) (*Player, error) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, fmt.Errorf("audio init: %w", err)
	}
	<-ready
	return &Player{ctx: ctx, onEvent: onEvent, volume: 1}, nil
}

// Play stops whatever is playing and streams a new URL.
func (p *Player) Play(url string) {
	p.mu.Lock()
	p.gen++
	gen := p.gen
	p.stopLocked()
	p.mu.Unlock()

	go p.run(gen, url)
}

func (p *Player) run(gen int, url string) {
	resp, err := http.Get(url) //nolint:gosec // server URL is built by this same process
	if err != nil {
		p.onEvent(playerMsg{gen: gen, err: err})
		return
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		p.onEvent(playerMsg{gen: gen, err: fmt.Errorf("stream: HTTP %d", resp.StatusCode)})
		return
	}

	dec, err := mp3.NewDecoder(resp.Body)
	if err != nil {
		resp.Body.Close()
		p.onEvent(playerMsg{gen: gen, err: fmt.Errorf("decode: %w", err)})
		return
	}

	oplayer := p.ctx.NewPlayer(dec)

	p.mu.Lock()
	if gen != p.gen { // superseded while we were setting up
		p.mu.Unlock()
		resp.Body.Close()
		return
	}
	oplayer.SetVolume(p.volume)
	p.current = oplayer
	p.closer = resp.Body.Close
	p.paused = false
	p.mu.Unlock()

	oplayer.Play()
	for {
		time.Sleep(100 * time.Millisecond)
		p.mu.Lock()
		stopped := gen != p.gen
		paused := p.paused
		p.mu.Unlock()
		if stopped {
			return // Stop/Play already fired the closer; no "finished" event
		}
		if paused {
			continue // IsPlaying() is also false while paused -- not the same as finished
		}
		if !oplayer.IsPlaying() {
			break
		}
	}
	resp.Body.Close()
	p.onEvent(playerMsg{gen: gen, finished: true})
}

func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return
	}
	if p.paused {
		p.paused = false
		p.current.Play()
	} else {
		p.paused = true
		p.current.Pause()
	}
}

// AdjustVolume changes the volume by delta (positive or negative), clamped to
// [0,1], and returns the resulting value.
func (p *Player) AdjustVolume(delta float64) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume += delta
	if p.volume < 0 {
		p.volume = 0
	}
	if p.volume > 1 {
		p.volume = 1
	}
	if p.current != nil {
		p.current.SetVolume(p.volume)
	}
	return p.volume
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gen++
	p.stopLocked()
}

// stopLocked silences and releases whatever is currently playing. Pausing
// (not Close, a no-op as of oto v3.4) is what actually stops the audio.
func (p *Player) stopLocked() {
	if p.current != nil {
		p.current.Pause()
		p.current = nil
	}
	if p.closer != nil {
		p.closer()
		p.closer = nil
	}
}
