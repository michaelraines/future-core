#version 330 core

in vec2 vTexCoord;
in vec4 vColor;

uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;

out vec4 fragColor;

void main() {
    // No rgb=min(rgb,a) clamp — ColorScale.ScaleAlpha scales all four
    // channels (matching Ebitengine), so vertex colors arrive correctly
    // premultiplied. See engine_js.go for the full history.
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
