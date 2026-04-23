//go:build (darwin || linux || freebsd || windows) && !soft

package webgpu

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/wgpu"
)

// pipelineVariantKey identifies a cached WGPURenderPipeline by the two
// pieces of state WebGPU bakes in: the color attachment format and the
// blend mode. SetBlendMode on this backend used to be a no-op, which
// meant every pipeline shipped with whatever blend was in the
// descriptor (typically BlendSourceOver). Callers that asked for a
// different blend at draw time — iso-combat's multiply lightmap
// composite, the lighting demo's additive light stamps and
// blendAddModAlpha shadow passes — silently fell back to SourceOver,
// which for a multiply blend means "replace scene with lightmap"
// instead of "darken/brighten scene by lightmap". Symptom: white
// background where the scene should be dimmed, lightmap contents
// bleeding through at full opacity. Caching per (format, blend)
// variant lets SetBlendMode drive a pipeline swap.
type pipelineVariantKey struct {
	format wgpu.TextureFormat
	blend  backend.BlendMode
}

// Pipeline implements backend.Pipeline for WebGPU.
// Stores the descriptor and lazily creates WGPURenderPipelines keyed
// by (target format, blend mode) — see pipelineVariantKey.
type Pipeline struct {
	dev  *Device
	desc backend.PipelineDescriptor

	// Per-(format, blend) cache. Populated lazily by
	// ensurePipelineForVariant. The encoder picks the key based on
	// its current target format + the blend SetBlendMode recorded.
	pipelines map[pipelineVariantKey]wgpu.RenderPipeline

	// handle + createdFormat track the LAST-activated variant so
	// ensurePipelineForFormat (legacy API — no blend arg) can keep
	// working for callers that haven't migrated yet. Everything on
	// the encoder hot path now goes through ensurePipelineForVariant.
	handle        wgpu.RenderPipeline
	createdFormat wgpu.TextureFormat

	// Cached bind group layouts for this pipeline.
	uniformBGL wgpu.BindGroupLayout // group 0: uniforms
	textureBGL wgpu.BindGroupLayout // group 1: texture + sampler
	layout     wgpu.PipelineLayout
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// ensurePipelineForVariant returns a cached pipeline for the given
// (format, blend) pair, creating it lazily if needed. Encoder's
// SetPipeline path calls this with the encoder's current target
// format AND the blend SetBlendMode recorded — so a multiply-blit
// composite on a swapchain pass and the SAME shader's additive blend
// on an offscreen RT both end up with the correct pipeline.
func (p *Pipeline) ensurePipelineForVariant(format wgpu.TextureFormat, blend backend.BlendMode) wgpu.RenderPipeline {
	if p.pipelines == nil {
		p.pipelines = make(map[pipelineVariantKey]wgpu.RenderPipeline)
	}
	key := pipelineVariantKey{format: format, blend: blend}
	if h := p.pipelines[key]; h != 0 {
		p.handle = h
		p.createdFormat = format
		return h
	}
	// Stash the format + blend so createPipeline reads them. Old
	// createPipeline shape is preserved for minimal churn; it still
	// writes into p.handle + p.createdFormat + p.layout/bind-group
	// layouts the FIRST time. Subsequent variants reuse the layout
	// and just add a new handle to the map.
	p.createdFormat = format
	p.createPipelineForBlend(blend)
	if p.handle != 0 {
		p.pipelines[key] = p.handle
	}
	return p.handle
}

// ensurePipelineForFormat is the legacy entry point — equivalent to
// asking for the pipeline-descriptor's default blend. Kept so any
// callers that haven't migrated keep working.
func (p *Pipeline) ensurePipelineForFormat(format wgpu.TextureFormat) {
	p.ensurePipelineForVariant(format, p.desc.BlendMode)
}

// createPipelineForBlend lazily compiles the shader and creates a render
// pipeline for the given blend mode. The resulting handle is written to
// p.handle; the caller (ensurePipelineForVariant) is responsible for
// caching it into the per-variant map. Bind group layouts are shared
// across variants and created on the first invocation.
func (p *Pipeline) createPipelineForBlend(blend backend.BlendMode) {
	if p.dev.device == 0 {
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

	// Create bind group layouts once, then reuse across variants.
	if p.layout == 0 {
		p.createBindGroupLayouts()
	}
	if p.layout == 0 {
		return
	}

	// Reset handle; createPipelineForBlend always produces a fresh
	// pipeline for this variant. The prior handle (if any) is owned by
	// the variant map and will be released in Dispose.
	p.handle = 0

	vertexEntrySV, vertexEntryKeep := wgpu.MakeStringView("vs_main")
	fragmentEntrySV, fragmentEntryKeep := wgpu.MakeStringView("fs_main")

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
	var bufferCount uintptr
	var vbl wgpu.VertexBufferLayout
	if len(attrs) > 0 {
		vbl = wgpu.VertexBufferLayout{
			ArrayStride:    stride,
			StepMode:       wgpu.VertexStepModeVertex,
			AttributeCount: uintptr(len(attrs)),
			Attributes:     uintptr(unsafe.Pointer(&attrs[0])),
		}
		buffersPtr = uintptr(unsafe.Pointer(&vbl))
		bufferCount = 1
	}

	// Configure blend state from the variant's blend mode (not the
	// descriptor default — SetBlendMode drives this).
	blendEnabled, blendState := wgpuBlendState(blend)

	// Use the format determined by the encoder's current render target.
	targetFormat := p.createdFormat
	if targetFormat == 0 {
		targetFormat = wgpu.TextureFormatRGBA8Unorm
	}
	target := wgpu.ColorTargetState{
		Format:    targetFormat,
		WriteMask: wgpu.ColorWriteMaskAll,
	}
	if blendEnabled {
		target.Blend = uintptr(unsafe.Pointer(&blendState))
	}

	fragment := wgpu.FragmentState{
		Module:      shader.fragmentModule,
		EntryPoint:  fragmentEntrySV,
		TargetCount: 1,
		Targets:     uintptr(unsafe.Pointer(&target)),
	}

	desc := wgpu.RenderPipelineDescriptor{
		Layout: p.layout,
		Vertex: wgpu.VertexState{
			Module:      shader.vertexModule,
			EntryPoint:  vertexEntrySV,
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
	runtime.KeepAlive(vertexEntryKeep)
	runtime.KeepAlive(fragmentEntryKeep)
	runtime.KeepAlive(attrs)
	runtime.KeepAlive(vbl)
	runtime.KeepAlive(blendState)
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
		EntryCount: uintptr(len(uniformEntries)),
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
		EntryCount: uintptr(len(textureEntries)),
		Entries:    uintptr(unsafe.Pointer(&textureEntries[0])),
	}
	p.textureBGL = wgpu.DeviceCreateBindGroupLayout(p.dev.device, &textureBGLDesc)
	runtime.KeepAlive(textureEntries)

	// Pipeline layout with both groups.
	bgls := []wgpu.BindGroupLayout{p.uniformBGL, p.textureBGL}
	plDesc := wgpu.PipelineLayoutDescriptor{
		BindGroupLayoutCount: uintptr(len(bgls)),
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

// wgpuBlendState builds a wgpu-native BlendState from the backend
// BlendMode struct. Honours arbitrary factor/operation combinations so
// that custom shader blends (e.g. lighting's shadow-modulated additive
// blend) reach the GPU unmodified.
func wgpuBlendState(mode backend.BlendMode) (enabled bool, state wgpu.BlendState) {
	if !mode.Enabled {
		return false, wgpu.BlendState{}
	}
	return true, wgpu.BlendState{
		Color: wgpu.BlendComponent{
			Operation: wgpuBlendOp(mode.OpRGB),
			SrcFactor: wgpuBlendFactor(mode.SrcFactorRGB),
			DstFactor: wgpuBlendFactor(mode.DstFactorRGB),
		},
		Alpha: wgpu.BlendComponent{
			Operation: wgpuBlendOp(mode.OpAlpha),
			SrcFactor: wgpuBlendFactor(mode.SrcFactorAlpha),
			DstFactor: wgpuBlendFactor(mode.DstFactorAlpha),
		},
	}
}

// wgpuBlendFactor maps a backend BlendFactor to the wgpu-native enum.
func wgpuBlendFactor(f backend.BlendFactor) wgpu.BlendFactor {
	switch f {
	case backend.BlendFactorZero:
		return wgpu.BlendFactorZero
	case backend.BlendFactorOne:
		return wgpu.BlendFactorOne
	case backend.BlendFactorSrcAlpha:
		return wgpu.BlendFactorSrcAlpha
	case backend.BlendFactorOneMinusSrcAlpha:
		return wgpu.BlendFactorOneMinusSrcAlpha
	case backend.BlendFactorDstAlpha:
		return wgpu.BlendFactorDstAlpha
	case backend.BlendFactorOneMinusDstAlpha:
		return wgpu.BlendFactorOneMinusDstAlpha
	case backend.BlendFactorSrcColor:
		return wgpu.BlendFactorSrc
	case backend.BlendFactorOneMinusSrcColor:
		return wgpu.BlendFactorOneMinusSrc
	case backend.BlendFactorDstColor:
		return wgpu.BlendFactorDst
	case backend.BlendFactorOneMinusDstColor:
		return wgpu.BlendFactorOneMinusDst
	default:
		return wgpu.BlendFactorOne
	}
}

// wgpuBlendOp maps a backend BlendOperation to the wgpu-native enum.
func wgpuBlendOp(op backend.BlendOperation) wgpu.BlendOperation {
	switch op {
	case backend.BlendOpAdd:
		return wgpu.BlendOperationAdd
	case backend.BlendOpSubtract:
		return wgpu.BlendOperationSubtract
	case backend.BlendOpReverseSubtract:
		return wgpu.BlendOperationReverseSubtract
	case backend.BlendOpMin:
		return wgpu.BlendOperationMin
	case backend.BlendOpMax:
		return wgpu.BlendOperationMax
	default:
		return wgpu.BlendOperationAdd
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
	for _, h := range p.pipelines {
		if h != 0 {
			wgpu.RenderPipelineRelease(h)
		}
	}
	p.pipelines = nil
	// p.handle always points at one of the map's entries (the last
	// activated variant). The map release above already freed it; just
	// clear the field so Dispose is idempotent.
	p.handle = 0
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
