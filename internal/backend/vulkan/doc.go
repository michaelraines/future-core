// Package vulkan implements backend.Device targeting the Vulkan API.
//
// Vulkan is a modern low-overhead GPU API for Linux, Windows, and Android.
// This backend models Vulkan concepts — VkInstance, physical/logical devices,
// queue families, VkFormat enums, VkBufferUsageFlags, SPIR-V shader modules,
// VkPipeline state objects, and VkCommandBuffer recording — while currently
// delegating actual rendering to the software rasterizer for conformance
// testing in any environment.
//
// The backend registers itself as "vulkan" in the backend registry.
//
// Key API-specific types:
//   - InstanceCreateInfo: mirrors VkInstanceCreateInfo (app name, API version, layers)
//   - PhysicalDeviceInfo: mirrors VkPhysicalDeviceProperties
//   - VkFormat/VkBufferUsageFlags constants: map backend types to Vulkan enums
//   - Validation layers: enabled via DeviceConfig.Debug
package vulkan
