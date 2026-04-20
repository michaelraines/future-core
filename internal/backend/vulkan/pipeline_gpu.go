//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/vk"
)

// Pipeline implements backend.Pipeline for Vulkan with a real VkPipeline.
//
// A single Pipeline object may be bound across multiple render passes
// (e.g. the sprite pass's default pipeline is used for offscreen RT
// passes AND the final screen/swapchain pass). Vulkan bakes the render
// pass into VkGraphicsPipelineCreateInfo, and pipelines are only
// compatible with their creation-time render pass (or a compatible
// one — same attachment descriptions). The offscreen RT and swapchain
// render passes have DIFFERENT color formats (RGBA8 vs swapchain
// format, typically BGRA8 on macOS via MoltenVK), so a pipeline baked
// for one produces undefined output when bound inside the other —
// this is what caused the "blank white screen" on the future app,
// which always renders to an offscreen RT first and then composites
// to the swapchain.
//
// Fix: cache a VkPipeline PER render pass. First bind creates the
// pipeline for the current render pass; subsequent binds reuse it.
// Dispose destroys every cached pipeline.
type Pipeline struct {
	dev            *Device
	desc           backend.PipelineDescriptor
	pipelines      map[vk.RenderPass]vk.Pipeline
	pipelineLayout vk.PipelineLayout
	descSetLayout  vk.DescriptorSetLayout
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// pipelineFor returns the cached VkPipeline compiled against renderPass,
// or 0 if none has been built yet.
func (p *Pipeline) pipelineFor(renderPass vk.RenderPass) vk.Pipeline {
	if p.pipelines == nil {
		return 0
	}
	return p.pipelines[renderPass]
}

// Dispose releases every cached VkPipeline plus the shared layout +
// descriptor set layout.
func (p *Pipeline) Dispose() {
	if p.dev == nil || p.dev.device == 0 {
		return
	}
	for _, pip := range p.pipelines {
		if pip != 0 {
			vk.DestroyPipeline(p.dev.device, pip)
		}
	}
	p.pipelines = nil
	if p.pipelineLayout != 0 {
		vk.DestroyPipelineLayout(p.dev.device, p.pipelineLayout)
	}
	if p.descSetLayout != 0 {
		vk.DestroyDescriptorSetLayout(p.dev.device, p.descSetLayout)
	}
}

// createVkPipeline creates the actual VkPipeline from the stored descriptor.
// This is called lazily on first bind per render pass since the render
// pass must be known at pipeline creation time.
func (p *Pipeline) createVkPipeline(renderPass vk.RenderPass) error {
	if p.pipelines == nil {
		p.pipelines = make(map[vk.RenderPass]vk.Pipeline)
	}
	if p.pipelines[renderPass] != 0 {
		return nil
	}

	// Create descriptor set layout with 3 bindings:
	//   Binding 0: Fragment combined image sampler (uTexture)
	//   Binding 1: Fragment UBO (uColorBody mat4 + uColorTranslation vec4 = 80 bytes)
	//   Binding 2: Vertex UBO (uProjection mat4 = 64 bytes)
	bindings := []vk.DescriptorSetLayoutBinding{
		{Binding: 0, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
		{Binding: 1, DescriptorType: vk.DescriptorTypeUniformBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageFragment},
		{Binding: 2, DescriptorType: vk.DescriptorTypeUniformBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageVertex},
	}
	dslCI := vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: uint32(len(bindings)),
		PBindings:    uintptr(unsafe.Pointer(&bindings[0])),
	}
	dsl, err := vk.CreateDescriptorSetLayout(p.dev.device, &dslCI)
	runtime.KeepAlive(bindings)
	if err != nil {
		return err
	}
	p.descSetLayout = dsl

	// Create pipeline layout — UBOs are in descriptor sets, no push constants needed.
	plCI := vk.PipelineLayoutCreateInfo{
		SType:          vk.StructureTypePipelineLayoutCreateInfo,
		SetLayoutCount: 1,
		PSetLayouts:    uintptr(unsafe.Pointer(&dsl)),
	}
	layout, err := vk.CreatePipelineLayout(p.dev.device, uintptr(unsafe.Pointer(&plCI)))
	if err != nil {
		return err
	}
	p.pipelineLayout = layout

	// Build vertex input state from the pipeline descriptor's vertex format.
	vf := p.desc.VertexFormat
	bindingDesc := vk.VertexInputBindingDescription{
		Binding:   0,
		Stride:    uint32(vf.Stride),
		InputRate: vk.VertexInputRateVertex,
	}

	attrDescs := make([]vk.VertexInputAttributeDescription, len(vf.Attributes))
	for i, attr := range vf.Attributes {
		attrDescs[i] = vk.VertexInputAttributeDescription{
			Location: uint32(i),
			Binding:  0,
			Format:   vkVertexAttrFormat(attr.Format),
			Offset:   uint32(attr.Offset),
		}
	}

	var pAttrDescs uintptr
	if len(attrDescs) > 0 {
		pAttrDescs = uintptr(unsafe.Pointer(&attrDescs[0]))
	}

	vertexInput := vk.PipelineVertexInputStateCreateInfo{
		SType:                           vk.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   1,
		PVertexBindingDescriptions:      uintptr(unsafe.Pointer(&bindingDesc)),
		VertexAttributeDescriptionCount: uint32(len(attrDescs)),
		PVertexAttributeDescriptions:    pAttrDescs,
	}

	topology := vkTopology(p.desc.Primitive)
	inputAssembly := vk.PipelineInputAssemblyStateCreateInfo{
		SType:    vk.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology: topology,
	}

	// Use dynamic viewport/scissor.
	viewportState := vk.PipelineViewportStateCreateInfo{
		SType:         vk.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		ScissorCount:  1,
	}

	rasterization := vk.PipelineRasterizationStateCreateInfo{
		SType:       vk.StructureTypePipelineRasterizationStateCreateInfo,
		PolygonMode: vk.PolygonModeFill,
		CullMode:    vkCullMode(p.desc.CullMode),
		FrontFace:   vk.FrontFaceCounterClockwise,
		LineWidth:   1.0,
	}

	multisample := vk.PipelineMultisampleStateCreateInfo{
		SType:                vk.StructureTypePipelineMultisampleStateCreateInfo,
		RasterizationSamples: vk.SampleCount1,
	}

	depthTest := uint32(0)
	depthWrite := uint32(0)
	if p.desc.DepthTest {
		depthTest = 1
	}
	if p.desc.DepthWrite {
		depthWrite = 1
	}
	depthStencil := vk.PipelineDepthStencilStateCreateInfo{
		SType:            vk.StructureTypePipelineDepthStencilStateCreateInfo,
		DepthTestEnable:  depthTest,
		DepthWriteEnable: depthWrite,
		DepthCompareOp:   vkCompareOp(p.desc.DepthFunc),
	}
	if p.desc.StencilEnable {
		sd := p.desc.Stencil
		depthStencil.StencilTestEnable = 1
		cmpOp := vkCompareOp(sd.Func)
		depthStencil.FrontFailOp = vkStencilOp(sd.Front.SFail)
		depthStencil.FrontPassOp = vkStencilOp(sd.Front.DPPass)
		depthStencil.FrontDepthFailOp = vkStencilOp(sd.Front.DPFail)
		depthStencil.FrontCompareOp = cmpOp
		depthStencil.FrontCompareMask = sd.Mask
		depthStencil.FrontWriteMask = sd.WriteMask
		backOps := sd.Front
		if sd.TwoSided {
			backOps = sd.Back
		}
		depthStencil.BackFailOp = vkStencilOp(backOps.SFail)
		depthStencil.BackPassOp = vkStencilOp(backOps.DPPass)
		depthStencil.BackDepthFailOp = vkStencilOp(backOps.DPFail)
		depthStencil.BackCompareOp = cmpOp
		depthStencil.BackCompareMask = sd.Mask
		depthStencil.BackWriteMask = sd.WriteMask
		// Reference is dynamic — set via vkCmdSetStencilReference at draw
		// time. The dynamic-state entry below is required; without it,
		// Vulkan bakes the FrontReference/BackReference values and the
		// encoder's SetStencilReference calls silently no-op.
	}

	colorBlendAttachment := vkBlendAttachment(p.desc.BlendMode)
	if p.desc.ColorWriteDisabled {
		// Stencil-only passes disable color writes at pipeline creation
		// time; masking all channels produces the same effect as
		// setcolor toggling on other backends. Needed for the sprite
		// pass's fill-rule stencil-write pipelines to populate stencil
		// without touching color.
		colorBlendAttachment.ColorWriteMask = 0
	}
	colorBlend := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		AttachmentCount: 1,
		PAttachments:    uintptr(unsafe.Pointer(&colorBlendAttachment)),
	}

	dynamicStates := []uint32{vk.DynamicStateViewport, vk.DynamicStateScissor}
	if p.desc.StencilEnable {
		// Must be included for vkCmdSetStencilReference to take effect —
		// omitting it silently bakes ref=0 into the pipeline and breaks
		// all stencil-based draws (e.g. fill-rule routing in sprite
		// pass) with no Vulkan validation error to flag the problem.
		dynamicStates = append(dynamicStates, vk.DynamicStateStencilReference)
	}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynamicStates)),
		PDynamicStates:    uintptr(unsafe.Pointer(&dynamicStates[0])),
	}

	// Shader stages — compile GLSL to SPIR-V and create VkShaderModules.
	shader, hasShader := p.desc.Shader.(*Shader)
	stages := []vk.PipelineShaderStageCreateInfo{}
	var mainName *byte
	if hasShader && shader != nil {
		if err := shader.compile(); err != nil {
			return fmt.Errorf("vulkan: shader compilation: %w", err)
		}
		mainName = vk.CStr("main")
		if shader.vertexModule != 0 {
			stages = append(stages, vk.PipelineShaderStageCreateInfo{
				SType:  vk.StructureTypePipelineShaderStageCreateInfo,
				Stage:  vk.ShaderStageVertex,
				Module: shader.vertexModule,
				PName:  uintptr(unsafe.Pointer(mainName)),
			})
		}
		if shader.fragmentModule != 0 {
			stages = append(stages, vk.PipelineShaderStageCreateInfo{
				SType:  vk.StructureTypePipelineShaderStageCreateInfo,
				Stage:  vk.ShaderStageFragment,
				Module: shader.fragmentModule,
				PName:  uintptr(unsafe.Pointer(mainName)),
			})
		}
	}

	if len(stages) == 0 {
		return fmt.Errorf("vulkan: cannot create pipeline without shader stages")
	}

	ci := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          uint32(len(stages)),
		PStages:             uintptr(unsafe.Pointer(&stages[0])),
		PVertexInputState:   uintptr(unsafe.Pointer(&vertexInput)),
		PInputAssemblyState: uintptr(unsafe.Pointer(&inputAssembly)),
		PViewportState:      uintptr(unsafe.Pointer(&viewportState)),
		PRasterizationState: uintptr(unsafe.Pointer(&rasterization)),
		PMultisampleState:   uintptr(unsafe.Pointer(&multisample)),
		PDepthStencilState:  uintptr(unsafe.Pointer(&depthStencil)),
		PColorBlendState:    uintptr(unsafe.Pointer(&colorBlend)),
		PDynamicState:       uintptr(unsafe.Pointer(&dynamicState)),
		Layout:              layout,
		RenderPass_:         renderPass,
	}

	pip, err := vk.CreateGraphicsPipeline(p.dev.device, uintptr(unsafe.Pointer(&ci)))
	// Keep all referenced objects alive past the FFI call.
	runtime.KeepAlive(stages)
	runtime.KeepAlive(mainName)
	runtime.KeepAlive(vertexInput)
	runtime.KeepAlive(inputAssembly)
	runtime.KeepAlive(viewportState)
	runtime.KeepAlive(rasterization)
	runtime.KeepAlive(multisample)
	runtime.KeepAlive(depthStencil)
	runtime.KeepAlive(colorBlend)
	runtime.KeepAlive(dynamicState)
	runtime.KeepAlive(bindingDesc)
	runtime.KeepAlive(attrDescs)
	runtime.KeepAlive(colorBlendAttachment)
	runtime.KeepAlive(dynamicStates)
	if err != nil {
		return err
	}
	p.pipelines[renderPass] = pip
	return nil
}

// vkVertexAttrFormat maps backend attribute format to VkFormat.
func vkVertexAttrFormat(f backend.AttributeFormat) uint32 {
	switch f {
	case backend.AttributeFloat2:
		return vk.FormatR32G32SFloat
	case backend.AttributeFloat3:
		return vk.FormatR32G32B32SFloat
	case backend.AttributeFloat4:
		return vk.FormatR32G32B32A32SFloat
	case backend.AttributeByte4Norm:
		return vk.FormatR8G8B8A8UNorm
	default:
		return vk.FormatR32G32B32A32SFloat
	}
}

// vkTopology maps backend primitive type to VkPrimitiveTopology.
func vkTopology(p backend.PrimitiveType) uint32 {
	switch p {
	case backend.PrimitiveTriangles:
		return vk.PrimitiveTopologyTriangleList
	case backend.PrimitiveTriangleStrip:
		return vk.PrimitiveTopologyTriangleStrip
	case backend.PrimitiveLines:
		return vk.PrimitiveTopologyLineList
	case backend.PrimitiveLineStrip:
		return vk.PrimitiveTopologyLineStrip
	case backend.PrimitivePoints:
		return vk.PrimitiveTopologyPointList
	default:
		return vk.PrimitiveTopologyTriangleList
	}
}

// vkCullMode maps backend cull mode to VkCullModeFlags.
func vkCullMode(c backend.CullMode) uint32 {
	switch c {
	case backend.CullNone:
		return vk.CullModeNone
	case backend.CullFront:
		return vk.CullModeFront
	case backend.CullBack:
		return vk.CullModeBack
	default:
		return vk.CullModeNone
	}
}

// vkStencilOp maps a backend.StencilOp to the corresponding VkStencilOp
// value. Clamp vs. wrap variants map to Vulkan's Increment/DecrementAndClamp
// and Increment/DecrementAndWrap respectively.
func vkStencilOp(op backend.StencilOp) uint32 {
	switch op {
	case backend.StencilZero:
		return vk.StencilOpZero
	case backend.StencilReplace:
		return vk.StencilOpReplace
	case backend.StencilIncr:
		return vk.StencilOpIncrementAndClamp
	case backend.StencilDecr:
		return vk.StencilOpDecrementAndClamp
	case backend.StencilInvert:
		return vk.StencilOpInvert
	case backend.StencilIncrWrap:
		return vk.StencilOpIncrementAndWrap
	case backend.StencilDecrWrap:
		return vk.StencilOpDecrementAndWrap
	default: // StencilKeep
		return vk.StencilOpKeep
	}
}

// vkCompareOp maps backend compare func to VkCompareOp.
func vkCompareOp(c backend.CompareFunc) uint32 {
	switch c {
	case backend.CompareNever:
		return vk.CompareOpNever
	case backend.CompareLess:
		return vk.CompareOpLess
	case backend.CompareLessEqual:
		return vk.CompareOpLessOrEqual
	case backend.CompareEqual:
		return vk.CompareOpEqual
	case backend.CompareGreaterEqual:
		return vk.CompareOpGreaterOrEqual
	case backend.CompareGreater:
		return vk.CompareOpGreater
	case backend.CompareNotEqual:
		return vk.CompareOpNotEqual
	case backend.CompareAlways:
		return vk.CompareOpAlways
	default:
		return vk.CompareOpLessOrEqual
	}
}

// vkBlendAttachment creates a VkPipelineColorBlendAttachmentState from a
// backend BlendMode struct. Honours arbitrary factor/operation combinations
// by mapping each struct field to the corresponding VkBlendFactor / VkBlendOp.
func vkBlendAttachment(mode backend.BlendMode) vk.PipelineColorBlendAttachmentState {
	base := vk.PipelineColorBlendAttachmentState{
		ColorWriteMask: vk.ColorComponentAll,
	}
	if !mode.Enabled {
		return base
	}
	base.BlendEnable = 1
	base.SrcColorBlendFactor = uint32(vkBlendFactor(mode.SrcFactorRGB))
	base.DstColorBlendFactor = uint32(vkBlendFactor(mode.DstFactorRGB))
	base.SrcAlphaBlendFactor = uint32(vkBlendFactor(mode.SrcFactorAlpha))
	base.DstAlphaBlendFactor = uint32(vkBlendFactor(mode.DstFactorAlpha))
	base.ColorBlendOp = uint32(vkBlendOp(mode.OpRGB))
	base.AlphaBlendOp = uint32(vkBlendOp(mode.OpAlpha))
	return base
}

// vkBlendFactor maps a backend BlendFactor to the Vulkan enum value.
func vkBlendFactor(f backend.BlendFactor) int {
	switch f {
	case backend.BlendFactorZero:
		return vk.BlendFactorZero
	case backend.BlendFactorOne:
		return vk.BlendFactorOne
	case backend.BlendFactorSrcAlpha:
		return vk.BlendFactorSrcAlpha
	case backend.BlendFactorOneMinusSrcAlpha:
		return vk.BlendFactorOneMinusSrcAlpha
	case backend.BlendFactorDstAlpha:
		return vk.BlendFactorDstAlpha
	case backend.BlendFactorOneMinusDstAlpha:
		return vk.BlendFactorOneMinusDstAlpha
	case backend.BlendFactorSrcColor:
		return vk.BlendFactorSrcColor
	case backend.BlendFactorOneMinusSrcColor:
		return vk.BlendFactorOneMinusSrcColor
	case backend.BlendFactorDstColor:
		return vk.BlendFactorDstColor
	case backend.BlendFactorOneMinusDstColor:
		return vk.BlendFactorOneMinusDstColor
	default:
		return vk.BlendFactorOne
	}
}

// vkBlendOp maps a backend BlendOperation to the Vulkan enum value.
func vkBlendOp(op backend.BlendOperation) int {
	switch op {
	case backend.BlendOpAdd:
		return vk.BlendOpAdd
	case backend.BlendOpSubtract:
		return vk.BlendOpSubtract
	case backend.BlendOpReverseSubtract:
		return vk.BlendOpReverseSubtract
	case backend.BlendOpMin:
		return vk.BlendOpMin
	case backend.BlendOpMax:
		return vk.BlendOpMax
	default:
		return vk.BlendOpAdd
	}
}
