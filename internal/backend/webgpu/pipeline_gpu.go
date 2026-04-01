//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/wgpu"
)

// Pipeline implements backend.Pipeline for WebGPU.
// Stores the descriptor and lazily creates a WGPURenderPipeline.
type Pipeline struct {
	dev    *Device
	desc   backend.PipelineDescriptor
	handle wgpu.RenderPipeline

	// Cached bind group layouts for this pipeline.
	uniformBGL wgpu.BindGroupLayout // group 0: uniforms
	textureBGL wgpu.BindGroupLayout // group 1: texture + sampler
	layout     wgpu.PipelineLayout
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// createPipeline lazily compiles the shader and creates the render pipeline.
func (p *Pipeline) createPipeline() {
	if p.handle != 0 || p.dev.device == 0 {
		return
	}

	shader, ok := p.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return
	}

	shader.compile()
	if shader.vertexModule == 0 || shader.fragmentModule == 0 {
		return
	}

	// Create bind group layouts.
	p.createBindGroupLayouts()
	if p.layout == 0 {
		return
	}

	vertexEntry := cstr("vs_main")
	fragmentEntry := cstr("fs_main")

	// Build vertex attributes from pipeline vertex format.
	var attrs []wgpu.VertexAttribute
	stride := uint64(p.desc.VertexFormat.Stride)
	for i, a := range p.desc.VertexFormat.Attributes {
		vf := wgpuVertexFormat(a.Format)
		attrs = append(attrs, wgpu.VertexAttribute{
			Format:         vf,
			Offset:         uint64(a.Offset),
			ShaderLocation: uint32(i),
		})
	}
	if stride == 0 {
		for _, a := range p.desc.VertexFormat.Attributes {
			end := uint64(a.Offset) + vertexFormatSize(a.Format)
			if end > stride {
				stride = end
			}
		}
	}

	var buffersPtr uintptr
	var bufferCount uint32
	var vbl wgpu.VertexBufferLayout
	if len(attrs) > 0 {
		vbl = wgpu.VertexBufferLayout{
			ArrayStride:    stride,
			StepMode:       wgpu.VertexStepModeVertex,
			AttributeCount: uint32(len(attrs)),
			Attributes:     uintptr(unsafe.Pointer(&attrs[0])),
		}
		buffersPtr = uintptr(unsafe.Pointer(&vbl))
		bufferCount = 1
	}

	// Configure blend state.
	blendEnabled, blend := wgpuBlendState(p.desc.BlendMode)

	// Use the surface format when rendering to the surface, otherwise RGBA8Unorm.
	targetFormat := wgpu.TextureFormatRGBA8Unorm
	if p.dev.hasSurface {
		targetFormat = p.dev.surfaceFormat
	}
	target := wgpu.ColorTargetState{
		Format:    targetFormat,
		WriteMask: wgpu.ColorWriteMaskAll,
	}
	if blendEnabled {
		target.Blend = uintptr(unsafe.Pointer(&blend))
	}

	fragment := wgpu.FragmentState{
		Module:      shader.fragmentModule,
		EntryPoint:  uintptr(unsafe.Pointer(fragmentEntry)),
		TargetCount: 1,
		Targets:     uintptr(unsafe.Pointer(&target)),
	}

	desc := wgpu.RenderPipelineDescriptor{
		Layout: p.layout,
		Vertex: wgpu.VertexState{
			Module:      shader.vertexModule,
			EntryPoint:  uintptr(unsafe.Pointer(vertexEntry)),
			BufferCount: bufferCount,
			Buffers:     buffersPtr,
		},
		Primitive: wgpu.PrimitiveState{
			Topology:   wgpuTopology(p.desc.Primitive),
			FrontFace_: wgpu.FrontFaceCCW,
			CullMode_:  wgpuCullMode(p.desc.CullMode),
		},
		Multisample: wgpu.MultisampleState{
			Count: 1,
			Mask:  0xFFFFFFFF,
		},
		Fragment: uintptr(unsafe.Pointer(&fragment)),
	}

	// Add depth/stencil state if depth testing is enabled.
	var depthStencil wgpu.DepthStencilState
	if p.desc.DepthTest {
		depthStencil = wgpu.DepthStencilState{
			Format:            wgpu.TextureFormatDepth24Plus,
			DepthWriteEnabled: boolToUint32(p.desc.DepthWrite),
			DepthCompare:      wgpuCompareFunc(p.desc.DepthFunc),
			StencilFront: wgpu.StencilFaceState{
				Compare: wgpu.CompareFunctionAlways,
			},
			StencilBack: wgpu.StencilFaceState{
				Compare: wgpu.CompareFunctionAlways,
			},
			StencilReadMask:  0xFF,
			StencilWriteMask: 0xFF,
		}
		desc.DepthStencil = uintptr(unsafe.Pointer(&depthStencil))
	}

	p.handle = wgpu.DeviceCreateRenderPipelineTyped(p.dev.device, &desc)
	runtime.KeepAlive(vertexEntry)
	runtime.KeepAlive(fragmentEntry)
	runtime.KeepAlive(attrs)
	runtime.KeepAlive(vbl)
	runtime.KeepAlive(blend)
	runtime.KeepAlive(target)
	runtime.KeepAlive(fragment)
	runtime.KeepAlive(depthStencil)
}

// createBindGroupLayouts creates the bind group layouts and pipeline layout.
func (p *Pipeline) createBindGroupLayouts() {
	if p.dev.device == 0 {
		return
	}

	// Group 0: Uniform buffer (vertex + fragment visibility).
	uniformEntries := []wgpu.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: 1 | 2, // Vertex | Fragment
			Buffer_: wgpu.BindGroupLayoutEntryBuffer{
				Type:           1, // Uniform
				MinBindingSize: 0,
			},
		},
	}
	uniformBGLDesc := wgpu.BindGroupLayoutDescriptor{
		EntryCount: uint32(len(uniformEntries)),
		Entries:    uintptr(unsafe.Pointer(&uniformEntries[0])),
	}
	p.uniformBGL = wgpu.DeviceCreateBindGroupLayout(p.dev.device, &uniformBGLDesc)
	runtime.KeepAlive(uniformEntries)

	// Group 1: Texture + sampler (fragment visibility).
	textureEntries := []wgpu.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: 2, // Fragment
			Texture_: wgpu.BindGroupLayoutEntryTexture{
				SampleType:    1, // Float
				ViewDimension: 2, // 2D
			},
		},
		{
			Binding:    1,
			Visibility: 2, // Fragment
			Sampler_: wgpu.BindGroupLayoutEntrySampler{
				Type: 1, // Filtering
			},
		},
	}
	textureBGLDesc := wgpu.BindGroupLayoutDescriptor{
		EntryCount: uint32(len(textureEntries)),
		Entries:    uintptr(unsafe.Pointer(&textureEntries[0])),
	}
	p.textureBGL = wgpu.DeviceCreateBindGroupLayout(p.dev.device, &textureBGLDesc)
	runtime.KeepAlive(textureEntries)

	// Pipeline layout with both groups.
	bgls := []wgpu.BindGroupLayout{p.uniformBGL, p.textureBGL}
	plDesc := wgpu.PipelineLayoutDescriptor{
		BindGroupLayoutCount: uint32(len(bgls)),
		BindGroupLayouts:     uintptr(unsafe.Pointer(&bgls[0])),
	}
	p.layout = wgpu.DeviceCreatePipelineLayout(p.dev.device, &plDesc)
	runtime.KeepAlive(bgls)
}

// wgpuCompareFunc maps backend CompareFunc to wgpu CompareFunction.
func wgpuCompareFunc(cf backend.CompareFunc) wgpu.CompareFunction {
	switch cf {
	case backend.CompareNever:
		return wgpu.CompareFunctionNever
	case backend.CompareLess:
		return wgpu.CompareFunctionLess
	case backend.CompareLessEqual:
		return wgpu.CompareFunctionLessEqual
	case backend.CompareEqual:
		return wgpu.CompareFunctionEqual
	case backend.CompareGreaterEqual:
		return wgpu.CompareFunctionGreaterEqual
	case backend.CompareGreater:
		return wgpu.CompareFunctionGreater
	case backend.CompareNotEqual:
		return wgpu.CompareFunctionNotEqual
	case backend.CompareAlways:
		return wgpu.CompareFunctionAlways
	default:
		return wgpu.CompareFunctionAlways
	}
}

// boolToUint32 converts a bool to a C-compatible uint32 (0 or 1).
func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// wgpuBlendState returns blend configuration for a backend blend mode.
func wgpuBlendState(mode backend.BlendMode) (enabled bool, state wgpu.BlendState) {
	switch mode {
	case backend.BlendSourceOver:
		return true, wgpu.BlendState{
			Color: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorSrcAlpha,
				DstFactor: wgpu.BlendFactorOneMinusSrcAlpha,
			},
			Alpha: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorOne,
				DstFactor: wgpu.BlendFactorOneMinusSrcAlpha,
			},
		}
	case backend.BlendAdditive:
		return true, wgpu.BlendState{
			Color: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorSrcAlpha,
				DstFactor: wgpu.BlendFactorOne,
			},
			Alpha: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorOne,
				DstFactor: wgpu.BlendFactorOne,
			},
		}
	case backend.BlendMultiplicative:
		return true, wgpu.BlendState{
			Color: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorDst,
				DstFactor: wgpu.BlendFactorZero,
			},
			Alpha: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorDstAlpha,
				DstFactor: wgpu.BlendFactorZero,
			},
		}
	case backend.BlendPremultiplied:
		return true, wgpu.BlendState{
			Color: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorOne,
				DstFactor: wgpu.BlendFactorOneMinusSrcAlpha,
			},
			Alpha: wgpu.BlendComponent{
				Operation: wgpu.BlendOperationAdd,
				SrcFactor: wgpu.BlendFactorOne,
				DstFactor: wgpu.BlendFactorOneMinusSrcAlpha,
			},
		}
	default:
		return false, wgpu.BlendState{}
	}
}

// wgpuTopology maps backend primitive type to WebGPU topology.
func wgpuTopology(p backend.PrimitiveType) wgpu.PrimitiveTopology {
	switch p {
	case backend.PrimitiveTriangles:
		return wgpu.PrimitiveTopologyTriangleList
	case backend.PrimitiveTriangleStrip:
		return wgpu.PrimitiveTopologyTriangleStrip
	case backend.PrimitiveLines:
		return wgpu.PrimitiveTopologyLineList
	case backend.PrimitiveLineStrip:
		return wgpu.PrimitiveTopologyLineStrip
	case backend.PrimitivePoints:
		return wgpu.PrimitiveTopologyPointList
	default:
		return wgpu.PrimitiveTopologyTriangleList
	}
}

// wgpuCullMode maps backend cull mode to WebGPU cull mode.
func wgpuCullMode(mode backend.CullMode) wgpu.CullMode {
	switch mode {
	case backend.CullFront:
		return wgpu.CullModeFront
	case backend.CullBack:
		return wgpu.CullModeBack
	default:
		return wgpu.CullModeNone
	}
}

// wgpuVertexFormat maps backend attribute format to WebGPU vertex format.
func wgpuVertexFormat(f backend.AttributeFormat) wgpu.VertexFormat {
	switch f {
	case backend.AttributeFloat2:
		return wgpu.VertexFormatFloat32x2
	case backend.AttributeFloat3:
		return wgpu.VertexFormatFloat32x3
	case backend.AttributeFloat4:
		return wgpu.VertexFormatFloat32x4
	case backend.AttributeByte4Norm:
		return wgpu.VertexFormatUnorm8x4
	default:
		return wgpu.VertexFormatFloat32x4
	}
}

// vertexFormatSize returns the byte size of a vertex attribute format.
func vertexFormatSize(f backend.AttributeFormat) uint64 {
	return uint64(backend.AttributeFormatSize(f))
}

// cstr converts a Go string to a null-terminated C string.
func cstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// Dispose releases pipeline resources.
func (p *Pipeline) Dispose() {
	if p.handle != 0 {
		wgpu.RenderPipelineRelease(p.handle)
		p.handle = 0
	}
	if p.layout != 0 {
		wgpu.PipelineLayoutRelease(p.layout)
		p.layout = 0
	}
	if p.uniformBGL != 0 {
		wgpu.BindGroupLayoutRelease(p.uniformBGL)
		p.uniformBGL = 0
	}
	if p.textureBGL != 0 {
		wgpu.BindGroupLayoutRelease(p.textureBGL)
		p.textureBGL = 0
	}
}
