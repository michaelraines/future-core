package mp3

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- Stream method tests (construct directly via bytes.Reader) ---

func TestStreamRead(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	s := &Stream{
		reader:     bytes.NewReader(data),
		sampleRate: 44100,
		length:     4,
	}

	buf := make([]byte, 4)
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, data, buf)
}

func TestStreamReadEOF(t *testing.T) {
	data := []byte{1, 2}
	s := &Stream{
		reader:     bytes.NewReader(data),
		sampleRate: 44100,
		length:     2,
	}

	buf := make([]byte, 4)
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = s.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 0, n)
}

func TestStreamSeek(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	s := &Stream{
		reader:     bytes.NewReader(data),
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

func TestStreamSeekCurrent(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	s := &Stream{
		reader:     bytes.NewReader(data),
		sampleRate: 44100,
		length:     8,
	}

	// Read 2 bytes to advance position.
	buf := make([]byte, 2)
	_, err := s.Read(buf)
	require.NoError(t, err)

	// Seek forward 2 from current.
	pos, err := s.Seek(2, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, int64(4), pos)

	// Read should return bytes at offset 4.
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, []byte{5, 6}, buf)
}

func TestStreamSeekEnd(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	s := &Stream{
		reader:     bytes.NewReader(data),
		sampleRate: 44100,
		length:     8,
	}

	// Seek to 4 bytes before end.
	pos, err := s.Seek(-4, io.SeekEnd)
	require.NoError(t, err)
	require.Equal(t, int64(4), pos)

	buf := make([]byte, 4)
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte{5, 6, 7, 8}, buf)
}

func TestStreamLength(t *testing.T) {
	s := &Stream{
		reader:     bytes.NewReader(make([]byte, 100)),
		sampleRate: 44100,
		length:     100,
	}
	require.Equal(t, int64(100), s.Length())
}

func TestStreamSampleRate(t *testing.T) {
	s := &Stream{
		reader:     bytes.NewReader(nil),
		sampleRate: 22050,
		length:     0,
	}
	require.Equal(t, 22050, s.SampleRate())
}

// --- readAll tests ---

func TestReadAll(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	result, err := readAll(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, data, result)
}

func TestReadAllEmpty(t *testing.T) {
	result, err := readAll(bytes.NewReader(nil))
	require.NoError(t, err)
	require.Nil(t, result)
}

type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestReadAllError(t *testing.T) {
	_, err := readAll(&errorReader{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mp3")
}

func TestReadAllLargeData(t *testing.T) {
	// Larger than internal buffer size (8192).
	data := make([]byte, 20000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	result, err := readAll(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, data, result)
}

// --- Decode error tests ---

func TestDecodeInvalidData(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte("not valid mp3 data")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "mp3")
}

func TestDecodeEmptyInput(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte{}))
	require.Error(t, err)
}

func TestDecodeWithSampleRateInvalid(t *testing.T) {
	_, err := DecodeWithSampleRate(48000, bytes.NewReader([]byte("bad")))
	require.Error(t, err)
}

// --- buildMP3Frame builds a minimal valid MPEG1 Layer 3 frame ---
//
// MPEG1 Layer 3, 128kbps, 44100Hz, Joint Stereo.
// Frame header: 0xFFFB9004
//   - sync: 0xFFF (12 bits)
//   - version: 1 (MPEG1), layer: 01 (Layer 3), protection: 1 (no CRC)
//   - bitrate: 1001 (128kbps), sample rate: 00 (44100Hz), padding: 0
//   - channel: 01 (Joint Stereo), mode ext: 00, other: 00
//
// Frame size = 144 * 128000 / 44100 = 417 bytes.
func buildMP3Frame() []byte {
	frame := make([]byte, 417)
	// Frame header: MPEG1, Layer 3, no CRC, 128kbps, 44100Hz, Joint Stereo.
	frame[0] = 0xFF
	frame[1] = 0xFB // sync + MPEG1 + Layer3 + no CRC
	frame[2] = 0x90 // 128kbps + 44100Hz + no padding
	frame[3] = 0x04 // Joint Stereo + mode ext 0 + not copyrighted + original
	// Side info (32 bytes for stereo MPEG1) — all zeros is valid (silence).
	// Main data — all zeros produces silence.
	return frame
}

// buildMP3 builds a minimal MP3 byte buffer with the given number of frames.
func buildMP3(frames int) []byte {
	frame := buildMP3Frame()
	var buf bytes.Buffer
	for range frames {
		buf.Write(frame)
	}
	return buf.Bytes()
}

func TestDecodeValidMP3Seekable(t *testing.T) {
	// bytes.Reader implements io.Seeker, so this exercises the streaming path.
	mp3Data := buildMP3(3)
	s, err := Decode(bytes.NewReader(mp3Data))
	require.NoError(t, err)
	require.Equal(t, 44100, s.SampleRate())
	require.Greater(t, s.Length(), int64(0))

	// Output should be stereo 16-bit LE (4 bytes per frame).
	require.Equal(t, int64(0), s.Length()%4)
}

// nonSeekReader wraps an io.Reader to remove Seek capability.
type nonSeekReader struct {
	r io.Reader
}

func (n *nonSeekReader) Read(p []byte) (int, error) {
	return n.r.Read(p)
}

func TestDecodeValidMP3NonSeekable(t *testing.T) {
	// Non-seekable source exercises the buffered path.
	mp3Data := buildMP3(3)
	s, err := Decode(&nonSeekReader{r: bytes.NewReader(mp3Data)})
	require.NoError(t, err)
	require.Equal(t, 44100, s.SampleRate())
	require.Greater(t, s.Length(), int64(0))
	require.Equal(t, int64(0), s.Length()%4)
}

func TestDecodeWithSampleRateSame(t *testing.T) {
	mp3Data := buildMP3(3)
	s, err := DecodeWithSampleRate(44100, bytes.NewReader(mp3Data))
	require.NoError(t, err)
	require.Equal(t, 44100, s.SampleRate())
}

func TestDecodeWithSampleRateResample(t *testing.T) {
	mp3Data := buildMP3(3)
	s, err := DecodeWithSampleRate(22050, bytes.NewReader(mp3Data))
	require.NoError(t, err)
	require.Equal(t, 22050, s.SampleRate())
	require.Greater(t, s.Length(), int64(0))
}

func TestDecodeStreamSeek(t *testing.T) {
	mp3Data := buildMP3(3)
	s, err := Decode(bytes.NewReader(mp3Data))
	require.NoError(t, err)

	// Read some data.
	buf := make([]byte, 16)
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 16, n)

	// Seek back to start.
	pos, err := s.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(0), pos)

	// Read again — should get same data.
	buf2 := make([]byte, 16)
	n, err = s.Read(buf2)
	require.NoError(t, err)
	require.Equal(t, 16, n)
	require.Equal(t, buf, buf2)
}
