//go:build (darwin || linux || freebsd || windows) && !soft

package vulkan

import (
	"runtime"
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/vk"
)

// Encoder implements backend.CommandEncoder for Vulkan by recording into
// a VkCommandBuffer.
type Encoder struct {
	dev *Device
	cmd vk.CommandBuffer

	// Current render pass state.
	inRenderPass      bool
	currentRenderPass vk.RenderPass
	currentPipeline   *Pipeline
	boundTexture    *Texture
	boundShader     *Shader
	boundSampler    vk.Sampler
	descriptorPool  vk.DescriptorPool
	descriptorSet   vk.DescriptorSet
	colorWriteOn    bool
}

// BeginRenderPass begins a Vulkan render pass.
func (e *Encoder) BeginRenderPass(desc backend.RenderPassDescriptor) {
	clearColor := vk.ClearValue{Color: desc.ClearColor}

	rp := e.dev.defaultRenderPass
	fb := e.dev.defaultFramebuffer
	w := uint32(e.dev.width)
	h := uint32(e.dev.height)

	if desc.Target != nil {
		if rt, ok := desc.Target.(*RenderTarget); ok {
			w = uint32(rt.w)
			h = uint32(rt.h)
			if rt.renderPass != 0 {
				rp = rt.renderPass
			}
			if rt.framebuffer != 0 {
				fb = rt.framebuffer
			}
		}
	} else if e.dev.hasSwapchain {
		// Rendering to the screen — use swapchain render pass and framebuffer.
		rp = e.dev.swapchainRenderPass
		fb = e.dev.swapchainFBs[e.dev.currentImageIndex]
		w = e.dev.swapchainExtent[0]
		h = e.dev.swapchainExtent[1]
	}

	rpBegin := vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass_:     rp,
		Framebuffer_:    fb,
		RenderAreaW:     w,
		RenderAreaH:     h,
		ClearValueCount: 1,
		PClearValues:    uintptr(unsafe.Pointer(&clearColor)),
	}
	vk.CmdBeginRenderPass(e.cmd, &rpBegin)
	runtime.KeepAlive(clearColor)
	e.inRenderPass = true
	e.currentRenderPass = rp

	// Set default scissor/viewport matching the render area. Vulkan requires
	// both to be set before draws when using dynamic state.
	vk.CmdSetScissor(e.cmd, vk.Rect2D{ExtentW: w, ExtentH: h})
	vk.CmdSetViewport(e.cmd, vk.Viewport{
		Width: float32(w), Height: float32(h),
		MaxDepth: 1,
	})
	e.colorWriteOn = true
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		vk.CmdEndRenderPass(e.cmd)
		e.inRenderPass = false
		e.currentRenderPass = 0
	}
	e.cleanupDescriptors()
}

// SetPipeline binds a VkPipeline.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	p, ok := pipeline.(*Pipeline)
	if !ok {
		return
	}
	e.currentPipeline = p

	// Track the shader for uniform binding.
	if s, ok := p.desc.Shader.(*Shader); ok {
		e.boundShader = s
	}

	// Create the VkPipeline lazily using the current render pass.
	if p.vkPipeline == 0 {
		rp := e.currentRenderPass
		if rp == 0 {
			rp = e.dev.defaultRenderPass
		}
		if err := p.createVkPipeline(rp); err != nil {
			// Pipeline creation failed — skip binding. Draw calls will
			// be no-ops since vkPipeline remains 0.
			return
		}
	}

	if p.vkPipeline != 0 {
		vk.CmdBindPipeline(e.cmd, p.vkPipeline)
	}
}

// SetVertexBuffer binds a vertex buffer at the most recently uploaded offset.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		vk.CmdBindVertexBuffer(e.cmd, uint32(slot), b.buffer, uint64(b.lastWriteOffset))
	}
}

// SetIndexBuffer binds an index buffer at the most recently uploaded offset.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		idxType := uint32(vk.IndexTypeUint16)
		if format == backend.IndexUint32 {
			idxType = vk.IndexTypeUint32
		}
		vk.CmdBindIndexBuffer(e.cmd, b.buffer, uint64(b.lastWriteOffset), idxType)
	}
}

// SetTexture records the texture to bind at the given slot. The actual
// descriptor set update happens in bindUniforms before each draw call so
// that the sampler, fragment UBO, and vertex UBO are all written together.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	t, ok := tex.(*Texture)
	if !ok {
		return
	}
	e.boundTexture = t
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(slot int, filter backend.TextureFilter) {
	// In Vulkan, filter state is part of the sampler. A full implementation
	// would maintain a sampler cache keyed by filter settings and rebind.
	// For now, use the default sampler.
	_ = slot
	_ = filter
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(_ bool, _ backend.StencilDescriptor) {
	// Stencil state is baked into the VkPipeline in Vulkan.
	// A full implementation would require pipeline variants per stencil config.
}

// SetColorWrite enables or disables writing to the color buffer.
func (e *Encoder) SetColorWrite(enabled bool) {
	// Color write mask is baked into the VkPipeline in Vulkan.
	// A full implementation would require pipeline variants.
	e.colorWriteOn = enabled
}

// SetViewport sets the rendering viewport.
func (e *Encoder) SetViewport(vp backend.Viewport) {
	vkVP := vk.Viewport{
		X: float32(vp.X), Y: float32(vp.Y),
		Width: float32(vp.Width), Height: float32(vp.Height),
		MinDepth: 0, MaxDepth: 1,
	}
	vk.CmdSetViewport(e.cmd, vkVP)
}

// SetScissor sets the scissor rectangle.
func (e *Encoder) SetScissor(rect *backend.ScissorRect) {
	if rect == nil {
		// Disable scissor by setting to full viewport size.
		vk.CmdSetScissor(e.cmd, vk.Rect2D{
			ExtentW: uint32(e.dev.width),
			ExtentH: uint32(e.dev.height),
		})
		return
	}
	vk.CmdSetScissor(e.cmd, vk.Rect2D{
		OffsetX: int32(rect.X),
		OffsetY: int32(rect.Y),
		ExtentW: uint32(rect.Width),
		ExtentH: uint32(rect.Height),
	})
}

// Draw issues a non-indexed draw call.
func (e *Encoder) Draw(vertexCount, instanceCount, firstVertex int) {
	e.bindUniforms()
	vk.CmdDraw(e.cmd, uint32(vertexCount), uint32(instanceCount), uint32(firstVertex), 0)
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	e.bindUniforms()
	vk.CmdDrawIndexed(e.cmd, uint32(indexCount), uint32(instanceCount), uint32(firstIndex), 0, 0)
}

// uniformAlignOffset is the minimum offset alignment for UBO descriptors.
// Vulkan spec requires minUniformBufferOffsetAlignment, typically 256 bytes.
const uniformAlignOffset = 256

// ensureDescriptorPool creates a descriptor pool supporting combined image
// samplers and uniform buffers if one does not already exist.
func (e *Encoder) ensureDescriptorPool() bool {
	if e.descriptorPool != 0 {
		return true
	}
	if e.currentPipeline == nil || e.currentPipeline.descSetLayout == 0 {
		return false
	}
	poolSizes := []vk.DescriptorPoolSize{
		{Type_: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 16},
		{Type_: vk.DescriptorTypeUniformBuffer, DescriptorCount: 32},
	}
	poolCI := vk.DescriptorPoolCreateInfo{
		SType:         vk.StructureTypeDescriptorPoolCreateInfo,
		MaxSets:       16,
		PoolSizeCount: uint32(len(poolSizes)),
		PPoolSizes:    uintptr(unsafe.Pointer(&poolSizes[0])),
	}
	pool, err := vk.CreateDescriptorPool(e.dev.device, &poolCI)
	runtime.KeepAlive(poolSizes)
	if err != nil {
		return false
	}
	e.descriptorPool = pool
	return true
}

// bindUniforms writes shader uniform data into the shared UBO buffer and
// binds a descriptor set with the sampler (binding 0), fragment UBO
// (binding 1), and vertex UBO (binding 2).
func (e *Encoder) bindUniforms() {
	if e.boundShader == nil || e.currentPipeline == nil || e.currentPipeline.pipelineLayout == 0 {
		return
	}
	if e.dev.uniformMapped == nil {
		return
	}
	if !e.ensureDescriptorPool() {
		return
	}

	// Pack vertex and fragment uniforms into the ring-buffer at increasing offsets.
	// Each draw gets its own UBO region so deferred commands read correct data.
	vtxBuf := e.boundShader.packUniformBuffer(e.boundShader.vertexUniformLayout)
	vtxSize := len(vtxBuf)
	fragBuf := e.boundShader.packUniformBuffer(e.boundShader.fragmentUniformLayout)
	fragSize := len(fragBuf)

	// Each draw needs: vtxSize (aligned to 256) + fragSize (aligned to 256).
	vtxAligned := (vtxSize + uniformAlignOffset - 1) &^ (uniformAlignOffset - 1)
	if vtxAligned < uniformAlignOffset {
		vtxAligned = uniformAlignOffset
	}
	fragAligned := (fragSize + uniformAlignOffset - 1) &^ (uniformAlignOffset - 1)
	if fragAligned < uniformAlignOffset {
		fragAligned = uniformAlignOffset
	}
	needed := vtxAligned + fragAligned

	// Wrap if we'd overflow.
	if e.dev.uniformCursor+needed > e.dev.uniformBufSize {
		e.dev.uniformCursor = 0
	}

	vtxOffset := e.dev.uniformCursor
	fragOffset := vtxOffset + vtxAligned

	fullBuf := unsafe.Slice((*byte)(e.dev.uniformMapped), e.dev.uniformBufSize)
	if vtxSize > 0 {
		copy(fullBuf[vtxOffset:vtxOffset+vtxSize], vtxBuf)
	}
	if fragSize > 0 {
		copy(fullBuf[fragOffset:fragOffset+fragSize], fragBuf)
	}
	e.dev.uniformCursor += needed

	// Allocate a descriptor set from the pool.
	set, err := vk.AllocateDescriptorSet(e.dev.device, e.descriptorPool, e.currentPipeline.descSetLayout)
	if err != nil {
		return
	}
	e.descriptorSet = set

	// Build descriptor writes for all 3 bindings.
	var writes []vk.WriteDescriptorSet

	// Binding 0: combined image sampler.
	if e.boundSampler == 0 {
		e.boundSampler = e.dev.ensureDefaultSampler()
	}
	tex := e.boundTexture
	if tex == nil {
		tex = e.dev.defaultTexture
	}
	imgInfo := vk.DescriptorImageInfo{
		Sampler:     e.boundSampler,
		ImageView:   tex.view,
		ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal,
	}
	writes = append(writes, vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      0,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeCombinedImageSampler,
		PImageInfo:      uintptr(unsafe.Pointer(&imgInfo)),
	})

	// Binding 1: fragment UBO.
	fragRange := uint64(uniformAlignOffset)
	if fragSize > 0 {
		fragRange = uint64(fragSize)
	}
	fragBufInfo := vk.DescriptorBufferInfo{
		Buffer_: e.dev.uniformBuffer,
		Offset:  uint64(fragOffset),
		Range_:  fragRange,
	}
	writes = append(writes, vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      1,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeUniformBuffer,
		PBufferInfo:     uintptr(unsafe.Pointer(&fragBufInfo)),
	})

	// Binding 2: vertex UBO.
	vtxRange := uint64(uniformAlignOffset)
	if vtxSize > 0 {
		vtxRange = uint64(vtxSize)
	}
	vtxBufInfo := vk.DescriptorBufferInfo{
		Buffer_: e.dev.uniformBuffer,
		Offset:  uint64(vtxOffset),
		Range_:  vtxRange,
	}
	writes = append(writes, vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      2,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeUniformBuffer,
		PBufferInfo:     uintptr(unsafe.Pointer(&vtxBufInfo)),
	})

	vk.UpdateDescriptorSets(e.dev.device, writes)
	runtime.KeepAlive(imgInfo)
	runtime.KeepAlive(fragBufInfo)
	runtime.KeepAlive(vtxBufInfo)

	// Bind the descriptor set.
	vk.CmdBindDescriptorSets(e.cmd, e.currentPipeline.pipelineLayout, 0, []vk.DescriptorSet{set})
}

// Flush is a no-op for Vulkan — submission happens in EndFrame.
func (e *Encoder) Flush() {}

// cleanupDescriptors releases per-frame descriptor resources.
func (e *Encoder) cleanupDescriptors() {
	if e.descriptorPool != 0 {
		vk.DestroyDescriptorPool(e.dev.device, e.descriptorPool)
		e.descriptorPool = 0
		e.descriptorSet = 0
	}
}
