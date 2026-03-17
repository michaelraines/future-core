//go:build vulkan

package vulkan

import (
	"unsafe"

	"github.com/michaelraines/future-render/internal/backend"
	"github.com/michaelraines/future-render/internal/vk"
)

// Texture implements backend.Texture for Vulkan using VkImage + VkImageView.
type Texture struct {
	dev    *Device
	image  vk.Image
	view   vk.ImageView
	memory vk.DeviceMemory
	w, h   int
	format backend.TextureFormat

	vkFormat  int
	vkUsage   int
	mipLevels int
}

// InnerTexture returns nil for GPU textures (no soft delegation).
func (t *Texture) InnerTexture() backend.Texture { return nil }

// Upload uploads pixel data to the texture via staging buffer + vkCmdCopyBufferToImage.
func (t *Texture) Upload(data []byte, _ int) {
	if len(data) == 0 || t.dev.stagingMapped == nil {
		return
	}
	n := len(data)
	if n > t.dev.stagingSize {
		n = t.dev.stagingSize
	}
	// Copy to staging buffer.
	dst := unsafe.Slice((*byte)(t.dev.stagingMapped), n)
	copy(dst, data[:n])

	// Record and submit a one-time command buffer to copy staging → image.
	cmd, err := vk.AllocateCommandBuffer(t.dev.device, t.dev.commandPool)
	if err != nil {
		return
	}
	if err := vk.BeginCommandBuffer(cmd, vk.CommandBufferUsageOneTimeSubmit); err != nil {
		return
	}

	// Transition image to transfer dst.
	barriers := []vk.ImageMemoryBarrier{{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       0,
		DstAccessMask:       vk.AccessTransferWrite,
		OldLayout:           vk.ImageLayoutUndefined,
		NewLayout:           vk.ImageLayoutTransferDstOptimal,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image_:              t.image,
		SubresAspectMask:    vk.ImageAspectColor,
		SubresLevelCount:    1,
		SubresLayerCount:    1,
	}}
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageTopOfPipe, vk.PipelineStageTransfer, barriers)

	region := vk.BufferImageCopy{
		AspectMask:   vk.ImageAspectColor,
		LayerCount:   1,
		ImageExtentW: uint32(t.w),
		ImageExtentH: uint32(t.h),
		ImageExtentD: 1,
	}
	vk.CmdCopyBufferToImage(cmd, t.dev.stagingBuffer, t.image, vk.ImageLayoutTransferDstOptimal, region)

	// Transition image to shader read optimal.
	barriers[0].SrcAccessMask = vk.AccessTransferWrite
	barriers[0].DstAccessMask = vk.AccessShaderRead
	barriers[0].OldLayout = vk.ImageLayoutTransferDstOptimal
	barriers[0].NewLayout = vk.ImageLayoutShaderReadOnlyOptimal
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, barriers)

	_ = vk.EndCommandBuffer(cmd)

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&cmd)),
	}
	_ = vk.QueueSubmit(t.dev.graphicsQueue, &submitInfo, 0)
	_ = vk.DeviceWaitIdle(t.dev.device)
	vk.FreeCommandBuffers(t.dev.device, t.dev.commandPool, cmd)
}

// UploadRegion uploads pixel data to a rectangular region.
func (t *Texture) UploadRegion(data []byte, x, y, w, h, _ int) {
	if len(data) == 0 || t.dev.stagingMapped == nil {
		return
	}
	n := len(data)
	if n > t.dev.stagingSize {
		n = t.dev.stagingSize
	}
	dst := unsafe.Slice((*byte)(t.dev.stagingMapped), n)
	copy(dst, data[:n])

	cmd, err := vk.AllocateCommandBuffer(t.dev.device, t.dev.commandPool)
	if err != nil {
		return
	}
	if err := vk.BeginCommandBuffer(cmd, vk.CommandBufferUsageOneTimeSubmit); err != nil {
		return
	}

	barriers := []vk.ImageMemoryBarrier{{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       0,
		DstAccessMask:       vk.AccessTransferWrite,
		OldLayout:           vk.ImageLayoutUndefined,
		NewLayout:           vk.ImageLayoutTransferDstOptimal,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image_:              t.image,
		SubresAspectMask:    vk.ImageAspectColor,
		SubresLevelCount:    1,
		SubresLayerCount:    1,
	}}
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageTopOfPipe, vk.PipelineStageTransfer, barriers)

	region := vk.BufferImageCopy{
		AspectMask:   vk.ImageAspectColor,
		LayerCount:   1,
		ImageOffsetX: int32(x),
		ImageOffsetY: int32(y),
		ImageExtentW: uint32(w),
		ImageExtentH: uint32(h),
		ImageExtentD: 1,
	}
	vk.CmdCopyBufferToImage(cmd, t.dev.stagingBuffer, t.image, vk.ImageLayoutTransferDstOptimal, region)

	barriers[0].SrcAccessMask = vk.AccessTransferWrite
	barriers[0].DstAccessMask = vk.AccessShaderRead
	barriers[0].OldLayout = vk.ImageLayoutTransferDstOptimal
	barriers[0].NewLayout = vk.ImageLayoutShaderReadOnlyOptimal
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, barriers)

	_ = vk.EndCommandBuffer(cmd)
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&cmd)),
	}
	_ = vk.QueueSubmit(t.dev.graphicsQueue, &submitInfo, 0)
	_ = vk.DeviceWaitIdle(t.dev.device)
	vk.FreeCommandBuffers(t.dev.device, t.dev.commandPool, cmd)
}

// ReadPixels reads RGBA pixel data from the texture via staging buffer.
func (t *Texture) ReadPixels(dst []byte) {
	if len(dst) == 0 || t.dev.stagingMapped == nil {
		return
	}

	dataSize := t.w * t.h * 4 // Assume RGBA8
	if dataSize > t.dev.stagingSize {
		// Staging buffer too small — zero-fill as fallback.
		for i := range dst {
			dst[i] = 0
		}
		return
	}

	cmd, err := vk.AllocateCommandBuffer(t.dev.device, t.dev.commandPool)
	if err != nil {
		return
	}
	if err := vk.BeginCommandBuffer(cmd, vk.CommandBufferUsageOneTimeSubmit); err != nil {
		return
	}

	// Transition image to transfer src. After Upload, layout is ShaderReadOnlyOptimal.
	barriers := []vk.ImageMemoryBarrier{{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       vk.AccessShaderRead,
		DstAccessMask:       vk.AccessTransferRead,
		OldLayout:           vk.ImageLayoutShaderReadOnlyOptimal,
		NewLayout:           vk.ImageLayoutTransferSrcOptimal,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image_:              t.image,
		SubresAspectMask:    vk.ImageAspectColor,
		SubresLevelCount:    1,
		SubresLayerCount:    1,
	}}
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageFragmentShader, vk.PipelineStageTransfer, barriers)

	// Copy image to staging buffer.
	region := vk.BufferImageCopy{
		AspectMask:   vk.ImageAspectColor,
		LayerCount:   1,
		ImageExtentW: uint32(t.w),
		ImageExtentH: uint32(t.h),
		ImageExtentD: 1,
	}
	vk.CmdCopyImageToBuffer(cmd, t.image, vk.ImageLayoutTransferSrcOptimal, t.dev.stagingBuffer, region)

	// Transition back to ShaderReadOnlyOptimal.
	barriers[0].SrcAccessMask = vk.AccessTransferRead
	barriers[0].DstAccessMask = vk.AccessShaderRead
	barriers[0].OldLayout = vk.ImageLayoutTransferSrcOptimal
	barriers[0].NewLayout = vk.ImageLayoutShaderReadOnlyOptimal
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageTransfer, vk.PipelineStageFragmentShader, barriers)

	_ = vk.EndCommandBuffer(cmd)
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    uintptr(unsafe.Pointer(&cmd)),
	}
	_ = vk.QueueSubmit(t.dev.graphicsQueue, &submitInfo, 0)
	_ = vk.DeviceWaitIdle(t.dev.device)

	// Copy from staging buffer to dst.
	n := len(dst)
	if n > dataSize {
		n = dataSize
	}
	src := unsafe.Slice((*byte)(t.dev.stagingMapped), n)
	copy(dst[:n], src)

	vk.FreeCommandBuffers(t.dev.device, t.dev.commandPool, cmd)
}

// Width returns the texture width.
func (t *Texture) Width() int { return t.w }

// Height returns the texture height.
func (t *Texture) Height() int { return t.h }

// Format returns the texture format.
func (t *Texture) Format() backend.TextureFormat { return t.format }

// Dispose releases the VkImage, VkImageView, and VkDeviceMemory.
func (t *Texture) Dispose() {
	if t.dev == nil || t.dev.device == 0 {
		return
	}
	if t.view != 0 {
		vk.DestroyImageView(t.dev.device, t.view)
	}
	if t.image != 0 {
		vk.DestroyImage(t.dev.device, t.image)
	}
	if t.memory != 0 {
		vk.FreeMemory(t.dev.device, t.memory)
	}
}
