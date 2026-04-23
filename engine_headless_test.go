//go:build darwin || linux || freebsd || windows

package futurerender

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetHeadlessConfigDisabled(t *testing.T) {
	t.Setenv("FUTURE_CORE_HEADLESS", "")
	require.Nil(t, getHeadlessConfig())
}

func TestGetHeadlessConfigInvalidValue(t *testing.T) {
	t.Setenv("FUTURE_CORE_HEADLESS", "abc")
	require.Nil(t, getHeadlessConfig())
}

func TestGetHeadlessConfigNegativeValue(t *testing.T) {
	t.Setenv("FUTURE_CORE_HEADLESS", "-1")
	require.Nil(t, getHeadlessConfig())
}

func TestGetHeadlessConfigZero(t *testing.T) {
	t.Setenv("FUTURE_CORE_HEADLESS", "0")
	require.Nil(t, getHeadlessConfig())
}

func TestGetHeadlessConfigValid(t *testing.T) {
	t.Setenv("FUTURE_CORE_HEADLESS", "60")
	t.Setenv("FUTURE_CORE_HEADLESS_OUTPUT", "")

	cfg := getHeadlessConfig()
	require.NotNil(t, cfg)
	require.Equal(t, 60, cfg.frames)
	require.Equal(t, "headless_output.png", cfg.output)
}

func TestGetHeadlessConfigCustomOutput(t *testing.T) {
	t.Setenv("FUTURE_CORE_HEADLESS", "30")
	t.Setenv("FUTURE_CORE_HEADLESS_OUTPUT", "/tmp/custom.png")

	cfg := getHeadlessConfig()
	require.NotNil(t, cfg)
	require.Equal(t, 30, cfg.frames)
	require.Equal(t, "/tmp/custom.png", cfg.output)
}

func TestSaveScreenshotReadScreen(t *testing.T) {
	dev := &mockDevice{
		readScreenFn: func(pixels []byte) bool {
			// Fill with a solid red pixel pattern.
			for i := 0; i < len(pixels); i += 4 {
				pixels[i] = 255   // R
				pixels[i+1] = 0   // G
				pixels[i+2] = 0   // B
				pixels[i+3] = 255 // A
			}
			return true
		},
	}
	e := &engine{device: dev}

	path := filepath.Join(t.TempDir(), "test_screenshot.png")
	err := e.saveScreenshot(2, 2, path, false)
	require.NoError(t, err)

	// Verify the PNG was written and is valid.
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck // test file, best-effort close

	img, err := png.Decode(f)
	require.NoError(t, err)
	require.Equal(t, 2, img.Bounds().Dx())
	require.Equal(t, 2, img.Bounds().Dy())
}

func TestSaveScreenshotReadScreenFails(t *testing.T) {
	dev := &mockDevice{
		readScreenFn: func(_ []byte) bool {
			return false
		},
	}
	e := &engine{device: dev, noGL: true}

	path := filepath.Join(t.TempDir(), "test_screenshot.png")
	err := e.saveScreenshot(2, 2, path, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support ReadScreen")
}

func TestSaveScreenshotInvalidPath(t *testing.T) {
	dev := &mockDevice{
		readScreenFn: func(pixels []byte) bool {
			for i := range pixels {
				pixels[i] = 0
			}
			return true
		},
	}
	e := &engine{device: dev}

	err := e.saveScreenshot(2, 2, "/nonexistent/dir/test.png", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create")
}
