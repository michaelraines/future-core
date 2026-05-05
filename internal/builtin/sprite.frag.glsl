#version 450 core

in vec2 vTexCoord;
in vec4 vColor;

uniform sampler2D uTexture;

// Shared std140 UBO — see sprite.vert.glsl for the why. Both stages
// must declare the same block so shaderc emits matching offsets and
// the engine's combined SpriteUniformLayout (uProjection=0,
// uColorBody=64, uColorTranslation=128) addresses the same bytes
// from both stages.
layout(std140, binding = 0) uniform UBO {
    mat4 uProjection;
    mat4 uColorBody;
    vec4 uColorTranslation;
};

out vec4 fragColor;

void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
