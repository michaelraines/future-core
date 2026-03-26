//go:build (darwin || linux || freebsd || windows) && !soft

package vk

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// TestStructSizes verifies that Go struct sizes match their Vulkan C equivalents.
// A mismatch here means the struct layout is wrong for FFI and will cause
// memory corruption when passed to Vulkan functions.
func TestStructSizes(t *testing.T) {
	tests := []struct {
		name     string
		got      uintptr
		expected uintptr
	}{
		{"DescriptorImageInfo", unsafe.Sizeof(DescriptorImageInfo{}), 24},
		{"DescriptorBufferInfo", unsafe.Sizeof(DescriptorBufferInfo{}), 24},
		{"WriteDescriptorSet", unsafe.Sizeof(WriteDescriptorSet{}), 64},
		{"DescriptorSetAllocateInfo", unsafe.Sizeof(DescriptorSetAllocateInfo{}), 40},
		{"DescriptorSetLayoutBinding", unsafe.Sizeof(DescriptorSetLayoutBinding{}), 24},
		{"DescriptorSetLayoutCreateInfo", unsafe.Sizeof(DescriptorSetLayoutCreateInfo{}), 32},
		{"DescriptorPoolCreateInfo", unsafe.Sizeof(DescriptorPoolCreateInfo{}), 40},
		{"DescriptorPoolSize", unsafe.Sizeof(DescriptorPoolSize{}), 8},
		{"GraphicsPipelineCreateInfo", unsafe.Sizeof(GraphicsPipelineCreateInfo{}), 144},
		{"PipelineShaderStageCreateInfo", unsafe.Sizeof(PipelineShaderStageCreateInfo{}), 48},
		{"PipelineVertexInputStateCreateInfo", unsafe.Sizeof(PipelineVertexInputStateCreateInfo{}), 48},
		{"PipelineInputAssemblyStateCreateInfo", unsafe.Sizeof(PipelineInputAssemblyStateCreateInfo{}), 32},
		{"PipelineViewportStateCreateInfo", unsafe.Sizeof(PipelineViewportStateCreateInfo{}), 48},
		{"PipelineRasterizationStateCreateInfo", unsafe.Sizeof(PipelineRasterizationStateCreateInfo{}), 64},
		{"PipelineMultisampleStateCreateInfo", unsafe.Sizeof(PipelineMultisampleStateCreateInfo{}), 48},
		{"PipelineDepthStencilStateCreateInfo", unsafe.Sizeof(PipelineDepthStencilStateCreateInfo{}), 104},
		{"PipelineColorBlendStateCreateInfo", unsafe.Sizeof(PipelineColorBlendStateCreateInfo{}), 56},
		{"PipelineColorBlendAttachmentState", unsafe.Sizeof(PipelineColorBlendAttachmentState{}), 32},
		{"PipelineDynamicStateCreateInfo", unsafe.Sizeof(PipelineDynamicStateCreateInfo{}), 32},
		{"PipelineLayoutCreateInfo", unsafe.Sizeof(PipelineLayoutCreateInfo{}), 48},
		{"RenderPassBeginInfo", unsafe.Sizeof(RenderPassBeginInfo{}), 64},
		{"BufferImageCopy", unsafe.Sizeof(BufferImageCopy{}), 56},
		{"ImageMemoryBarrier", unsafe.Sizeof(ImageMemoryBarrier{}), 72},
		{"SubmitInfo", unsafe.Sizeof(SubmitInfo{}), 72},
		{"ShaderModuleCreateInfo", unsafe.Sizeof(ShaderModuleCreateInfo{}), 40},
		{"ClearValue", unsafe.Sizeof(ClearValue{}), 16},
		{"Viewport", unsafe.Sizeof(Viewport{}), 24},
		{"Rect2D", unsafe.Sizeof(Rect2D{}), 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.got,
				"%s: Go size %d != expected C size %d", tt.name, tt.got, tt.expected)
		})
	}
}
