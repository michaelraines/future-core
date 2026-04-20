package webgl

// ContextAttributes mirrors WebGL2 context creation attributes. The type is
// shared between the soft-delegating build (device.go, //go:build !js)
// and the browser build (device_js.go, //go:build js), so it lives in an
// untagged file.
type ContextAttributes struct {
	Alpha                 bool
	Depth                 bool
	Stencil               bool
	Antialias             bool
	PremultipliedAlpha    bool
	PreserveDrawingBuffer bool
	PowerPreference       string // "default", "high-performance", "low-power"
}

// DefaultContextAttributes returns sensible defaults for WebGL2. Stencil
// is requested by default so the canvas framebuffer carries a stencil
// buffer; the sprite pass routes fill-rule batches through stencil when
// the device reports SupportsStencil=true and the target has stencil.
func DefaultContextAttributes() ContextAttributes {
	return ContextAttributes{
		Alpha:              true,
		Depth:              true,
		Stencil:            true,
		Antialias:          true,
		PremultipliedAlpha: true,
		PowerPreference:    "default",
	}
}
