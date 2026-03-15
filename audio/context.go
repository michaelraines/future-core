// Package audio provides audio playback for Future Render.
//
// The audio system is built on composable io.Reader streams. Decoders produce
// streams of signed 16-bit little-endian stereo PCM data, which are fed to
// Players for playback. Multiple Players mix automatically.
//
// Basic usage:
//
//	ctx, _ := audio.NewContext(48000)
//	f, _ := os.Open("bgm.ogg")
//	stream, _ := vorbis.Decode(f)
//	player := ctx.NewPlayer(stream)
//	player.Play()
package audio

import (
	"errors"
	"io"
	"sync"

	"github.com/ebitengine/oto/v3"
)

// bytesPerSample is the number of bytes per sample frame (stereo 16-bit).
const bytesPerSample = 4

// otoPlayer is the interface satisfied by *oto.Player, used for testing.
type otoPlayer interface {
	Play()
	Pause()
	IsPlaying() bool
	SetVolume(volume float64)
	Volume() float64
	Err() error
	Seek(offset int64, whence int) (int64, error)
}

// playerFactory creates oto-compatible players from io.Readers.
type playerFactory interface {
	newPlayer(src io.Reader) otoPlayer
}

// otoFactory wraps a real oto.Context as a playerFactory.
type otoFactory struct {
	ctx *oto.Context
}

func (f *otoFactory) newPlayer(src io.Reader) otoPlayer {
	return f.ctx.NewPlayer(src)
}

// Context is the audio playback context. At most one Context may exist per
// process. All Players are created from a Context and share its sample rate.
type Context struct {
	factory    playerFactory
	ready      <-chan struct{}
	mu         sync.Mutex
	closed     bool
	sampleRate int
}

var (
	currentCtx   *Context
	currentCtxMu sync.Mutex
)

// NewContext creates a new audio Context with the given sample rate.
// Only one Context may exist at a time; creating a second returns an error.
func NewContext(sampleRate int) (*Context, error) {
	currentCtxMu.Lock()
	defer currentCtxMu.Unlock()

	if currentCtx != nil {
		return nil, errors.New("audio: context already exists")
	}

	otoCtx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, err
	}

	c := &Context{
		factory:    &otoFactory{ctx: otoCtx},
		ready:      ready,
		sampleRate: sampleRate,
	}
	currentCtx = c
	return c, nil
}

// newTestContext creates a Context with a mock factory for testing.
func newTestContext(factory playerFactory) *Context {
	ch := make(chan struct{})
	close(ch) // immediately ready
	return &Context{
		factory:    factory,
		ready:      ch,
		sampleRate: 48000,
	}
}

// CurrentContext returns the current audio Context, or nil if none exists.
func CurrentContext() *Context {
	currentCtxMu.Lock()
	defer currentCtxMu.Unlock()
	return currentCtx
}

// SampleRate returns the Context's sample rate in Hz.
func (c *Context) SampleRate() int {
	return c.sampleRate
}

// IsReady returns whether the audio device has been initialized and is ready
// for playback.
func (c *Context) IsReady() bool {
	select {
	case <-c.ready:
		return true
	default:
		return false
	}
}

// NewPlayer creates a new Player that reads audio data from src.
// The source must provide signed 16-bit little-endian stereo PCM data
// at the Context's sample rate.
func (c *Context) NewPlayer(src io.Reader) *Player {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	p := c.factory.newPlayer(src)
	return &Player{
		player:     p,
		src:        src,
		sampleRate: c.sampleRate,
	}
}

// Close shuts down the audio context and releases resources.
// After Close, no new Players can be created.
func (c *Context) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	currentCtxMu.Lock()
	if currentCtx == c {
		currentCtx = nil
	}
	currentCtxMu.Unlock()

	return nil
}

// resetForTesting resets the global context state. Test-only.
func resetForTesting() {
	currentCtxMu.Lock()
	currentCtx = nil
	currentCtxMu.Unlock()
}
