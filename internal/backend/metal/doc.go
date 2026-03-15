// Package metal implements backend.Device targeting Apple's Metal API.
//
// Metal is Apple's GPU API for macOS and iOS, accessed via purego +
// Objective-C runtime calls (objc_msgSend). This backend models Metal
// concepts — MTLDevice, MTLCommandQueue, MTLRenderPipelineState,
// MTLPixelFormat, MTLTextureUsage, MTLStorageMode, and feature sets —
// while currently delegating actual rendering to the software rasterizer
// for conformance testing in any environment.
//
// The backend registers itself as "metal" in the backend registry.
//
// Key API-specific types:
//   - FeatureSet: represents Metal GPU family capabilities
//   - MTLPixelFormat constants: map backend.TextureFormat to Metal pixel formats
//   - MTLStorageMode constants: model GPU memory modes (shared/managed/private)
package metal
