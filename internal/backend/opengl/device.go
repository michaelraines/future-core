//go:build darwin || linux || freebsd || windows

// Package opengl implements the backend.Device interface using OpenGL 3.3 core.
package opengl

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/gl"
)

// Device implements backend.Device for OpenGL 3.3 core profile.
type Device struct {
	width, height int
	encoder       *commandEncoder
}

// New creates a new OpenGL device (uninitialized — call Init after GL context is current).
func New() *Device {
	return &Device{}
}

// Init initializes OpenGL. Must be called after the GL context is current.
func (d *Device) Init(cfg backend.DeviceConfig) error {
	if err := gl.Init(); err != nil {
		return fmt.Errorf("opengl init: %w", err)
	}

	version := gl.GetGoString(gl.VERSION)
	if version == "" {
		return fmt.Errorf("opengl: could not query GL version")
	}

	d.width = cfg.Width
	d.height = cfg.Height
	d.encoder = &commandEncoder{}

	// Standard 2D defaults.
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	return nil
}

// Dispose releases device resources.
func (d *Device) Dispose() {}

// BeginFrame prepares for a new frame.
func (d *Device) BeginFrame() {}

// EndFrame finalizes the frame.
func (d *Device) EndFrame() {}

// NewTexture creates a new OpenGL texture.
func (d *Device) NewTexture(desc backend.TextureDescriptor) (backend.Texture, error) {
	var id uint32
	gl.GenTextures(1, &id)
	gl.BindTexture(gl.TEXTURE_2D, id)

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, wrapToGL(desc.WrapU))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, wrapToGL(desc.WrapV))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filterToGL(desc.Filter))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filterToGL(desc.Filter))

	internalFormat, format, typ := formatToGL(desc.Format)

	var dataPtr unsafe.Pointer
	if desc.Data != nil {
		dataPtr = gl.Ptr(desc.Data)
	} else if desc.Image != nil {
		dataPtr = gl.Ptr(desc.Image.Pix)
	}

	gl.TexImage2D(gl.TEXTURE_2D, 0, internalFormat,
		int32(desc.Width), int32(desc.Height), 0,
		format, typ, dataPtr)

	gl.BindTexture(gl.TEXTURE_2D, 0)

	return &texture{
		id:     id,
		width:  desc.Width,
		height: desc.Height,
		format: desc.Format,
	}, nil
}

// NewBuffer creates a new OpenGL buffer.
func (d *Device) NewBuffer(desc backend.BufferDescriptor) (backend.Buffer, error) {
	var id uint32
	gl.GenBuffers(1, &id)

	target := usageToGL(desc.Usage)
	gl.BindBuffer(target, id)

	usage := uint32(gl.STATIC_DRAW)
	if desc.Dynamic {
		usage = gl.DYNAMIC_DRAW
	}

	if desc.Data != nil {
		gl.BufferData(target, len(desc.Data), gl.Ptr(desc.Data), usage)
	} else {
		gl.BufferData(target, desc.Size, nil, usage)
	}

	gl.BindBuffer(target, 0)

	return &buffer{
		id:    id,
		size:  desc.Size,
		usage: desc.Usage,
	}, nil
}

// NewShader compiles and links an OpenGL shader program.
func (d *Device) NewShader(desc backend.ShaderDescriptor) (backend.Shader, error) {
	vs, err := compileShader(desc.VertexSource, gl.VERTEX_SHADER)
	if err != nil {
		return nil, fmt.Errorf("vertex shader: %w", err)
	}
	fs, err := compileShader(desc.FragmentSource, gl.FRAGMENT_SHADER)
	if err != nil {
		gl.DeleteShader(vs)
		return nil, fmt.Errorf("fragment shader: %w", err)
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vs)
	gl.AttachShader(program, fs)
	gl.LinkProgram(program)

	// Shaders can be deleted after linking.
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLen)
		logBuf := make([]byte, logLen+1)
		gl.GetProgramInfoLog(program, logLen, nil, &logBuf[0])
		gl.DeleteProgram(program)
		return nil, fmt.Errorf("link: %s", strings.TrimRight(string(logBuf), "\x00"))
	}

	return &shader{program: program}, nil
}

// NewRenderTarget creates an OpenGL framebuffer object.
func (d *Device) NewRenderTarget(desc backend.RenderTargetDescriptor) (backend.RenderTarget, error) {
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)

	// Color attachment.
	colorTex, err := d.NewTexture(backend.TextureDescriptor{
		Width:        desc.Width,
		Height:       desc.Height,
		Format:       desc.ColorFormat,
		Filter:       backend.FilterLinear,
		RenderTarget: true,
	})
	if err != nil {
		gl.DeleteFramebuffers(1, &fbo)
		return nil, err
	}
	ct := colorTex.(*texture)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, ct.id, 0)

	var dt *texture
	if desc.HasDepth {
		depthTex, derr := d.NewTexture(backend.TextureDescriptor{
			Width:        desc.Width,
			Height:       desc.Height,
			Format:       desc.DepthFormat,
			Filter:       backend.FilterNearest,
			RenderTarget: true,
		})
		if derr != nil {
			colorTex.Dispose()
			gl.DeleteFramebuffers(1, &fbo)
			return nil, derr
		}
		dt = depthTex.(*texture)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.TEXTURE_2D, dt.id, 0)
	}

	status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	if status != gl.FRAMEBUFFER_COMPLETE {
		colorTex.Dispose()
		if dt != nil {
			dt.Dispose()
		}
		gl.DeleteFramebuffers(1, &fbo)
		return nil, fmt.Errorf("framebuffer incomplete: status %d", status)
	}

	return &renderTarget{
		fbo:      fbo,
		color:    ct,
		depth:    dt,
		rtWidth:  desc.Width,
		rtHeight: desc.Height,
	}, nil
}

// NewPipeline creates a render pipeline state.
func (d *Device) NewPipeline(desc backend.PipelineDescriptor) (backend.Pipeline, error) {
	return &pipelineState{desc: desc}, nil
}

// Capabilities returns the device capabilities.
func (d *Device) Capabilities() backend.DeviceCapabilities {
	var maxTexSize int32
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxTexSize)
	return backend.DeviceCapabilities{
		MaxTextureSize:    int(maxTexSize),
		MaxRenderTargets:  8,
		SupportsInstanced: true,
		SupportsMSAA:      true,
		MaxMSAASamples:    4,
	}
}

// Encoder returns the device's command encoder.
func (d *Device) Encoder() backend.CommandEncoder {
	return d.encoder
}

// --- Helper types ---

type texture struct {
	id     uint32
	width  int
	height int
	format backend.TextureFormat
}

func (t *texture) Upload(data []byte, level int) {
	gl.BindTexture(gl.TEXTURE_2D, t.id)
	_, format, typ := formatToGL(t.format)
	gl.TexSubImage2D(gl.TEXTURE_2D, int32(level), 0, 0,
		int32(t.width), int32(t.height), format, typ, gl.Ptr(data))
	gl.BindTexture(gl.TEXTURE_2D, 0)
}

func (t *texture) UploadRegion(data []byte, x, y, width, height, level int) {
	gl.BindTexture(gl.TEXTURE_2D, t.id)
	_, format, typ := formatToGL(t.format)
	gl.TexSubImage2D(gl.TEXTURE_2D, int32(level), int32(x), int32(y),
		int32(width), int32(height), format, typ, gl.Ptr(data))
	gl.BindTexture(gl.TEXTURE_2D, 0)
}

func (t *texture) ReadPixels(dst []byte) {
	gl.BindTexture(gl.TEXTURE_2D, t.id)
	_, format, typ := formatToGL(t.format)
	gl.GetTexImage(gl.TEXTURE_2D, 0, format, typ, gl.Ptr(dst))
	gl.BindTexture(gl.TEXTURE_2D, 0)
}

func (t *texture) Width() int                    { return t.width }
func (t *texture) Height() int                   { return t.height }
func (t *texture) Format() backend.TextureFormat { return t.format }
func (t *texture) Dispose()                      { gl.DeleteTextures(1, &t.id) }

type buffer struct {
	id    uint32
	size  int
	usage backend.BufferUsage
}

func (b *buffer) Upload(data []byte) {
	target := usageToGL(b.usage)
	gl.BindBuffer(target, b.id)
	gl.BufferSubData(target, 0, len(data), gl.Ptr(data))
	gl.BindBuffer(target, 0)
}

func (b *buffer) UploadRegion(data []byte, offset int) {
	target := usageToGL(b.usage)
	gl.BindBuffer(target, b.id)
	gl.BufferSubData(target, offset, len(data), gl.Ptr(data))
	gl.BindBuffer(target, 0)
}

func (b *buffer) Size() int { return b.size }
func (b *buffer) Dispose()  { gl.DeleteBuffers(1, &b.id) }

type shader struct {
	program uint32
}

func (s *shader) SetUniformFloat(name string, v float32) {
	loc := gl.GetUniformLocation(s.program, gl.Str(name+"\x00"))
	gl.ProgramUniform1f(s.program, loc, v)
}

func (s *shader) SetUniformVec2(name string, v [2]float32) {
	loc := gl.GetUniformLocation(s.program, gl.Str(name+"\x00"))
	gl.ProgramUniform2fv(s.program, loc, 1, &v[0])
}

func (s *shader) SetUniformVec4(name string, v [4]float32) {
	loc := gl.GetUniformLocation(s.program, gl.Str(name+"\x00"))
	gl.ProgramUniform4fv(s.program, loc, 1, &v[0])
}

func (s *shader) SetUniformMat4(name string, v [16]float32) {
	loc := gl.GetUniformLocation(s.program, gl.Str(name+"\x00"))
	gl.ProgramUniformMatrix4fv(s.program, loc, 1, false, &v[0])
}

func (s *shader) SetUniformInt(name string, v int32) {
	loc := gl.GetUniformLocation(s.program, gl.Str(name+"\x00"))
	gl.ProgramUniform1i(s.program, loc, v)
}

func (s *shader) SetUniformBlock(_ string, _ []byte) {
	// UBO binding — not needed for M1, stubbed.
}

func (s *shader) Dispose() { gl.DeleteProgram(s.program) }

type renderTarget struct {
	fbo      uint32
	color    *texture
	depth    *texture
	rtWidth  int
	rtHeight int
}

func (rt *renderTarget) ColorTexture() backend.Texture {
	return rt.color
}

func (rt *renderTarget) DepthTexture() backend.Texture {
	if rt.depth == nil {
		return nil
	}
	return rt.depth
}

func (rt *renderTarget) Width() int  { return rt.rtWidth }
func (rt *renderTarget) Height() int { return rt.rtHeight }

func (rt *renderTarget) Dispose() {
	gl.DeleteFramebuffers(1, &rt.fbo)
	rt.color.Dispose()
	if rt.depth != nil {
		rt.depth.Dispose()
	}
}

type pipelineState struct {
	desc backend.PipelineDescriptor
}

func (p *pipelineState) Dispose() {}

// --- Shader compilation ---

func compileShader(source string, shaderType uint32) (uint32, error) {
	s := gl.CreateShader(shaderType)
	csource := gl.Str(source + "\x00")
	gl.ShaderSource(s, 1, &csource, nil)
	runtime.KeepAlive(csource) // prevent GC of backing []byte before ShaderSource completes
	gl.CompileShader(s)

	var status int32
	gl.GetShaderiv(s, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(s, gl.INFO_LOG_LENGTH, &logLen)
		logBuf := make([]byte, logLen+1)
		gl.GetShaderInfoLog(s, logLen, nil, &logBuf[0])
		gl.DeleteShader(s)
		return 0, fmt.Errorf("%s", strings.TrimRight(string(logBuf), "\x00"))
	}
	return s, nil
}

// --- GL enum conversions ---

func filterToGL(f backend.TextureFilter) int32 {
	switch f {
	case backend.FilterLinear:
		return gl.LINEAR
	default:
		return gl.NEAREST
	}
}

func wrapToGL(w backend.TextureWrap) int32 {
	switch w {
	case backend.WrapRepeat:
		return gl.REPEAT
	case backend.WrapMirror:
		return gl.MIRRORED_REPEAT
	default:
		return gl.CLAMP_TO_EDGE
	}
}

func formatToGL(f backend.TextureFormat) (internalFmt int32, format, typ uint32) {
	switch f {
	case backend.TextureFormatRGBA8:
		return gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE
	case backend.TextureFormatRGB8:
		return gl.RGB8, gl.RGB, gl.UNSIGNED_BYTE
	case backend.TextureFormatR8:
		return gl.R8, gl.RED, gl.UNSIGNED_BYTE
	case backend.TextureFormatRGBA16F:
		return gl.RGBA16F, gl.RGBA, gl.FLOAT
	case backend.TextureFormatRGBA32F:
		return gl.RGBA32F, gl.RGBA, gl.FLOAT
	case backend.TextureFormatDepth24:
		return gl.DEPTH_COMPONENT24, gl.DEPTH_COMPONENT, gl.UNSIGNED_INT
	case backend.TextureFormatDepth32F:
		return gl.DEPTH_COMPONENT32F, gl.DEPTH_COMPONENT, gl.FLOAT
	default:
		return gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE
	}
}

func usageToGL(u backend.BufferUsage) uint32 {
	switch u {
	case backend.BufferUsageIndex:
		return gl.ELEMENT_ARRAY_BUFFER
	default:
		return gl.ARRAY_BUFFER
	}
}
