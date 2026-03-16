package vorbis

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- Stream method tests (construct directly) ---

func TestStreamRead(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	s := &Stream{
		data:       bytes.NewReader(data),
		raw:        data,
		sampleRate: 44100,
		length:     4,
	}

	buf := make([]byte, 4)
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, data, buf)
}

func TestStreamSeek(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	s := &Stream{
		data:       bytes.NewReader(data),
		raw:        data,
		sampleRate: 48000,
		length:     8,
	}

	// Read 4 bytes.
	buf := make([]byte, 4)
	_, err := s.Read(buf)
	require.NoError(t, err)

	// Seek back to start.
	pos, err := s.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(0), pos)

	// Read should return first bytes again.
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte{1, 2, 3, 4}, buf)
}

func TestStreamLength(t *testing.T) {
	s := &Stream{
		data:       bytes.NewReader(make([]byte, 100)),
		raw:        make([]byte, 100),
		sampleRate: 44100,
		length:     100,
	}
	require.Equal(t, int64(100), s.Length())
}

func TestStreamSampleRate(t *testing.T) {
	s := &Stream{
		data:       bytes.NewReader(nil),
		raw:        nil,
		sampleRate: 22050,
		length:     0,
	}
	require.Equal(t, 22050, s.SampleRate())
}

// --- mock float32 reader ---

type mockF32Reader struct {
	data []float32
	pos  int
}

func (m *mockF32Reader) Read(p []float32) (int, error) {
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}
	n := copy(p, m.data[m.pos:])
	m.pos += n
	if m.pos >= len(m.data) {
		return n, io.EOF
	}
	return n, nil
}

type errorF32Reader struct{}

func (e *errorF32Reader) Read(_ []float32) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

// --- readAllSamples tests ---

func TestReadAllSamples(t *testing.T) {
	data := []float32{0.1, 0.2, 0.3, 0.4}
	r := &mockF32Reader{data: data}
	result, err := readAllSamples(r)
	require.NoError(t, err)
	require.Equal(t, data, result)
}

func TestReadAllSamplesEmpty(t *testing.T) {
	r := &mockF32Reader{data: nil}
	result, err := readAllSamples(r)
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestReadAllSamplesError(t *testing.T) {
	r := &errorF32Reader{}
	_, err := readAllSamples(r)
	require.Error(t, err)
}

// --- newStream tests ---

func TestNewStream(t *testing.T) {
	samples := []float32{0.5, -0.5, 0.25, -0.25}
	s := newStream(samples, 2, 44100)

	require.Equal(t, 44100, s.SampleRate())
	require.Equal(t, int64(8), s.Length()) // 2 frames × 4 bytes
	require.NotNil(t, s.raw)

	buf := make([]byte, s.Length())
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, int(s.Length()), n)
}

func TestNewStreamMono(t *testing.T) {
	samples := []float32{0.5, -0.5}
	s := newStream(samples, 1, 48000)

	require.Equal(t, 48000, s.SampleRate())
	require.Equal(t, int64(8), s.Length()) // 2 mono → 2 stereo frames
}

// --- Decode error tests ---

func TestDecodeInvalidData(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte("not valid ogg data")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "vorbis")
}

func TestDecodeEmptyInput(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte{}))
	require.Error(t, err)
}

func TestDecodeWithSampleRateInvalid(t *testing.T) {
	_, err := DecodeWithSampleRate(48000, bytes.NewReader([]byte("bad")))
	require.Error(t, err)
}

// --- Conversion function tests ---

func TestClampFloat32(t *testing.T) {
	tests := []struct {
		name string
		in   float32
		want float32
	}{
		{"within range", 0.5, 0.5},
		{"at max", 1.0, 1.0},
		{"at min", -1.0, -1.0},
		{"above max", 1.5, 1.0},
		{"below min", -1.5, -1.0},
		{"zero", 0.0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.InDelta(t, tt.want, clampFloat32(tt.in), 1e-6)
		})
	}
}

func TestFloatToInt16(t *testing.T) {
	tests := []struct {
		name string
		in   float32
		want int16
	}{
		{"silence", 0.0, 0},
		{"max", 1.0, math.MaxInt16},
		{"min", -1.0, -math.MaxInt16},
		{"half", 0.5, math.MaxInt16 / 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, floatToInt16(tt.in))
		})
	}
}

func TestFloatToStereo16Mono(t *testing.T) {
	samples := []float32{0.5, -0.5}
	result := floatToStereo16(samples, 1)

	require.Equal(t, 8, len(result))

	l0 := int16(binary.LittleEndian.Uint16(result[0:]))
	r0 := int16(binary.LittleEndian.Uint16(result[2:]))
	l1 := int16(binary.LittleEndian.Uint16(result[4:]))
	r1 := int16(binary.LittleEndian.Uint16(result[6:]))

	require.Equal(t, l0, r0)
	require.Equal(t, l1, r1)
	require.Greater(t, l0, int16(0))
	require.Less(t, l1, int16(0))
}

func TestFloatToStereo16Stereo(t *testing.T) {
	samples := []float32{0.5, -0.5}
	result := floatToStereo16(samples, 2)

	require.Equal(t, 4, len(result))

	l := int16(binary.LittleEndian.Uint16(result[0:]))
	r := int16(binary.LittleEndian.Uint16(result[2:]))

	require.Greater(t, l, int16(0))
	require.Less(t, r, int16(0))
}
