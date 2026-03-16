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
	if len(data) == 0 || t.dev.stagingMapped == 0 {
		return
	}
	n := len(data)
	if n > t.dev.stagingSize {
		n = t.dev.stagingSize
	}
	// Copy to staging buffer.
	dst := unsafe.Slice((*byte)(unsafe.Pointer(t.dev.stagingMapped)), n)
	copy(dst, data[:n])

	// Record and submit a one-time command buffer to copy staging → image.
	cmd, err := vk.AllocateCommandBuffer(t.dev.device, t.dev.commandPool)
	if err != nil {
		return
	}
	if err := vk.BeginCommandBuffer(cmd, vk.CommandBufferUsageOneTimeSubmit); err != nil {
		return
	}

	region := vk.BufferImageCopy{
		AspectMask:   vk.ImageAspectColor,
		LayerCount:   1,
		ImageExtentW: uint32(t.w),
		ImageExtentH: uint32(t.h),
		ImageExtentD: 1,
	}
	vk.CmdCopyBufferToImage(cmd, t.dev.stagingBuffer, t.image, vk.ImageLayoutTransferDstOptimal, region)

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
func (t *Texture) UploadRegion(data []byte, _, _, _, _, _ int) {
	// Full upload via staging for simplicity.
	t.Upload(data, 0)
}

// ReadPixels reads RGBA pixel data from the texture.
func (t *Texture) ReadPixels(dst []byte) {
	if len(dst) == 0 {
		return
	}
	// In a full implementation, this would copy the image to a staging
	// buffer via vkCmdCopyImageToBuffer, then read from mapped memory.
	// For now, zero-fill (actual pixel readback requires command submission).
	for i := range dst {
		dst[i] = 0
	}
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
