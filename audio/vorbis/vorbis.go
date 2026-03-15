// Package vorbis provides OGG Vorbis audio decoding for Future Render.
//
// The decoder wraps github.com/jfreymuth/oggvorbis and produces signed
// 16-bit little-endian stereo PCM data suitable for audio.Context.NewPlayer.
package vorbis

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/jfreymuth/oggvorbis"
)

// Stream is a decoded OGG Vorbis audio stream. It provides signed 16-bit
// LE stereo PCM data.
type Stream struct {
	data       *bytes.Reader
	raw        []byte
	sampleRate int
	length     int64
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) {
	return s.data.Read(p)
}

// Seek implements io.Seeker.
func (s *Stream) Seek(offset int64, whence int) (int64, error) {
	return s.data.Seek(offset, whence)
}

// Length returns the total length of decoded audio data in bytes.
func (s *Stream) Length() int64 {
	return s.length
}

// SampleRate returns the sample rate of the OGG Vorbis file.
func (s *Stream) SampleRate() int {
	return s.sampleRate
}

// Decode reads an OGG Vorbis file from src and returns a Stream of stereo
// 16-bit signed LE PCM data at the file's native sample rate.
func Decode(src io.Reader) (*Stream, error) {
	reader, err := oggvorbis.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("vorbis: open reader: %w", err)
	}

	channels := reader.Channels()
	sampleRate := reader.SampleRate()

	allSamples, err := readAllSamples(reader)
	if err != nil {
		return nil, err
	}

	return newStream(allSamples, channels, sampleRate), nil
}

// float32Reader reads float32 samples.
type float32Reader interface {
	Read(p []float32) (int, error)
}

// readAllSamples reads all float32 samples from a vorbis reader.
func readAllSamples(r float32Reader) ([]float32, error) {
	buf := make([]float32, 4096)
	var allSamples []float32
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			allSamples = append(allSamples, buf[:n]...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("vorbis: decode: %w", readErr)
		}
	}
	return allSamples, nil
}

// newStream creates a Stream from float32 samples.
func newStream(samples []float32, channels, sampleRate int) *Stream {
	pcm16 := floatToStereo16(samples, channels)
	return &Stream{
		data:       bytes.NewReader(pcm16),
		raw:        pcm16,
		sampleRate: sampleRate,
		length:     int64(len(pcm16)),
	}
}

// DecodeWithSampleRate reads an OGG Vorbis file from src and resamples the
// output to the given sample rate.
func DecodeWithSampleRate(sampleRate int, src io.Reader) (*Stream, error) {
	s, err := Decode(src)
	if err != nil {
		return nil, err
	}
	if s.sampleRate == sampleRate {
		return s, nil
	}
	resampled := resample(s.raw, s.sampleRate, sampleRate)
	return &Stream{
		data:       bytes.NewReader(resampled),
		raw:        resampled,
		sampleRate: sampleRate,
		length:     int64(len(resampled)),
	}, nil
}

// floatToStereo16 converts interleaved float32 samples to stereo 16-bit
// signed LE PCM.
func floatToStereo16(samples []float32, channels int) []byte {
	framesCount := len(samples) / channels
	out := make([]byte, framesCount*4) // stereo 16-bit = 4 bytes per frame

	for i := 0; i < framesCount; i++ {
		left := clampFloat32(samples[i*channels])
		var right float32
		if channels >= 2 {
			right = clampFloat32(samples[i*channels+1])
		} else {
			right = left
		}

		binary.LittleEndian.PutUint16(out[i*4:], uint16(floatToInt16(left)))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(floatToInt16(right)))
	}
	return out
}

// clampFloat32 clamps a float32 to [-1.0, 1.0].
func clampFloat32(v float32) float32 {
	if v > 1.0 {
		return 1.0
	}
	if v < -1.0 {
		return -1.0
	}
	return v
}

// floatToInt16 converts a float32 in [-1.0, 1.0] to int16.
func floatToInt16(v float32) int16 {
	return int16(v * float32(math.MaxInt16))
}

// resample performs linear interpolation resampling from srcRate to dstRate.
// Input and output are stereo 16-bit signed LE PCM.
func resample(data []byte, srcRate, dstRate int) []byte {
	srcFrames := len(data) / 4
	if srcFrames < 2 {
		return data
	}

	ratio := float64(srcRate) / float64(dstRate)
	dstFrames := int(float64(srcFrames) / ratio)
	out := make([]byte, dstFrames*4)

	for i := 0; i < dstFrames; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= srcFrames-1 {
			srcIdx = srcFrames - 2
			frac = 1.0
		}
		if srcIdx < 0 {
			srcIdx = 0
			frac = 0
		}

		l0 := int16(binary.LittleEndian.Uint16(data[srcIdx*4:]))
		l1 := int16(binary.LittleEndian.Uint16(data[(srcIdx+1)*4:]))
		left := int16(float64(l0) + frac*(float64(l1)-float64(l0)))

		r0 := int16(binary.LittleEndian.Uint16(data[srcIdx*4+2:]))
		r1 := int16(binary.LittleEndian.Uint16(data[(srcIdx+1)*4+2:]))
		right := int16(float64(r0) + frac*(float64(r1)-float64(r0)))

		binary.LittleEndian.PutUint16(out[i*4:], uint16(left))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(right))
	}
	return out
}
