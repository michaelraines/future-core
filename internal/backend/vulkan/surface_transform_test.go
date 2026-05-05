//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/vk"
)

func TestSurfaceTransformDegrees(t *testing.T) {
	tests := []struct {
		name      string
		transform uint32
		want      int
	}{
		{"identity", vk.SurfaceTransformIdentityKHR, 0},
		{"rotate90", vk.SurfaceTransformRotate90KHR, 90},
		{"rotate180", vk.SurfaceTransformRotate180KHR, 180},
		{"rotate270", vk.SurfaceTransformRotate270KHR, 270},
		{"inherit", vk.SurfaceTransformInheritKHR, 0},
		{"zero", 0, 0},
		// HFLIP combinations and any unrecognized bit pattern: don't
		// silently pre-rotate. Engine takes the no-rotation path.
		{"unknown", 0x10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, surfaceTransformDegrees(tt.transform))
		})
	}
}
