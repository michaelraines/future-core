//go:build glfw

// Command audio demonstrates audio playback: generating a sine wave WAV
// in memory, decoding it, looping it with InfiniteLoop, and controlling
// playback with keyboard input.
//
// Build: go build -tags glfw ./cmd/audio
// Run:   ./audio
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"

	futurerender "github.com/michaelraines/future-render"
	"github.com/michaelraines/future-render/audio"
	"github.com/michaelraines/future-render/audio/wav"
	fmath "github.com/michaelraines/future-render/math"
	"github.com/michaelraines/future-render/text"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	screenW    = 640
	screenH    = 480
	sampleRate = 44100
	freq       = 440.0 // A4 note
	duration   = 1.0   // seconds
)

// generateSineWAV builds a mono 16-bit PCM WAV file containing a sine wave
// at the given frequency and duration, returned as raw bytes.
func generateSineWAV() []byte {
	numSamples := int(sampleRate * duration)
	dataSize := numSamples * 2 // 16-bit = 2 bytes per sample
	fileSize := 36 + dataSize  // RIFF header (12) + fmt chunk (24) + data header (8) - 8 for RIFF id/size = 36

	buf := new(bytes.Buffer)
	buf.Grow(44 + dataSize)

	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(fileSize))
	buf.WriteString("WAVE")

	// fmt sub-chunk
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))           // chunk size
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))            // audio format (PCM)
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))            // channels (mono)
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))   // sample rate
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate*2)) // byte rate (sampleRate * channels * bitsPerSample/8)
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))            // block align (channels * bitsPerSample/8)
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))           // bits per sample

	// data sub-chunk
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))

	// Generate sine wave samples.
	for i := range numSamples {
		t := float64(i) / float64(sampleRate)
		sample := int16(math.Sin(2*math.Pi*freq*t) * 0.5 * math.MaxInt16)
		_ = binary.Write(buf, binary.LittleEndian, sample)
	}

	return buf.Bytes()
}

type audioGame struct {
	face    *text.Face
	ctx     *audio.Context
	player  *audio.Player
	initErr error
}

func (g *audioGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}

	if g.player == nil {
		return nil
	}

	// Toggle play/pause on space.
	if futurerender.IsKeyJustPressed(futurerender.KeySpace) {
		if g.player.IsPlaying() {
			g.player.Pause()
		} else {
			g.player.Play()
		}
	}

	return nil
}

func (g *audioGame) Draw(screen *futurerender.Image) {
	screen.Fill(futurerender.ColorFromRGBA(0.05, 0.05, 0.15, 1.0))

	if g.face == nil {
		return
	}

	// Title
	titleOpts := &text.DrawOptions{
		ColorScale: fmath.Color{R: 1, G: 1, B: 1, A: 1},
	}
	text.Draw(screen, "Future Render — Audio Example", g.face, 20, 30, titleOpts)

	// Instructions
	instrOpts := &text.DrawOptions{
		ColorScale: fmath.Color{R: 0.7, G: 0.8, B: 1.0, A: 1},
	}
	text.Draw(screen, "Controls:", g.face, 20, 80, instrOpts)
	text.Draw(screen, "  Space  — Play / Pause", g.face, 20, 110, instrOpts)
	text.Draw(screen, "  Escape — Exit", g.face, 20, 140, instrOpts)

	// Playback state
	if g.initErr != nil {
		errOpts := &text.DrawOptions{
			ColorScale: fmath.Color{R: 1, G: 0.3, B: 0.3, A: 1},
		}
		text.Draw(screen, fmt.Sprintf("Audio init error: %v", g.initErr), g.face, 20, 200, errOpts)
		return
	}

	if g.player == nil {
		return
	}

	state := "Paused"
	stateColor := fmath.Color{R: 1, G: 0.6, B: 0.2, A: 1}
	if g.player.IsPlaying() {
		state = "Playing"
		stateColor = fmath.Color{R: 0.3, G: 1, B: 0.3, A: 1}
	}
	stateOpts := &text.DrawOptions{
		ColorScale: stateColor,
	}
	text.Draw(screen, fmt.Sprintf("Status: %s", state), g.face, 20, 200, stateOpts)

	infoOpts := &text.DrawOptions{
		ColorScale: fmath.Color{R: 0.6, G: 0.6, B: 0.6, A: 1},
	}
	text.Draw(screen, fmt.Sprintf("Tone: %gHz sine wave, %gs, %dHz sample rate", freq, duration, sampleRate), g.face, 20, 240, infoOpts)
}

func (g *audioGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

func main() {
	futurerender.SetWindowSize(screenW, screenH)
	futurerender.SetWindowTitle("Future Render — Audio Example")

	game := &audioGame{}
	wrapper := &lazyInitGame{game: game}

	if err := futurerender.RunGame(wrapper); err != nil {
		log.Fatal(err)
	}
}

// lazyInitGame wraps audioGame to initialize GPU-dependent resources and
// audio on first Update, after the rendering device is ready.
type lazyInitGame struct {
	game   *audioGame
	inited bool
}

func (g *lazyInitGame) Update() error {
	if !g.inited {
		g.inited = true

		// Initialize font face.
		face, err := text.NewFace(goregular.TTF, 18)
		if err != nil {
			log.Printf("failed to create font face: %v", err)
		}
		g.game.face = face

		// Initialize audio.
		g.game.initErr = g.initAudio()
	}
	return g.game.Update()
}

func (g *lazyInitGame) initAudio() error {
	ctx, err := audio.NewContext(sampleRate)
	if err != nil {
		return fmt.Errorf("create audio context: %w", err)
	}
	g.game.ctx = ctx

	// Generate a sine wave WAV in memory.
	wavData := generateSineWAV()

	// Decode the WAV.
	stream, err := wav.Decode(bytes.NewReader(wavData))
	if err != nil {
		return fmt.Errorf("decode WAV: %w", err)
	}

	// Wrap in an infinite loop for continuous playback.
	loop := audio.NewInfiniteLoop(stream, stream.Length())

	// Create and start the player.
	player := ctx.NewPlayer(loop)
	player.Play()
	g.game.player = player

	return nil
}

func (g *lazyInitGame) Draw(screen *futurerender.Image) {
	g.game.Draw(screen)
}

func (g *lazyInitGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.game.Layout(outsideWidth, outsideHeight)
}
