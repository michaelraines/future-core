//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-render/internal/backend"
)

// Texture implements backend.Texture for WebGL2.
type Texture struct {
	gl     js.Value
	handle js.Value
	w, h   int
	format backend.TextureFormat
}

// InnerTexture returns nil for GPU textures (no soft delegation).
func (t *Texture) InnerTexture() backend.Texture { return nil }

// Upload replaces the entire texture data.
func (t *Texture) Upload(data []byte, mipLevel int) {
	tex2D := t.gl.Get("TEXTURE_2D").Int()
	t.gl.Call("bindTexture", tex2D, t.handle)

	internalFmt := glInternalFormat(t.gl, t.format)
	baseFmt := glBaseFormat(t.gl, t.format)
	pixelType := glPixelType(t.gl, t.format)

	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	t.gl.Call("texImage2D", tex2D, mipLevel, internalFmt,
		t.w, t.h, 0, baseFmt, pixelType, arr)

	t.gl.Call("bindTexture", tex2D, js.Null())
}

// UploadRegion uploads a sub-region of texture data.
func (t *Texture) UploadRegion(data []byte, x, y, w, h, mipLevel int) {
	tex2D := t.gl.Get("TEXTURE_2D").Int()
	t.gl.Call("bindTexture", tex2D, t.handle)

	baseFmt := glBaseFormat(t.gl, t.format)
	pixelType := glPixelType(t.gl, t.format)

	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	t.gl.Call("texSubImage2D", tex2D, mipLevel, x, y, w, h, baseFmt, pixelType, arr)

	t.gl.Call("bindTexture", tex2D, js.Null())
}

// ReadPixels reads texture data back to CPU.
func (t *Texture) ReadPixels(dst []byte) {
	// Create a temporary framebuffer to read from.
	fbo := t.gl.Call("createFramebuffer")
	fbTarget := t.gl.Get("FRAMEBUFFER").Int()
	t.gl.Call("bindFramebuffer", fbTarget, fbo)
	t.gl.Call("framebufferTexture2D", fbTarget,
		t.gl.Get("COLOR_ATTACHMENT0").Int(),
		t.gl.Get("TEXTURE_2D").Int(), t.handle, 0)

	arr := js.Global().Get("Uint8Array").New(len(dst))
	t.gl.Call("readPixels", 0, 0, t.w, t.h,
		t.gl.Get("RGBA").Int(), t.gl.Get("UNSIGNED_BYTE").Int(), arr)
	js.CopyBytesToGo(dst, arr)

	t.gl.Call("bindFramebuffer", fbTarget, js.Null())
	t.gl.Call("deleteFramebuffer", fbo)
}

// Width returns the texture width.
func (t *Texture) Width() int { return t.w }

// Height returns the texture height.
func (t *Texture) Height() int { return t.h }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.format }

// Dispose releases the texture.
func (t *Texture) Dispose() {
	t.gl.Call("deleteTexture", t.handle)
}
