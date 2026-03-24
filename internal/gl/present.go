//go:build darwin || linux || freebsd || windows

package gl

import "unsafe"

// Presenter blits CPU pixel data to the OpenGL default framebuffer.
// Used by backends that render to their own surface (Metal, Vulkan, etc.)
// rather than directly to the GL context.
type Presenter struct {
	Tex    uint32 // GL texture for pixel upload
	Fbo    uint32 // FBO with tex attached (used as READ_FRAMEBUFFER for blit)
	Width  int
	Height int
	Buf    []byte // CPU pixel buffer (width * height * 4)
}

// InitPresenter creates a new Presenter with GL resources.
func InitPresenter(w, h int) *Presenter {
	p := &Presenter{
		Width:  w,
		Height: h,
		Buf:    make([]byte, w*h*4),
	}

	GenTextures(1, &p.Tex)
	BindTexture(TEXTURE_2D, p.Tex)
	TexImage2D(TEXTURE_2D, 0, RGBA, int32(w), int32(h), 0, RGBA, UNSIGNED_BYTE, nil)
	TexParameteri(TEXTURE_2D, TEXTURE_MIN_FILTER, NEAREST)
	TexParameteri(TEXTURE_2D, TEXTURE_MAG_FILTER, NEAREST)
	BindTexture(TEXTURE_2D, 0)

	GenFramebuffers(1, &p.Fbo)
	BindFramebuffer(READ_FRAMEBUFFER, p.Fbo)
	FramebufferTexture2D(READ_FRAMEBUFFER, COLOR_ATTACHMENT0, TEXTURE_2D, p.Tex, 0)
	BindFramebuffer(READ_FRAMEBUFFER, 0)

	return p
}

// Present uploads pixels to the GL texture and blits to the default
// framebuffer. srcW/srcH are the rendered image dimensions; dstW/dstH
// are the physical framebuffer dimensions (may differ on HiDPI).
func (p *Presenter) Present(pixels []byte, srcW, srcH, dstW, dstH int) {
	BindTexture(TEXTURE_2D, p.Tex)
	TexSubImage2D(TEXTURE_2D, 0, 0, 0, int32(srcW), int32(srcH),
		RGBA, UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	BindTexture(TEXTURE_2D, 0)

	// Blit from our FBO (read) to default framebuffer (draw).
	// Flip Y: most backends use top-left origin, GL uses bottom-left.
	BindFramebuffer(READ_FRAMEBUFFER, p.Fbo)
	BindFramebuffer(DRAW_FRAMEBUFFER, 0)
	BlitFramebuffer(
		0, int32(srcH), int32(srcW), 0,
		0, 0, int32(dstW), int32(dstH),
		COLOR_BUFFER_BIT, NEAREST,
	)
	BindFramebuffer(READ_FRAMEBUFFER, 0)
}

// Resize recreates the texture if the screen dimensions changed.
func (p *Presenter) Resize(w, h int) {
	if w == p.Width && h == p.Height {
		return
	}
	p.Width = w
	p.Height = h
	p.Buf = make([]byte, w*h*4)

	BindTexture(TEXTURE_2D, p.Tex)
	TexImage2D(TEXTURE_2D, 0, RGBA, int32(w), int32(h), 0, RGBA, UNSIGNED_BYTE, nil)
	BindTexture(TEXTURE_2D, 0)
}

// Dispose releases GL resources.
func (p *Presenter) Dispose() {
	if p.Fbo != 0 {
		DeleteFramebuffers(1, &p.Fbo)
		p.Fbo = 0
	}
	if p.Tex != 0 {
		DeleteTextures(1, &p.Tex)
		p.Tex = 0
	}
}
