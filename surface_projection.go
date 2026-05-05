package futurerender

import (
	gomath "math"

	fmath "github.com/michaelraines/future-core/math"
)

// buildAndroidProjection returns the orthographic projection for a
// logical screen of the given dimensions, composed with a Z-rotation
// that compensates for the device's surface pre-rotation. rotationDeg
// is the VkSurfaceTransform value (0/90/180/270); other values are
// treated as 0.
//
// On Android with a portrait-natural panel and a landscape-locked
// Activity, the Vulkan swapchain is created with
// PreTransform=ROTATE_90 and an extent in the natural orientation.
// The pre-rotation matrix maps logical-landscape content into that
// swapped image; the GPU's PreTransform then rotates it back for
// display, and the user sees the original content right-side up.
//
// Direction: the rotation is applied as Mat4RotateZ(-rotationDeg).
// VK_SURFACE_TRANSFORM_ROTATE_90_BIT_KHR means "image content is
// presented as if the image was rotated 90° clockwise relative to
// the device's natural orientation." To compensate, the engine
// pre-rotates clip-space content by the same magnitude in the
// opposite (CCW-in-the-device's-natural-frame) direction. After the
// Vulkan backend's Y-flip on upload, this works out to a negative
// Mat4RotateZ angle in our Y-up engine clip space — matching the
// Khronos vulkan_pre_rotation sample's "rotate around -Z by N°"
// convention. Verified empirically: +rotationDeg displays content
// rotated 180° on the Galaxy S25 (doubles the existing offset);
// -rotationDeg cancels it.
//
// Composition order matters: uProjection = rotation × ortho. Vertices
// transform in the shader as `clip = uProjection * pos`, so logical
// coords pass through ortho first, then through the rotation.
//
// Lives in an untagged file (rather than engine_android.go) so it can
// be unit-tested on any host — the math is platform-independent and
// the Android driver is the only caller in production.
func buildAndroidProjection(screenW, screenH, rotationDeg int) fmath.Mat4 {
	ortho := fmath.Mat4Ortho(0, float64(screenW), float64(screenH), 0, -1, 1)
	switch rotationDeg {
	case 90, 180, 270:
		rad := float64(-rotationDeg) * gomath.Pi / 180
		return fmath.Mat4RotateZ(rad).Mul(ortho)
	default:
		return ortho
	}
}
