package wav

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildWAV creates a minimal valid WAV file in memory.
func buildWAV(channels, bitsPerSample uint16, sampleRate uint32, data []byte) []byte {
	var buf bytes.Buffer

	dataSize := uint32(len(data))
	fileSize := 36 + dataSize

	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, fileSize)
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // chunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM format
	_ = binary.Write(&buf, binary.LittleEndian, channels)
	_ = binary.Write(&buf, binary.LittleEndian, sampleRate)
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	_ = binary.Write(&buf, binary.LittleEndian, byteRate)
	blockAlign := channels * bitsPerSample / 8
	_ = binary.Write(&buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, bitsPerSample)

	// data chunk
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)
	buf.Write(data)

	return buf.Bytes()
}

func TestDecodeStereo16(t *testing.T) {
	// 2 sample frames of stereo 16-bit: [L=100, R=200, L=300, R=400]
	pcm := make([]byte, 8)
	binary.LittleEndian.PutUint16(pcm[0:], uint16(100))
	binary.LittleEndian.PutUint16(pcm[2:], uint16(200))
	binary.LittleEndian.PutUint16(pcm[4:], uint16(300))
	binary.LittleEndian.PutUint16(pcm[6:], uint16(400))

	wav := buildWAV(2, 16, 44100, pcm)

	stream, err := Decode(bytes.NewReader(wav))
	require.NoError(t, err)
	require.Equal(t, 44100, stream.SampleRate())
	require.Equal(t, int64(8), stream.Length())

	out := make([]byte, 8)
	n, err := io.ReadFull(stream, out)
	require.NoError(t, err)
	require.Equal(t, 8, n)
	require.Equal(t, pcm, out)
}

func TestDecodeMono16(t *testing.T) {
	// 2 mono samples.
	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:], uint16(1000))
	binary.LittleEndian.PutUint16(pcm[2:], uint16(2000))

	wav := buildWAV(1, 16, 48000, pcm)

	stream, err := Decode(bytes.NewReader(wav))
	require.NoError(t, err)
	require.Equal(t, 48000, stream.SampleRate())
	// Mono→stereo doubles the size.
	require.Equal(t, int64(8), stream.Length())

	out := make([]byte, 8)
	_, err = io.ReadFull(stream, out)
	require.NoError(t, err)

	// Each mono sample should be duplicated to both channels.
	l0 := binary.LittleEndian.Uint16(out[0:])
	r0 := binary.LittleEndian.Uint16(out[2:])
	l1 := binary.LittleEndian.Uint16(out[4:])
	r1 := binary.LittleEndian.Uint16(out[6:])
	require.Equal(t, uint16(1000), l0)
	require.Equal(t, uint16(1000), r0)
	require.Equal(t, uint16(2000), l1)
	require.Equal(t, uint16(2000), r1)
}

func TestDecode8Bit(t *testing.T) {
	// 2 mono 8-bit samples: 128 (silence), 255 (max positive).
	pcm := []byte{128, 255}
	wav := buildWAV(1, 8, 22050, pcm)

	stream, err := Decode(bytes.NewReader(wav))
	require.NoError(t, err)
	require.Equal(t, 22050, stream.SampleRate())

	out := make([]byte, stream.Length())
	_, err = io.ReadFull(stream, out)
	require.NoError(t, err)

	// 128 → 0 (silence in 16-bit signed).
	l0 := int16(binary.LittleEndian.Uint16(out[0:]))
	require.Equal(t, int16(0), l0)

	// 255 → (255-128)*256 = 32512 (near max positive).
	l1 := int16(binary.LittleEndian.Uint16(out[4:]))
	require.Equal(t, int16(32512), l1)
}

func TestDecode8BitStereo(t *testing.T) {
	// 1 frame: L=0 (min), R=255 (max).
	pcm := []byte{0, 255}
	wav := buildWAV(2, 8, 44100, pcm)

	stream, err := Decode(bytes.NewReader(wav))
	require.NoError(t, err)

	out := make([]byte, stream.Length())
	_, err = io.ReadFull(stream, out)
	require.NoError(t, err)

	l := int16(binary.LittleEndian.Uint16(out[0:]))
	r := int16(binary.LittleEndian.Uint16(out[2:]))
	require.Equal(t, int16(-32768), l) // 0 - 128 = -128, * 256 = -32768
	require.Equal(t, int16(32512), r)  // 255 - 128 = 127, * 256 = 32512
}

func TestDecodeWithSampleRate(t *testing.T) {
	// Generate 100 frames of stereo 16-bit at 44100 Hz.
	pcm := make([]byte, 100*4)
	for i := 0; i < 100; i++ {
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(i*100))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(i*100))
	}
	wav := buildWAV(2, 16, 44100, pcm)

	stream, err := DecodeWithSampleRate(22050, bytes.NewReader(wav))
	require.NoError(t, err)
	require.Equal(t, 22050, stream.SampleRate())
	// Downsampled to roughly half the frames.
	require.Greater(t, stream.Length(), int64(0))
}

func TestDecodeWithSampleRateSameRate(t *testing.T) {
	pcm := make([]byte, 8)
	wav := buildWAV(2, 16, 48000, pcm)

	stream, err := DecodeWithSampleRate(48000, bytes.NewReader(wav))
	require.NoError(t, err)
	require.Equal(t, 48000, stream.SampleRate())
	require.Equal(t, int64(8), stream.Length())
}

func TestStreamSeek(t *testing.T) {
	pcm := make([]byte, 16)
	wav := buildWAV(2, 16, 44100, pcm)

	stream, err := Decode(bytes.NewReader(wav))
	require.NoError(t, err)

	// Read 4 bytes.
	buf := make([]byte, 4)
	_, err = io.ReadFull(stream, buf)
	require.NoError(t, err)

	// Seek back to start.
	pos, err := stream.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(0), pos)
}

func TestDecodeInvalidRIFF(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte("NOT A WAV FILE")))
	require.Error(t, err)
}

func TestDecodeEmptyInput(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte{}))
	require.Error(t, err)
}

func TestDecodeMissingWAVE(t *testing.T) {
	data := []byte("RIFF\x00\x00\x00\x00NOPE")
	_, err := Decode(bytes.NewReader(data))
	require.Error(t, err)
}

func TestDecodeNonPCMFormat(t *testing.T) {
	// Build a WAV with audioFormat=2 (ADPCM, unsupported).
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2)) // non-PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(44100))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(88200))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))

	_, err := Decode(bytes.NewReader(buf.Bytes()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported audio format")
}

func TestConvertToStereo16UnsupportedBitDepth(t *testing.T) {
	result := convertToStereo16([]byte{1, 2, 3}, 1, 24)
	require.Empty(t, result)
}
