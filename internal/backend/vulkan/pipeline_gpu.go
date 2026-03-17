//go:build vulkan

package vulkan

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/vk"
)

// Pipeline implements backend.Pipeline for Vulkan with a real VkPipeline.
type Pipeline struct {
	dev            *Device
	desc           backend.PipelineDescriptor
	vkPipeline     vk.Pipeline
	pipelineLayout vk.PipelineLayout
	descSetLayout  vk.DescriptorSetLayout
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// Dispose releases the VkPipeline and associated resources.
func (p *Pipeline) Dispose() {
	if p.dev == nil || p.dev.device == 0 {
		return
	}
	if p.vkPipeline != 0 {
		vk.DestroyPipeline(p.dev.device, p.vkPipeline)
	}
	if p.pipelineLayout != 0 {
		vk.DestroyPipelineLayout(p.dev.device, p.pipelineLayout)
	}
	if p.descSetLayout != 0 {
		vk.DestroyDescriptorSetLayout(p.dev.device, p.descSetLayout)
	}
}

// createVkPipeline creates the actual VkPipeline from the stored descriptor.
// This is called lazily on first bind since the render pass must be known.
func (p *Pipeline) createVkPipeline(renderPass vk.RenderPass) error {
	if p.vkPipeline != 0 {
		return nil
	}

	// Create descriptor set layout for texture binding (binding 0 = combined image sampler).
	binding := vk.DescriptorSetLayoutBinding{
		Binding:            0,
		DescriptorType:     vk.DescriptorTypeCombinedImageSampler,
		DescriptorCount:    1,
		StageFlags:         vk.ShaderStageFragment,
		PImmutableSamplers: 0,
	}
	dslCI := vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: 1,
		PBindings:    uintptr(unsafe.Pointer(&binding)),
	}
	dsl, err := vk.CreateDescriptorSetLayout(p.dev.device, &dslCI)
	runtime.KeepAlive(binding)
	if err != nil {
		return err
	}
	p.descSetLayout = dsl

	// Create pipeline layout.
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

	colorBlendAttachment := vkBlendAttachment(p.desc.BlendMode)
	colorBlend := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		AttachmentCount: 1,
		PAttachments:    uintptr(unsafe.Pointer(&colorBlendAttachment)),
	}

	dynamicStates := []uint32{vk.DynamicStateViewport, vk.DynamicStateScissor}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynamicStates)),
		PDynamicStates:    uintptr(unsafe.Pointer(&dynamicStates[0])),
	}

	// Shader stages — placeholder modules.
	// In a production implementation, GLSL would be compiled to SPIR-V here.
	// For now, create an empty pipeline with the correct state configuration.
	stages := []vk.PipelineShaderStageCreateInfo{}

	var pStages uintptr
	if len(stages) > 0 {
		pStages = uintptr(unsafe.Pointer(&stages[0]))
	}

	ci := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          uint32(len(stages)),
		PStages:             pStages,
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
		// Pipeline creation may fail without SPIR-V shaders — this is expected
		// until runtime shader compilation is implemented.
		return err
	}
	p.vkPipeline = pip
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

// vkBlendAttachment creates a VkPipelineColorBlendAttachmentState from a backend blend mode.
func vkBlendAttachment(mode backend.BlendMode) vk.PipelineColorBlendAttachmentState {
	base := vk.PipelineColorBlendAttachmentState{
		ColorWriteMask: vk.ColorComponentAll,
	}
	switch mode {
	case backend.BlendSourceOver:
		base.BlendEnable = 1
		base.SrcColorBlendFactor = vk.BlendFactorSrcAlpha
		base.DstColorBlendFactor = vk.BlendFactorOneMinusSrcAlpha
		base.ColorBlendOp = vk.BlendOpAdd
		base.SrcAlphaBlendFactor = vk.BlendFactorOne
		base.DstAlphaBlendFactor = vk.BlendFactorOneMinusSrcAlpha
		base.AlphaBlendOp = vk.BlendOpAdd
	case backend.BlendAdditive:
		base.BlendEnable = 1
		base.SrcColorBlendFactor = vk.BlendFactorSrcAlpha
		base.DstColorBlendFactor = vk.BlendFactorOne
		base.ColorBlendOp = vk.BlendOpAdd
		base.SrcAlphaBlendFactor = vk.BlendFactorOne
		base.DstAlphaBlendFactor = vk.BlendFactorOne
		base.AlphaBlendOp = vk.BlendOpAdd
	case backend.BlendMultiplicative:
		base.BlendEnable = 1
		base.SrcColorBlendFactor = vk.BlendFactorDstColor
		base.DstColorBlendFactor = vk.BlendFactorZero
		base.ColorBlendOp = vk.BlendOpAdd
		base.SrcAlphaBlendFactor = vk.BlendFactorDstAlpha
		base.DstAlphaBlendFactor = vk.BlendFactorZero
		base.AlphaBlendOp = vk.BlendOpAdd
	case backend.BlendPremultiplied:
		base.BlendEnable = 1
		base.SrcColorBlendFactor = vk.BlendFactorOne
		base.DstColorBlendFactor = vk.BlendFactorOneMinusSrcAlpha
		base.ColorBlendOp = vk.BlendOpAdd
		base.SrcAlphaBlendFactor = vk.BlendFactorOne
		base.DstAlphaBlendFactor = vk.BlendFactorOneMinusSrcAlpha
		base.AlphaBlendOp = vk.BlendOpAdd
	default:
		// BlendNone — no blending.
	}
	return base
}
