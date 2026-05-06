// sprite vertex shader — hand-written WGSL variant of sprite.vert.glsl.
//
// Direct WGSL bypasses the shadertranslate.GLSLToWGSLVertex path, which
// doesn't recognise `layout(std140) uniform <Block> { ... }` syntax and
// silently emits bare `uProjection` references that fail wgpu's parser
// with "no definition in scope for identifier: uProjection". With this
// variant + the WGSL branch in sprite_shader.go, WebGPU desktop loads
// the engine's built-in sprite pipeline cleanly.
//
// Uniform layout matches builtin.SpriteUniformLayout (uProjection at 0,
// uColorBody at 64, uColorTranslation at 128). Both stages declare the
// same Uniforms struct so the engine's combined-uniform packer fills
// the buffer once and both stages read the same bytes.

struct VertexInput {
    @location(0) aPosition: vec2<f32>,
    @location(1) aTexCoord: vec2<f32>,
    @location(2) aColor: vec4<f32>,
};

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) vTexCoord: vec2<f32>,
    @location(1) vColor: vec4<f32>,
};

struct Uniforms {
    uProjection:       mat4x4<f32>,
    uColorBody:        mat4x4<f32>,
    uColorTranslation: vec4<f32>,
};

@group(0) @binding(0) var<uniform> uniforms: Uniforms;

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    out.vTexCoord = in.aTexCoord;
    out.vColor = in.aColor;
    out.position = uniforms.uProjection * vec4<f32>(in.aPosition, 0.0, 1.0);
    return out;
}
