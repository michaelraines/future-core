//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
)

// TestLavapipeDetection guards against silent ICD misconfig in the
// Docker-based Vulkan CI path. The container's Dockerfile sets
// FUTURE_CORE_EXPECT_LAVAPIPE=1 and installs only Mesa's lavapipe ICD;
// if Mesa changes the ICD JSON layout or the image picks up a different
// driver (e.g. swiftshader), the conformance tests would still run but
// against the wrong driver — and its goldens would collide with
// lavapipe's.
//
// The test skips when the env var isn't set so Mac dev-loop runs
// against MoltenVK stay green. A dedicated env var, rather than
// inspecting VK_ICD_FILENAMES, keeps the gate independent of
// loader-path discovery — the Dockerfile leaves VK_ICD_FILENAMES unset
// so the loader auto-discovers the per-arch lavapipe JSON.
func TestLavapipeDetection(t *testing.T) {
	if os.Getenv("FUTURE_CORE_EXPECT_LAVAPIPE") != "1" {
		t.Skip("FUTURE_CORE_EXPECT_LAVAPIPE not set; skipping lavapipe-specific check (expected outside the Docker CI image)")
	}

	dev := New()
	require.NoError(t, dev.Init(backend.DeviceConfig{Width: 64, Height: 64}))
	t.Cleanup(func() { dev.Dispose() })

	// DeviceName is a fixed-size [256]byte null-terminated C string.
	// Trim at the first NUL before comparing.
	name := dev.devProps.DeviceName[:]
	if i := bytes.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	gotName := string(name)
	t.Logf("Vulkan physical device: %q", gotName)
	require.Contains(t, strings.ToLower(gotName), "llvmpipe",
		"expected lavapipe (llvmpipe) as physical device; got %q — check installed drivers in the container", gotName)
}
