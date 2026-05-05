#version 450 core

layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;

// Single std140 UBO shared across vertex + fragment stages so the
// engine's per-draw uniform packer (one layout, written to both
// stages' buffers) sees consistent member offsets. Without this,
// shaderc's auto-bind packs each stage's loose uniforms into
// separate per-stage UBOs with different layouts — the fragment
// shader then reads uColorBody at offset 0 (vec4-aligned) but the
// engine packs it at offset 64 (where the fragment expects
// nothing), producing garbage matrices and black-fragment output.
// Manifested as scene-selector rendering black on Adreno because
// the backend skipped per-stage layout extraction; desktop didn't
// catch it because the GLSL path runs ExtractUniformLayout on each
// stage source separately.
layout(std140, binding = 0) uniform UBO {
    mat4 uProjection;
    mat4 uColorBody;
    vec4 uColorTranslation;
};

out vec2 vTexCoord;
out vec4 vColor;

void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
