package webgl

import "github.com/michaelraines/future-core/internal/backend"

// WebGL2 constant equivalents. These mirror the real WebGL2 GLenum values
// that a syscall/js implementation would use.
const (
	glTexture2D = 0x0DE1
	glRGBA      = 0x1908
	glRGB       = 0x1907
	glRed       = 0x1903 // WebGL2 RED format
	glRGBA16F   = 0x881A
	glRGBA32F   = 0x8814
	glDepth24   = 0x81A6 // DEPTH_COMPONENT24
	glDepth32F  = 0x8CAC // DEPTH_COMPONENT32F

	glArrayBuffer        = 0x8892
	glElementArrayBuffer = 0x8893
	glUniformBuffer      = 0x8A11
)

// glFormatFromTextureFormat maps backend texture formats to WebGL2 internal format constants.
func glFormatFromTextureFormat(f backend.TextureFormat) int {
	switch f {
	case backend.TextureFormatRGBA8:
		return glRGBA
	case backend.TextureFormatRGB8:
		return glRGB
	case backend.TextureFormatR8:
		return glRed
	case backend.TextureFormatRGBA16F:
		return glRGBA16F
	case backend.TextureFormatRGBA32F:
		return glRGBA32F
	case backend.TextureFormatDepth24:
		return glDepth24
	case backend.TextureFormatDepth32F:
		return glDepth32F
	default:
		return glRGBA
	}
}

// glUsageFromBufferUsage maps backend buffer usage to WebGL2 buffer target constants.
func glUsageFromBufferUsage(u backend.BufferUsage) int {
	switch u {
	case backend.BufferUsageVertex:
		return glArrayBuffer
	case backend.BufferUsageIndex:
		return glElementArrayBuffer
	case backend.BufferUsageUniform:
		return glUniformBuffer
	default:
		return glArrayBuffer
	}
}

// translateGLSLES rewrites GLSL 330 core source as GLSL ES 3.00, the
// dialect WebGL2 accepts. The transformations are limited to the
// minimum needed for the engine's hand-written shaders — the
// in/out/uniform/layout(location=) syntax is shared, so only the
// version directive and ES-mandatory precision qualifiers need
// adjustment.
//
// Specifically:
//   - #version 330 core → #version 300 es (accept either case)
//   - Insert default precision qualifiers immediately after the version
//     line, gated to fragment shaders by detecting fragColor / out vec4
//     / in vec2 vTexCoord-style declarations. Vertex shaders default to
//     highp on every WebGL2 implementation; fragment shaders do not.
//
// Hand-written shaders that already include #version 300 es or explicit
// precision qualifiers pass through with only the version line
// normalised, so adding hand-written .glsles variants later doesn't
// double-prefix.
func translateGLSLES(source string) string {
	if source == "" {
		return source
	}

	// Normalise the version directive. Accept #version 330 core, #version
	// 330, #version 300 es. Always emit #version 300 es as the first
	// non-empty line so WebGL2 parses it correctly.
	out := source
	for _, prefix := range []string{
		"#version 330 core",
		"#version 330",
		"#version 300 core",
		"#version 300 es",
	} {
		if hasLinePrefix(out, prefix) {
			out = replaceLinePrefix(out, prefix, "#version 300 es")
			break
		}
	}
	if !hasLinePrefix(out, "#version 300 es") {
		out = "#version 300 es\n" + out
	}

	// Detect fragment shader by presence of an `out` declaration that
	// looks like a fragment color sink, OR explicit fragColor identifier.
	// Vertex shaders write to gl_Position and don't need fragment
	// precision defaults.
	isFrag := containsToken(out, "fragColor") ||
		containsToken(out, "FragColor") ||
		containsToken(out, "gl_FragColor")

	// Insert default precision qualifiers if absent.
	if isFrag && !containsToken(out, "precision highp float") &&
		!containsToken(out, "precision mediump float") {
		out = injectAfterVersion(out,
			"precision highp float;\n"+
				"precision highp int;\n"+
				"precision highp sampler2D;\n")
	}

	return out
}

// hasLinePrefix reports whether `s` has a line that starts with `prefix`.
func hasLinePrefix(s, prefix string) bool {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return true
	}
	for i := 0; i+len(prefix) <= len(s); i++ {
		if s[i] == '\n' && i+1+len(prefix) <= len(s) && s[i+1:i+1+len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// replaceLinePrefix replaces the first occurrence of `prefix` (anchored
// at the start of a line) with `replacement`.
func replaceLinePrefix(s, prefix, replacement string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return replacement + s[len(prefix):]
	}
	for i := 0; i+len(prefix) <= len(s); i++ {
		if s[i] == '\n' && i+1+len(prefix) <= len(s) && s[i+1:i+1+len(prefix)] == prefix {
			return s[:i+1] + replacement + s[i+1+len(prefix):]
		}
	}
	return s
}

// containsToken reports whether `s` contains the substring `tok`. Used
// for quick keyword presence checks; not a full GLSL parser.
func containsToken(s, tok string) bool {
	if tok == "" || len(s) < len(tok) {
		return false
	}
	for i := 0; i+len(tok) <= len(s); i++ {
		if s[i:i+len(tok)] == tok {
			return true
		}
	}
	return false
}

// injectAfterVersion inserts `block` immediately after the first line
// in `s`, which is expected to be the #version directive.
func injectAfterVersion(s, block string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i+1] + block + s[i+1:]
		}
	}
	return s + "\n" + block
}
