//go:build js && !soft

package webgpu

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Texture implements backend.Texture for WebGPU via the browser JS API.
type Texture struct {
	dev    *Device
	handle js.Value
	view   js.Value
	w, h   int
	format backend.TextureFormat
}

// InnerTexture returns nil for GPU textures (no soft delegation).
func (t *Texture) InnerTexture() backend.Texture { return nil }

// Upload replaces the entire texture data.
func (t *Texture) Upload(data []byte, mipLevel int) {
	if len(data) == 0 {
		return
	}

	dst := js.Global().Get("Object").New()
	dst.Set("texture", t.handle)
	dst.Set("mipLevel", mipLevel)

	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)

	bpp := bytesPerPixelJS(t.format)
	layout := js.Global().Get("Object").New()
	layout.Set("bytesPerRow", t.w*bpp)
	layout.Set("rowsPerImage", t.h)

	size := js.Global().Get("Object").New()
	size.Set("width", t.w)
	size.Set("height", t.h)

	t.dev.queue.Call("writeTexture", dst, arr, layout, size)
}

// UploadRegion uploads a sub-region of texture data.
func (t *Texture) UploadRegion(data []byte, x, y, w, h, mipLevel int) {
	if len(data) == 0 {
		return
	}

	dst := js.Global().Get("Object").New()
	dst.Set("texture", t.handle)
	dst.Set("mipLevel", mipLevel)
	origin := js.Global().Get("Array").New(x, y, 0)
	dst.Set("origin", origin)

	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)

	bpp := bytesPerPixelJS(t.format)
	layout := js.Global().Get("Object").New()
	layout.Set("bytesPerRow", w*bpp)
	layout.Set("rowsPerImage", h)

	size := js.Global().Get("Object").New()
	size.Set("width", w)
	size.Set("height", h)

	t.dev.queue.Call("writeTexture", dst, arr, layout, size)
}

// ReadPixels reads texture data back to CPU.
func (t *Texture) ReadPixels(dst []byte) {
	if len(dst) == 0 {
		return
	}

	bpp := bytesPerPixelJS(t.format)
	bytesPerRow := t.w * bpp
	alignedBytesPerRow := (bytesPerRow + 255) &^ 255
	totalSize := alignedBytesPerRow * t.h

	// Create staging buffer.
	bufDesc := js.Global().Get("Object").New()
	bufDesc.Set("size", totalSize)
	bufDesc.Set("usage",
		jsGPUBufferUsage(t.dev.device, "COPY_DST")|jsGPUBufferUsage(t.dev.device, "MAP_READ"))
	stagingBuf := t.dev.device.Call("createBuffer", bufDesc)

	// Encode copy.
	enc := t.dev.device.Call("createCommandEncoder")

	srcObj := js.Global().Get("Object").New()
	srcObj.Set("texture", t.handle)

	dstObj := js.Global().Get("Object").New()
	dstObj.Set("buffer", stagingBuf)
	dstObj.Set("bytesPerRow", alignedBytesPerRow)
	dstObj.Set("rowsPerImage", t.h)

	sizeObj := js.Global().Get("Object").New()
	sizeObj.Set("width", t.w)
	sizeObj.Set("height", t.h)

	enc.Call("copyTextureToBuffer", srcObj, dstObj, sizeObj)
	cmdBuf := enc.Call("finish")
	t.dev.queue.Call("submit", js.Global().Get("Array").New(cmdBuf))

	// Map and read.
	mapPromise := stagingBuf.Call("mapAsync", jsGPUMapMode(t.dev.device, "READ"))
	if _, err := awaitPromise(mapPromise); err != nil {
		return
	}

	mapped := stagingBuf.Call("getMappedRange")
	srcArr := js.Global().Get("Uint8Array").New(mapped)

	dstOffset := 0
	for row := 0; row < t.h; row++ {
		srcStart := alignedBytesPerRow * row
		n := bytesPerRow
		if dstOffset+n > len(dst) {
			n = len(dst) - dstOffset
		}
		if n <= 0 {
			break
		}
		rowSlice := srcArr.Call("slice", srcStart, srcStart+n)
		js.CopyBytesToGo(dst[dstOffset:dstOffset+n], rowSlice)
		dstOffset += n
	}

	stagingBuf.Call("unmap")
	stagingBuf.Call("destroy")
}

// Width returns the texture width.
func (t *Texture) Width() int { return t.w }

// Height returns the texture height.
func (t *Texture) Height() int { return t.h }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.format }

// Dispose releases the texture.
func (t *Texture) Dispose() {
	if !t.handle.IsUndefined() && !t.handle.IsNull() {
		t.handle.Call("destroy")
	}
}

// bytesPerPixelJS returns the bytes per pixel for a texture format.
func bytesPerPixelJS(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatR8:
		return 1
	case backend.TextureFormatRGB8:
		return 3
	case backend.TextureFormatRGBA8:
		return 4
	case backend.TextureFormatRGBA16F:
		return 8
	case backend.TextureFormatRGBA32F:
		return 16
	default:
		return 4
	}
}
