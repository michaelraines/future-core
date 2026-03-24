// Package soft implements backend.Device as a pure Go software rasterizer.
//
// This backend is intended for headless testing and CI environments where
// no GPU is available. It does not render pixels — draw calls are recorded
// but not rasterized. It faithfully implements all Device, Texture, Buffer,
// Shader, Pipeline, RenderTarget, and CommandEncoder interfaces so that
// higher-level code (batcher, pipeline, engine) can be exercised without a
// GPU context.
package soft

import (
	"fmt"
	"sync/atomic"

	"github.com/michaelraines/future-render/internal/backend"
)

var nextID uint64

func genID() uint64 {
	return atomic.AddUint64(&nextID, 1)
}

// Device implements backend.Device for headless software rendering.
type Device struct {
	width, height int
	encoder       *Encoder
	screenRT      *RenderTarget
	inited        bool
}

// New creates a new software device.
func New() *Device {
	return &Device{}
}

// Init initializes the software device.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("soft: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	d.width = cfg.Width
	d.height = cfg.Height

	// Create an internal screen render target so the encoder can rasterize
	// into it when BeginRenderPass is called with Target == nil.
	rt, err := d.NewRenderTarget(backend.RenderTargetDescriptor{
		Width:       cfg.Width,
		Height:      cfg.Height,
		ColorFormat: backend.TextureFormatRGBA8,
	})
	if err != nil {
		return fmt.Errorf("soft: screen render target: %w", err)
	}
	d.screenRT = rt.(*RenderTarget)

	d.encoder = &Encoder{dev: d}
	d.inited = true
	return nil
}

// Dispose releases device resources.
func (d *Device) Dispose() {
	d.inited = false
}

// ReadScreen copies the rendered screen pixels into dst. Returns true if
// the screen render target has been initialized.
func (d *Device) ReadScreen(dst []byte) bool {
	if d.screenRT == nil {
		return false
	}
	if len(dst) > 0 {
		d.screenRT.color.ReadPixels(dst)
	}
	return true
}

// ScreenRenderTarget returns the internal render target used for screen
// rendering, or nil if Init has not been called.
func (d *Device) ScreenRenderTarget() *RenderTarget { return d.screenRT }

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {}

// EndFrame finalizes the frame.
func (d *Device) EndFrame() {}

// NewTexture creates a software texture backed by a byte slice.
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("soft: invalid texture dimensions %dx%d", desc.Width, desc.Height)
	}
	bpp := bytesPerPixel(desc.Format)
	size := desc.Width * desc.Height * bpp
	pixels := make([]byte, size)

	if desc.Data != nil {
		copy(pixels, desc.Data)
	} else if desc.Image != nil {
		copy(pixels, desc.Image.Pix)
	}

	return &Texture{
		id:     genID(),
		w:      desc.Width,
		h:      desc.Height,
		fmt:    desc.Format,
		filter: desc.Filter,
		pixels: pixels,
		bpp:    bpp,
	}, nil
}

// NewBuffer creates a software buffer backed by a byte slice.
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	if desc.Size <= 0 && len(desc.Data) == 0 {
		return nil, fmt.Errorf("soft: invalid buffer size %d", desc.Size)
	}
	size := desc.Size
	if size == 0 {
		size = len(desc.Data)
	}
	data := make([]byte, size)
	if desc.Data != nil {
		copy(data, desc.Data)
	}
	return &Buffer{
		id:   genID(),
		data: data,
	}, nil
}

// NewShader creates a software shader that stores uniform values.
func (d *Device) NewShader(_ backend.ShaderDescriptor) (backend.Shader, error) {
	return &Shader{
		id:       genID(),
		uniforms: make(map[string]any),
	}, nil
}

// NewRenderTarget creates a software render target with color (and optional
// depth) texture attachments.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	if desc.Width <= 0 || desc.Height <= 0 {
		return nil, fmt.Errorf("soft: invalid render target dimensions %dx%d", desc.Width, desc.Height)
	}
	colorTex, err := d.NewTexture(backend.TextureDescriptor{
		Width:        desc.Width,
		Height:       desc.Height,
		Format:       desc.ColorFormat,
		RenderTarget: true,
	})
	if err != nil {
		return nil, err
	}
	var depthTex backend.Texture
	if desc.HasDepth {
		depthTex, err = d.NewTexture(backend.TextureDescriptor{
			Width:        desc.Width,
			Height:       desc.Height,
			Format:       desc.DepthFormat,
			RenderTarget: true,
		})
		if err != nil {
			colorTex.Dispose()
			return nil, err
		}
	}
	return &RenderTarget{
		id:       genID(),
		color:    colorTex.(*Texture),
		depth:    depthTex,
		rtWidth:  desc.Width,
		rtHeight: desc.Height,
	}, nil
}

// NewPipeline creates a software pipeline state object.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &Pipeline{
		id:   genID(),
		desc: desc,
	}, nil
}

// Capabilities returns the software device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	return backend.DeviceCapabilities{
		MaxTextureSize:    8192,
		MaxRenderTargets:  8,
		SupportsInstanced: true,
		SupportsCompute:   false,
		SupportsMSAA:      false,
		MaxMSAASamples:    1,
		SupportsFloat16:   true,
	}
}

// Encoder returns the device's command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return d.encoder
}

// bytesPerPixel returns the number of bytes per pixel for a texture format.
func bytesPerPixel(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return 4
	case backend.TextureFormatRGB8:
		return 3
	case backend.TextureFormatR8:
		return 1
	case backend.TextureFormatRGBA16F:
		return 8
	case backend.TextureFormatRGBA32F:
		return 16
	case backend.TextureFormatDepth24:
		return 4
	case backend.TextureFormatDepth32F:
		return 4
	default:
		return 4
	}
}
