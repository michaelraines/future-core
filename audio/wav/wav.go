// Package wav provides WAV audio decoding for Future Render.
//
// The decoder reads RIFF/WAV files and produces signed 16-bit little-endian
// stereo PCM data suitable for audio.Context.NewPlayer.
//
// Supported input formats: 8-bit unsigned PCM, 16-bit signed LE PCM,
// mono or stereo. Output is always stereo 16-bit signed LE.
package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Stream is a decoded WAV audio stream. It provides signed 16-bit LE stereo
// PCM data. If the underlying source implements io.Seeker, Stream also
// supports seeking.
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

// SampleRate returns the original sample rate of the WAV file.
func (s *Stream) SampleRate() int {
	return s.sampleRate
}

// wavHeader represents the key fields from a RIFF/WAV file header.
type wavHeader struct {
	audioFormat   uint16
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
	dataSize      uint32
}

// Decode reads a WAV file from src and returns a Stream of stereo 16-bit
// signed LE PCM data at the file's native sample rate.
func Decode(src io.Reader) (*Stream, error) {
	return decode(src)
}

// DecodeWithSampleRate reads a WAV file from src and resamples the output
// to the given sample rate. The output is stereo 16-bit signed LE PCM.
func DecodeWithSampleRate(sampleRate int, src io.Reader) (*Stream, error) {
	s, err := decode(src)
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

func decode(src io.Reader) (*Stream, error) {
	all, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("wav: read source: %w", err)
	}

	hdr, dataStart, err := parseHeader(all)
	if err != nil {
		return nil, err
	}

	if hdr.audioFormat != 1 {
		return nil, fmt.Errorf("wav: unsupported audio format %d (only PCM supported)", hdr.audioFormat)
	}

	rawData := all[dataStart : dataStart+int(hdr.dataSize)]
	pcm16Stereo := convertToStereo16(rawData, hdr.numChannels, hdr.bitsPerSample)

	return &Stream{
		data:       bytes.NewReader(pcm16Stereo),
		raw:        pcm16Stereo,
		sampleRate: int(hdr.sampleRate),
		length:     int64(len(pcm16Stereo)),
	}, nil
}

func parseHeader(data []byte) (wavHeader, int, error) {
	if len(data) < 44 {
		return wavHeader{}, 0, errors.New("wav: file too short")
	}

	// RIFF header
	if string(data[0:4]) != "RIFF" {
		return wavHeader{}, 0, errors.New("wav: missing RIFF header")
	}
	if string(data[8:12]) != "WAVE" {
		return wavHeader{}, 0, errors.New("wav: missing WAVE format")
	}

	// Find fmt and data chunks by scanning.
	var hdr wavHeader
	fmtFound := false
	offset := 12

	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])

		switch chunkID {
		case "fmt ":
			if offset+8+int(chunkSize) > len(data) {
				return wavHeader{}, 0, errors.New("wav: fmt chunk truncated")
			}
			fmtData := data[offset+8:]
			hdr.audioFormat = binary.LittleEndian.Uint16(fmtData[0:2])
			hdr.numChannels = binary.LittleEndian.Uint16(fmtData[2:4])
			hdr.sampleRate = binary.LittleEndian.Uint32(fmtData[4:8])
			hdr.bitsPerSample = binary.LittleEndian.Uint16(fmtData[14:16])
			fmtFound = true

		case "data":
			if !fmtFound {
				return wavHeader{}, 0, errors.New("wav: data chunk before fmt chunk")
			}
			hdr.dataSize = chunkSize
			if offset+8+int(chunkSize) > len(data) {
				// Truncated data chunk — use what's available.
				hdr.dataSize = uint32(len(data) - offset - 8)
			}
			return hdr, offset + 8, nil
		}

		// Advance to next chunk (chunks are 2-byte aligned).
		advance := 8 + int(chunkSize)
		if advance%2 != 0 {
			advance++
		}
		offset += advance
	}

	if !fmtFound {
		return wavHeader{}, 0, errors.New("wav: missing fmt chunk")
	}
	return wavHeader{}, 0, errors.New("wav: missing data chunk")
}

// convertToStereo16 converts raw PCM data to stereo signed 16-bit LE.
func convertToStereo16(data []byte, channels, bitsPerSample uint16) []byte {
	switch bitsPerSample {
	case 8:
		return convert8To16Stereo(data, channels)
	case 16:
		return convert16ToStereo(data, channels)
	default:
		// Unsupported bit depth — return empty slice.
		return []byte{}
	}
}

// convert8To16Stereo converts 8-bit unsigned PCM to 16-bit signed stereo.
func convert8To16Stereo(data []byte, channels uint16) []byte {
	sampleCount := len(data) / int(channels)
	out := make([]byte, sampleCount*4) // stereo 16-bit = 4 bytes per frame

	for i := 0; i < sampleCount; i++ {
		// Convert unsigned 8-bit [0,255] to signed 16-bit [-32768, 32767].
		sample := int16((int(data[i*int(channels)]) - 128) * 256)
		var right int16
		if channels == 2 {
			right = int16((int(data[i*int(channels)+1]) - 128) * 256)
		} else {
			right = sample
		}

		binary.LittleEndian.PutUint16(out[i*4:], uint16(sample))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(right))
	}
	return out
}

// convert16ToStereo converts 16-bit signed PCM to stereo if mono.
func convert16ToStereo(data []byte, channels uint16) []byte {
	if channels == 2 {
		// Already stereo 16-bit — return as-is.
		cp := make([]byte, len(data))
		copy(cp, data)
		return cp
	}

	// Mono to stereo: duplicate each sample.
	sampleCount := len(data) / 2
	out := make([]byte, sampleCount*4)
	for i := 0; i < sampleCount; i++ {
		sample := data[i*2 : i*2+2]
		copy(out[i*4:], sample)
		copy(out[i*4+2:], sample)
	}
	return out
}

// resample performs linear interpolation resampling from srcRate to dstRate.
// Input and output are stereo 16-bit signed LE PCM.
func resample(data []byte, srcRate, dstRate int) []byte {
	srcFrames := len(data) / 4 // 4 bytes per stereo frame
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

		// Left channel
		l0 := int16(binary.LittleEndian.Uint16(data[srcIdx*4:]))
		l1 := int16(binary.LittleEndian.Uint16(data[(srcIdx+1)*4:]))
		left := int16(float64(l0) + frac*(float64(l1)-float64(l0)))

		// Right channel
		r0 := int16(binary.LittleEndian.Uint16(data[srcIdx*4+2:]))
		r1 := int16(binary.LittleEndian.Uint16(data[(srcIdx+1)*4+2:]))
		right := int16(float64(r0) + frac*(float64(r1)-float64(r0)))

		binary.LittleEndian.PutUint16(out[i*4:], uint16(left))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(right))
	}

	return out
}
