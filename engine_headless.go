//go:build darwin || linux || freebsd || windows

package futurerender

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"unsafe"

	"github.com/michaelraines/future-core/internal/gl"
)

// headlessConfig holds screenshot-and-exit configuration from env vars.
// When enabled, the engine renders N frames through the full windowed
// pipeline, captures the screen, saves to PNG, and exits cleanly.
//
// Environment variables:
//
//	FUTURE_CORE_HEADLESS=N            Capture after N frames and exit.
//	FUTURE_CORE_HEADLESS_OUTPUT       Output PNG path (default: headless_output.png).
//	FUTURE_CORE_HEADLESS_WINDOW=1     Capture the window framebuffer via
//	                                  glReadPixels instead of the backend's
//	                                  internal RT via device.ReadScreen.
//	                                  Includes the GL-presenter blit (and
//	                                  any swapchain colorspace / format
//	                                  conversion) that users actually see
//	                                  on-screen. Use this to diagnose bugs
//	                                  that exist only in the presentation
//	                                  step (e.g. Vulkan → GL blit color
//	                                  shifts, aspect-ratio stretching).
type headlessConfig struct {
	frames        int
	output        string
	windowCapture bool
}

// getHeadlessConfig reads headless configuration from the environment.
// Returns nil if headless mode is not enabled.
func getHeadlessConfig() *headlessConfig {
	v := os.Getenv("FUTURE_CORE_HEADLESS")
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return nil
	}
	output := os.Getenv("FUTURE_CORE_HEADLESS_OUTPUT")
	if output == "" {
		output = "headless_output.png"
	}
	windowCapture := os.Getenv("FUTURE_CORE_HEADLESS_WINDOW") == "1"
	return &headlessConfig{frames: n, output: output, windowCapture: windowCapture}
}

// saveScreenshot reads pixels and writes a PNG. Two capture modes:
//
//   - Device mode (default): `device.ReadScreen` copies the backend's
//     final offscreen color buffer before it's handed to the
//     presenter. Fast, bypasses any presenter/swapchain quirks — but
//     therefore can't see bugs that exist only in the presentation
//     step.
//
//   - Window mode (FUTURE_CORE_HEADLESS_WINDOW=1): glReadPixels on
//     the GL default framebuffer after the presenter has blit the
//     backend's output to it. This captures exactly what the user sees
//     in the window, including Vulkan → GL RGBA→BGRA conversions,
//     aspect-ratio stretching by the presenter quad, viewport
//     mismatches, and any color-space drift. Required for diagnosing
//     bugs that were invisible in device-mode captures but visible on
//     the live window.
func (e *engine) saveScreenshot(width, height int, path string, windowCapture bool) error {
	pixels := make([]byte, width*height*4)

	readFromWindow := windowCapture && !e.noGL
	readFromDevice := !readFromWindow

	if readFromDevice {
		if !e.device.ReadScreen(pixels) {
			if e.noGL {
				return fmt.Errorf("headless: backend does not support ReadScreen and GL is not available")
			}
			// Fall back to glReadPixels for backends like OpenGL that
			// render directly to the window framebuffer.
			readFromWindow = true
		}
	}

	if readFromWindow {
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
