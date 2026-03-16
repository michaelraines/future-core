//go:build !vulkan

package vulkan

import "github.com/michaelraines/future-render/internal/backend"

// Shader implements backend.Shader for Vulkan.
// Models a pair of VkShaderModule (vertex + fragment) plus a SPIR-V blob.
// In a real implementation, GLSL source would be compiled to SPIR-V via
// glslang or accepted as pre-compiled SPIR-V.
type Shader struct {
	backend.Shader // delegates all Shader methods to inner
}
