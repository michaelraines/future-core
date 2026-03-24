//go:build darwin || linux || freebsd || windows

package futurerender

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"unsafe"

	"github.com/michaelraines/future-render/internal/gl"
)

// headlessConfig holds screenshot-and-exit configuration from env vars.
// When enabled, the engine renders N frames through the full windowed
// pipeline, captures the screen, saves to PNG, and exits cleanly.
//
// Environment variables:
//
//	FUTURE_RENDER_HEADLESS=N       Capture after N frames and exit.
//	FUTURE_RENDER_HEADLESS_OUTPUT  Output PNG path (default: headless_output.png).
type headlessConfig struct {
	frames int
	output string
}

// getHeadlessConfig reads headless configuration from the environment.
// Returns nil if headless mode is not enabled.
func getHeadlessConfig() *headlessConfig {
	v := os.Getenv("FUTURE_RENDER_HEADLESS")
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return nil
	}
	output := os.Getenv("FUTURE_RENDER_HEADLESS_OUTPUT")
	if output == "" {
		output = "headless_output.png"
	}
	return &headlessConfig{frames: n, output: output}
}

// saveScreenshot reads pixels from the device and writes a PNG.
func (e *engine) saveScreenshot(width, height int, path string) error {
	pixels := make([]byte, width*height*4)

	// Try ReadScreen first (works for soft, vulkan, metal, etc.).
	if !e.device.ReadScreen(pixels) {
		// OpenGL renders directly to the window framebuffer.
		// Read pixels via glReadPixels from the default framebuffer.
		gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE,
			unsafe.Pointer(&pixels[0]))

		// glReadPixels returns bottom-up; flip vertically.
		stride := width * 4
		row := make([]byte, stride)
		for y := 0; y < height/2; y++ {
			top := y * stride
			bot := (height - 1 - y) * stride
			copy(row, pixels[top:top+stride])
			copy(pixels[top:top+stride], pixels[bot:bot+stride])
			copy(pixels[bot:bot+stride], row)
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, pixels)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("headless: create %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on screenshot file

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("headless: encode png: %w", err)
	}

	return nil
}
