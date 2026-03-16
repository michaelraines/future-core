// Package mp3 provides MP3 audio decoding for Future Render.
//
// The decoder wraps github.com/hajimehoshi/go-mp3 and produces signed
// 16-bit little-endian stereo PCM data suitable for audio.Context.NewPlayer.
//
// When the source implements io.Seeker, Decode streams directly from the
// underlying go-mp3 Decoder without buffering the entire file in memory.
package mp3

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	gomp3 "github.com/hajimehoshi/go-mp3"

	"github.com/michaelraines/future-render/audio/internal/pcm"
)

// Stream is a decoded MP3 audio stream. It provides signed 16-bit
// LE stereo PCM data.
type Stream struct {
	reader     io.ReadSeeker
	sampleRate int
	length     int64
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

// Seek implements io.Seeker.
func (s *Stream) Seek(offset int64, whence int) (int64, error) {
	return s.reader.Seek(offset, whence)
}

// Length returns the total length of decoded audio data in bytes.
func (s *Stream) Length() int64 {
	return s.length
}

// SampleRate returns the sample rate of the MP3 file.
func (s *Stream) SampleRate() int {
	return s.sampleRate
}

// Decode reads an MP3 file from src and returns a Stream of stereo
// 16-bit signed LE PCM data at the file's native sample rate.
//
// If src implements io.Seeker, the returned Stream reads directly from the
// go-mp3 Decoder on the fly, avoiding buffering the full decoded audio in
// memory. Otherwise, the entire decoded output is buffered.
func Decode(src io.Reader) (*Stream, error) {
	decoder, err := gomp3.NewDecoder(src)
	if err != nil {
		return nil, fmt.Errorf("mp3: open decoder: %w", err)
	}

	sampleRate := decoder.SampleRate()

	// If the source is seekable, the decoder supports Seek and Length.
	// Stream directly without buffering.
	if decoder.Length() >= 0 {
		return &Stream{
			reader:     decoder,
			sampleRate: sampleRate,
			length:     decoder.Length(),
		}, nil
	}

	// Non-seekable source: buffer everything for seeking support.
	data, err := readAll(decoder)
	if err != nil {
		return nil, err
	}

	return &Stream{
		reader:     bytes.NewReader(data),
		sampleRate: sampleRate,
		length:     int64(len(data)),
	}, nil
}

// readAll reads all PCM bytes from an io.Reader.
func readAll(r io.Reader) ([]byte, error) {
	buf := make([]byte, 8192)
	var all []byte
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			all = append(all, buf[:n]...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("mp3: decode: %w", readErr)
		}
	}
	return all, nil
}

// DecodeWithSampleRate reads an MP3 file from src and resamples the
// output to the given sample rate.
func DecodeWithSampleRate(sampleRate int, src io.Reader) (*Stream, error) {
	s, err := Decode(src)
	if err != nil {
		return nil, err
	}
	if s.sampleRate == sampleRate {
		return s, nil
	}

	// Resampling requires the full PCM data in memory.
	data, err := io.ReadAll(s.reader)
	if err != nil {
		return nil, fmt.Errorf("mp3: read for resample: %w", err)
	}

	resampled := pcm.Resample(data, s.sampleRate, sampleRate)
	return &Stream{
		reader:     bytes.NewReader(resampled),
		sampleRate: sampleRate,
		length:     int64(len(resampled)),
	}, nil
}
