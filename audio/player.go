package audio

import (
	"errors"
	"io"
	"time"
)

// Player plays audio from an io.Reader source. It wraps an oto.Player and
// provides time-based position and seek operations.
type Player struct {
	player     otoPlayer
	src        io.Reader
	sampleRate int
}

// Play starts or resumes playback.
func (p *Player) Play() {
	p.player.Play()
}

// Pause pauses playback. Call Play() to resume.
func (p *Player) Pause() {
	p.player.Pause()
}

// IsPlaying returns whether the player is currently playing.
func (p *Player) IsPlaying() bool {
	return p.player.IsPlaying()
}

// SetVolume sets the playback volume. 0.0 is silent, 1.0 is full volume.
func (p *Player) SetVolume(volume float64) {
	p.player.SetVolume(volume)
}

// Volume returns the current playback volume.
func (p *Player) Volume() float64 {
	return p.player.Volume()
}

// SetPosition seeks to the given time offset from the start.
// The source must implement io.Seeker.
func (p *Player) SetPosition(offset time.Duration) error {
	seeker, ok := p.src.(io.Seeker)
	if !ok {
		return errors.New("audio: source does not support seeking")
	}
	byteOffset := durationToBytes(offset, p.sampleRate)
	if _, err := seeker.Seek(byteOffset, io.SeekStart); err != nil {
		return err
	}
	// Reset the oto player's internal buffer after seeking.
	p.player.Seek(byteOffset, io.SeekStart) //nolint:errcheck // best-effort reset of oto internal buffer
	return nil
}

// Rewind resets playback to the beginning.
func (p *Player) Rewind() error {
	return p.SetPosition(0)
}

// Close stops playback and releases resources.
func (p *Player) Close() error {
	p.player.Pause()
	return nil
}

// Err returns any error that occurred during playback.
func (p *Player) Err() error {
	return p.player.Err()
}

// durationToBytes converts a time.Duration to a byte offset for the given
// sample rate. Format is stereo signed 16-bit LE (4 bytes per sample frame).
func durationToBytes(d time.Duration, sampleRate int) int64 {
	samples := int64(d.Seconds() * float64(sampleRate))
	return samples * bytesPerSample
}

// bytesToDuration converts a byte offset to a time.Duration for the given
// sample rate.
func bytesToDuration(bytes int64, sampleRate int) time.Duration {
	samples := bytes / bytesPerSample
	return time.Duration(samples) * time.Second / time.Duration(sampleRate)
}
