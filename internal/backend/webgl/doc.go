// Package webgl implements backend.Device targeting the WebGL2 API.
//
// WebGL2 is the browser graphics API accessed via syscall/js under
// GOOS=js GOARCH=wasm. This backend models WebGL2 concepts — GL texture
// targets, buffer bindings, context attributes, and GLSL ES 3.00 shader
// translation — while currently delegating actual rendering to the software
// rasterizer for conformance testing in any environment.
//
// The backend registers itself as "webgl" in the backend registry.
//
// Key API-specific types:
//   - ContextAttributes: mirrors WebGL2 canvas context creation options
//   - GL format/target constants: map backend.TextureFormat to GL enums
//   - translateGLSLES: stub for GLSL 330 → GLSL ES 3.00 source rewriting
package webgl
