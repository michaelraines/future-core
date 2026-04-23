//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// kageEmittedSample mirrors the shape of what internal/shaderir/kage.go
// emits for a fragment shader that declares all four image slots but
// only samples from slot 0. The exact wording of the helper signatures
// and the trailing blank-line separator matches the emitter's output;
// if the emitter's format drifts the assertions here catch it before
// shaderc ever sees the drift.
const kageEmittedSample = `#version 450
precision highp float;

uniform sampler2D uTexture0;
uniform sampler2D uTexture1;
uniform sampler2D uTexture2;
uniform sampler2D uTexture3;

uniform vec4 uImageSrc0Origin;
uniform vec4 uImageSrc0Size;

vec4 imageSrc0At(vec2 pos) {
	return texture(uTexture0, pos);
}

vec4 imageSrc0UnsafeAt(vec2 pos) {
	return texture(uTexture0, pos);
}

vec4 imageSrc1At(vec2 pos) {
	return texture(uTexture1, pos);
}

vec4 imageSrc1UnsafeAt(vec2 pos) {
	return texture(uTexture1, pos);
}

vec4 imageSrc2At(vec2 pos) {
	return texture(uTexture2, pos);
}

vec4 imageSrc2UnsafeAt(vec2 pos) {
	return texture(uTexture2, pos);
}

vec4 imageSrc3At(vec2 pos) {
	return texture(uTexture3, pos);
}

vec4 imageSrc3UnsafeAt(vec2 pos) {
	return texture(uTexture3, pos);
}

void main() {
	gl_FragColor = imageSrc0At(vec2(0.5, 0.5));
}
`

func TestStripUnusedSamplers_RemovesUnusedSlots(t *testing.T) {
	out := stripUnusedSamplers(kageEmittedSample)

	// Slot 0 declaration and helpers must remain — the shader's main()
	// calls imageSrc0At.
	require.Contains(t, out, "uniform sampler2D uTexture0;")
	require.Contains(t, out, "vec4 imageSrc0At(vec2 pos) {")
	require.Contains(t, out, "vec4 imageSrc0UnsafeAt(vec2 pos) {")

	// Slots 1/2/3 — declarations AND both helper bodies must be gone.
	for _, n := range []string{"1", "2", "3"} {
		require.NotContains(t, out, "uniform sampler2D uTexture"+n+";",
			"slot %s declaration should have been stripped", n)
		require.NotContains(t, out, "vec4 imageSrc"+n+"At(vec2 pos) {",
			"slot %s imageSrcAt helper should have been stripped", n)
		require.NotContains(t, out, "vec4 imageSrc"+n+"UnsafeAt(vec2 pos) {",
			"slot %s imageSrcUnsafeAt helper should have been stripped", n)
		require.NotContains(t, out, "texture(uTexture"+n+",",
			"stripped slot %s must not leave a dangling sampler reference", n)
	}

	// User body untouched.
	require.Contains(t, out, "gl_FragColor = imageSrc0At(vec2(0.5, 0.5));")
}

func TestStripUnusedSamplers_KeepsUsedSlot(t *testing.T) {
	// If main() calls imageSrc2At, slot 2 must be kept even though
	// slots 1 and 3 are unused.
	src := strings.Replace(kageEmittedSample,
		"gl_FragColor = imageSrc0At(vec2(0.5, 0.5));",
		"gl_FragColor = imageSrc2At(vec2(0.5, 0.5));", 1)
	out := stripUnusedSamplers(src)

	require.Contains(t, out, "uniform sampler2D uTexture2;")
	require.Contains(t, out, "vec4 imageSrc2At(vec2 pos) {")
	require.NotContains(t, out, "uniform sampler2D uTexture1;")
	require.NotContains(t, out, "uniform sampler2D uTexture3;")
}

func TestStripUnusedSamplers_UnsafeAtAlsoCountsAsUse(t *testing.T) {
	// imageSrcNUnsafeAt usage should pin slot N.
	src := strings.Replace(kageEmittedSample,
		"gl_FragColor = imageSrc0At(vec2(0.5, 0.5));",
		"gl_FragColor = imageSrc1UnsafeAt(vec2(0.5, 0.5));", 1)
	out := stripUnusedSamplers(src)

	require.Contains(t, out, "uniform sampler2D uTexture1;")
	require.Contains(t, out, "vec4 imageSrc1UnsafeAt(vec2 pos) {")
}

func TestStripUnusedSamplers_NeverStripsSlot0(t *testing.T) {
	// Slot 0 is never stripped by policy, even when unused, because the
	// Kage convention binds the primary texture there. Use a body that
	// references no slots and confirm slot 0 declarations stay put.
	src := strings.Replace(kageEmittedSample,
		"gl_FragColor = imageSrc0At(vec2(0.5, 0.5));",
		"gl_FragColor = vec4(1.0);", 1)
	out := stripUnusedSamplers(src)

	require.Contains(t, out, "uniform sampler2D uTexture0;")
	require.Contains(t, out, "vec4 imageSrc0At(vec2 pos) {")
}

func TestStripUnusedSamplers_NoMainReturnsInputUnchanged(t *testing.T) {
	// Guard against the edge where the emitter didn't include main().
	input := "uniform sampler2D uTexture1;\n"
	require.Equal(t, input, stripUnusedSamplers(input))
}

func TestStripSamplerSlot_BailsIfTerminatorMissing(t *testing.T) {
	// If the emitter's trailing "}\n\n" separator ever drifts, the
	// rewrite should leave the whole source unchanged rather than
	// partially strip (leaving a dangling sampler reference).
	drifted := `uniform sampler2D uTexture1;

vec4 imageSrc1At(vec2 pos) {
	return texture(uTexture1, pos);
}
// NOTE: missing trailing blank line before next helper starts
vec4 imageSrc1UnsafeAt(vec2 pos) {
	return texture(uTexture1, pos);
}`
	require.Equal(t, drifted, stripSamplerSlot(drifted, "1"))
}

func TestStripSamplerSlot_NoDeclarationReturnsUnchanged(t *testing.T) {
	// If the declaration isn't present at all, stripSamplerSlot should
	// return the source unchanged (don't strip orphan helpers).
	src := `vec4 imageSrc1At(vec2 pos) { return vec4(0); }

`
	require.Equal(t, src, stripSamplerSlot(src, "1"))
}
