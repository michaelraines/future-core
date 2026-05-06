// sprite fragment shader — hand-written WGSL variant of sprite.frag.glsl.
// See sprite.vert.wgsl for why the translator can't produce this.

struct FragmentInput {
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
@group(1) @binding(0) var uTexture: texture_2d<f32>;
@group(1) @binding(1) var uTexture_sampler: sampler;

@fragment
fn fs_main(in: FragmentInput) -> @location(0) vec4<f32> {
    let c = textureSample(uTexture, uTexture_sampler, in.vTexCoord) * in.vColor;
    return uniforms.uColorBody * c + uniforms.uColorTranslation;
}
