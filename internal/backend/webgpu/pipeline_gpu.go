//go:build wgpunative

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/wgpu"
)

// Pipeline implements backend.Pipeline for WebGPU.
// Stores the descriptor and lazily creates a WGPURenderPipeline.
type Pipeline struct {
	dev    *Device
	desc   backend.PipelineDescriptor
	handle wgpu.RenderPipeline
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
		// Infer stride from attributes.
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

	target := wgpu.ColorTargetState{
		Format:    wgpu.TextureFormatRGBA8Unorm,
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

	p.handle = wgpu.DeviceCreateRenderPipelineTyped(p.dev.device, &desc)
	runtime.KeepAlive(vertexEntry)
	runtime.KeepAlive(fragmentEntry)
	runtime.KeepAlive(attrs)
	runtime.KeepAlive(vbl)
	runtime.KeepAlive(blend)
	runtime.KeepAlive(target)
	runtime.KeepAlive(fragment)
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
}
