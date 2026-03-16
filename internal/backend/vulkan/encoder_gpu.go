//go:build vulkan

package vulkan

import (
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
		PClearValues:    uintptrOf(&clearColor),
	}
	vk.CmdBeginRenderPass(e.cmd, &rpBegin)
	e.inRenderPass = true
}

// EndRenderPass ends the current render pass.
func (e *Encoder) EndRenderPass() {
	if e.inRenderPass {
		vk.CmdEndRenderPass(e.cmd)
		e.inRenderPass = false
	}
}

// SetPipeline binds a VkPipeline.
func (e *Encoder) SetPipeline(pipeline backend.Pipeline) {
	if p, ok := pipeline.(*Pipeline); ok {
		e.currentPipeline = p
		// In a full implementation, we'd bind the actual VkPipeline here:
		// vk.CmdBindPipeline(e.cmd, p.vkPipeline)
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

// SetTexture binds a texture to a slot.
func (e *Encoder) SetTexture(_ backend.Texture, _ int) {
	// In a full implementation, this would update descriptor sets.
}

// SetTextureFilter overrides the texture filter for a slot.
func (e *Encoder) SetTextureFilter(_ int, _ backend.TextureFilter) {
	// Would create/bind a sampler with the specified filter.
}

// SetStencil configures stencil test state.
func (e *Encoder) SetStencil(_ bool, _ backend.StencilDescriptor) {
	// Stencil state is baked into the VkPipeline in Vulkan.
}

// SetColorWrite enables or disables color writing.
func (e *Encoder) SetColorWrite(_ bool) {
	// Color write mask is baked into the VkPipeline in Vulkan.
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

// uintptrOf returns the uintptr of a pointer for use in Vulkan structs.
func uintptrOf[T any](p *T) uintptr {
	return uintptr(unsafePointer(p))
}

// unsafePointer converts a typed pointer to unsafe.Pointer.
//
//go:nosplit
func unsafePointer[T any](p *T) unsafe.Pointer {
	return unsafe.Pointer(p)
}
