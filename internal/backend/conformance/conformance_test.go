package conformance

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/backend/soft"
)

func newSoftDevice(t *testing.T) (*soft.Device, backend.CommandEncoder) {
	t.Helper()
	dev := soft.New()
	require.NoError(t, dev.Init(backend.DeviceConfig{Width: SceneSize, Height: SceneSize}))
	t.Cleanup(func() { dev.Dispose() })
	enc := dev.Encoder()
	return dev, enc
}

func TestConformanceSoft(t *testing.T) {
	dev, enc := newSoftDevice(t)
	RunAll(t, dev, enc)
}

func TestComparePixelsIdentical(t *testing.T) {
	pixels := make([]byte, 16)
	for i := range pixels {
		pixels[i] = 128
	}
	result := ComparePixels(pixels, pixels, 2, 2, 0)
	require.True(t, result.Match)
	require.Equal(t, 0, result.MaxDiff)
	require.Equal(t, 0, result.MismatchCount)
}

func TestComparePixelsWithinTolerance(t *testing.T) {
	a := []byte{100, 100, 100, 255, 200, 200, 200, 255}
	b := []byte{102, 100, 100, 255, 200, 200, 200, 255}
	result := ComparePixels(a, b, 2, 1, 3)
	require.True(t, result.Match)
	require.Equal(t, 2, result.MaxDiff)
}

func TestComparePixelsExceedsTolerance(t *testing.T) {
	a := []byte{100, 100, 100, 255}
	b := []byte{200, 100, 100, 255}
	result := ComparePixels(a, b, 1, 1, 3)
	require.False(t, result.Match)
	require.Equal(t, 100, result.MaxDiff)
	require.Equal(t, 1, result.MismatchCount)
}

func TestComparePixelsSizeMismatch(t *testing.T) {
	a := []byte{100, 100, 100, 255}
	b := []byte{100, 100, 100, 255, 0, 0, 0, 0}
	result := ComparePixels(a, b, 1, 1, 0)
	require.False(t, result.Match)
}

func TestScenesCount(t *testing.T) {
	scenes := Scenes()
	require.GreaterOrEqual(t, len(scenes), 10)
	for _, s := range scenes {
		require.NotEmpty(t, s.Name)
		require.NotEmpty(t, s.Description)
		require.NotNil(t, s.Render)
	}
}

func TestAbsDiff(t *testing.T) {
	require.Equal(t, 0, absDiff(100, 100))
	require.Equal(t, 50, absDiff(100, 150))
	require.Equal(t, 50, absDiff(150, 100))
}

func TestClampByte255(t *testing.T) {
	require.Equal(t, uint8(100), clampByte255(100))
	require.Equal(t, uint8(255), clampByte255(300))
}
