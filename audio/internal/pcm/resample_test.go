package pcm

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResampleIdentity(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint16(data[0:], 1000)
	binary.LittleEndian.PutUint16(data[2:], 2000)
	binary.LittleEndian.PutUint16(data[4:], 3000)
	binary.LittleEndian.PutUint16(data[6:], 4000)

	result := Resample(data, 44100, 44100)
	require.Equal(t, len(data), len(result))
}

func TestResampleTooShort(t *testing.T) {
	data := make([]byte, 4) // 1 frame, need at least 2
	result := Resample(data, 44100, 48000)
	require.Equal(t, data, result)
}

func TestResampleNilInput(t *testing.T) {
	result := Resample(nil, 44100, 48000)
	require.Nil(t, result)
}

func TestResampleDownsample(t *testing.T) {
	// 100 stereo frames at 44100, resample to 22050 (halve).
	data := make([]byte, 100*4)
	for i := 0; i < 100; i++ {
		binary.LittleEndian.PutUint16(data[i*4:], uint16(i*100))
		binary.LittleEndian.PutUint16(data[i*4+2:], uint16(i*100))
	}
	result := Resample(data, 44100, 22050)
	require.Greater(t, len(result), 0)
	resultFrames := len(result) / 4
	require.InDelta(t, 50, resultFrames, 5)
}

func TestResampleUpsample(t *testing.T) {
	// 100 stereo frames at 22050, resample to 44100 (double).
	data := make([]byte, 100*4)
	for i := 0; i < 100; i++ {
		binary.LittleEndian.PutUint16(data[i*4:], uint16(i*100))
		binary.LittleEndian.PutUint16(data[i*4+2:], uint16(i*100))
	}
	result := Resample(data, 22050, 44100)
	require.Greater(t, len(result), 0)
	resultFrames := len(result) / 4
	require.InDelta(t, 200, resultFrames, 5)
}

func TestResampleInterpolation(t *testing.T) {
	// 4 frames with known values, upsample 2x. Verify interpolated values.
	data := make([]byte, 4*4)
	for i := 0; i < 4; i++ {
		v := uint16(i * 10000)
		binary.LittleEndian.PutUint16(data[i*4:], v)
		binary.LittleEndian.PutUint16(data[i*4+2:], v)
	}
	result := Resample(data, 22050, 44100)
	require.Greater(t, len(result), 0)

	// Check that midpoint samples are interpolated between neighbors.
	frames := len(result) / 4
	require.GreaterOrEqual(t, frames, 6)

	v0 := int16(binary.LittleEndian.Uint16(result[0:]))
	v1 := int16(binary.LittleEndian.Uint16(result[4:]))
	// v1 should be between v0 and the second original sample (10000).
	require.GreaterOrEqual(t, v1, v0)
	require.LessOrEqual(t, v1, int16(10000))
}
