//go:build vulkan

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
	inRenderPass    bool
	currentPipeline *Pipeline
	boundTexture    *Texture
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
	e.colorWriteOn = true
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		vk.CmdEndRenderPass(e.cmd)
		e.inRenderPass = false
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

	// Attempt to create the VkPipeline lazily if not yet created.
	if p.vkPipeline == 0 {
		_ = p.createVkPipeline(e.dev.defaultRenderPass)
	}

	if p.vkPipeline != 0 {
		vk.CmdBindPipeline(e.cmd, p.vkPipeline)
	}
}

// SetVertexBuffer binds a vertex buffer.
func (e *Encoder) SetVertexBuffer(buf backend.Buffer, slot int) {
	if b, ok := buf.(*Buffer); ok {
		vk.CmdBindVertexBuffer(e.cmd, uint32(slot), b.buffer, 0)
	}
}

// SetIndexBuffer binds an index buffer.
func (e *Encoder) SetIndexBuffer(buf backend.Buffer, format backend.IndexFormat) {
	if b, ok := buf.(*Buffer); ok {
		idxType := uint32(vk.IndexTypeUint16)
		if format == backend.IndexUint32 {
			idxType = vk.IndexTypeUint32
		}
		vk.CmdBindIndexBuffer(e.cmd, b.buffer, 0, idxType)
	}
}

// SetTexture binds a texture to a slot via descriptor sets.
func (e *Encoder) SetTexture(tex backend.Texture, slot int) {
	t, ok := tex.(*Texture)
	if !ok || e.currentPipeline == nil {
		return
	}
	e.boundTexture = t

	// Create descriptor pool if needed.
	if e.descriptorPool == 0 && e.currentPipeline.descSetLayout != 0 {
		poolSize := vk.DescriptorPoolSize{
			Type_:           vk.DescriptorTypeCombinedImageSampler,
			DescriptorCount: 16,
		}
		poolCI := vk.DescriptorPoolCreateInfo{
			SType:         vk.StructureTypeDescriptorPoolCreateInfo,
			MaxSets:       16,
			PoolSizeCount: 1,
			PPoolSizes:    uintptr(unsafe.Pointer(&poolSize)),
		}
		pool, err := vk.CreateDescriptorPool(e.dev.device, &poolCI)
		runtime.KeepAlive(poolSize)
		if err != nil {
			return
		}
		e.descriptorPool = pool
	}

	if e.currentPipeline.descSetLayout == 0 || e.descriptorPool == 0 {
		return
	}

	// Allocate descriptor set.
	set, err := vk.AllocateDescriptorSet(e.dev.device, e.descriptorPool, e.currentPipeline.descSetLayout)
	if err != nil {
		return
	}
	e.descriptorSet = set

	// Ensure we have a sampler.
	if e.boundSampler == 0 {
		e.boundSampler = e.dev.ensureDefaultSampler()
	}

	// Update the descriptor set with the texture's image view.
	imgInfo := vk.DescriptorImageInfo{
		Sampler:     e.boundSampler,
		ImageView:   t.view,
		ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal,
	}
	write := vk.WriteDescriptorSet{
		SType:           vk.StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DstBinding:      0,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeCombinedImageSampler,
		PImageInfo:      uintptr(unsafe.Pointer(&imgInfo)),
	}
	vk.UpdateDescriptorSets(e.dev.device, []vk.WriteDescriptorSet{write})
	runtime.KeepAlive(imgInfo)

	// Bind the descriptor set.
	if e.currentPipeline.pipelineLayout != 0 {
		vk.CmdBindDescriptorSets(e.cmd, e.currentPipeline.pipelineLayout, 0, []vk.DescriptorSet{set})
	}
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
	vk.CmdDraw(e.cmd, uint32(vertexCount), uint32(instanceCount), uint32(firstVertex), 0)
}

// DrawIndexed issues an indexed draw call.
func (e *Encoder) DrawIndexed(indexCount, instanceCount, firstIndex int) {
	vk.CmdDrawIndexed(e.cmd, uint32(indexCount), uint32(instanceCount), uint32(firstIndex), 0, 0)
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
