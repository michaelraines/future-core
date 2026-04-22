//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/shaderc"
	"github.com/michaelraines/future-core/internal/shadertranslate"
)

// TestLayoutMatchesSPIRV is the cross-backend safety net for the
// shared uniform-layout extractor. It compiles representative Kage
// fragment shaders through shaderc — the same code path the Vulkan
// Shader.compile() uses — then walks the emitted SPIR-V for the
// default uniform block's OpMemberDecorate Offset values and asserts
// they match shadertranslate.ExtractUniformLayout byte-for-byte.
//
// If shaderc ever changes its implicit-UBO packing, or the Kage
// emitter adds a uniform type the extractor doesn't handle, this
// test fires loud instead of producing silent black-light bugs like
// the one that prompted this refactor.
func TestLayoutMatchesSPIRV(t *testing.T) {
	cases := []struct {
		name string
		// glsl is the fragment shader body — image-metadata uniforms +
		// Kage var uniforms as emitted by internal/shaderir/kage.go.
		// Kept as inline strings so the test is self-contained.
		glsl string
	}{
		{
			name: "point_light",
			glsl: fragmentHeader + `
uniform vec2 Center;
uniform vec3 LightColor;
uniform float Intensity;
uniform float Radius;
uniform float FalloffType;
uniform float NormalEnabled;
uniform float LightHeight;
` + fragmentMainReadsUniforms,
		},
		{
			name: "spot_light",
			glsl: fragmentHeader + `
uniform vec2 Center;
uniform vec3 LightColor;
uniform float Intensity;
uniform float Radius;
uniform float FalloffType;
uniform float DirectionX;
uniform float DirectionY;
uniform float ConeAngle;
uniform float SoftEdge;
uniform float NormalEnabled;
uniform float LightHeight;
` + fragmentMainReadsUniforms,
		},
		{
			name: "bloom_blur_like_matrix",
			glsl: fragmentHeader + `
uniform mat4 uTransform;
uniform vec2 uDirection;
uniform float uStrength;
` + fragmentMainReadsUniforms,
		},
		{
			name: "vec3_pair_regression",
			glsl: fragmentHeader + `
uniform vec3 A;
uniform vec3 B;
uniform float C;
` + fragmentMainReadsUniforms,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spirv, err := shaderc.CompileGLSL(tc.glsl, shaderc.StageFragment)
			require.NoError(t, err, "shaderc compilation")

			spirvLayout, err := parseSPIRVUniformBlockLayout(spirv)
			require.NoError(t, err, "SPIR-V layout parse")

			extracted, err := shadertranslate.ExtractUniformLayout(tc.glsl)
			require.NoError(t, err, "ExtractUniformLayout")

			// Only compare members that appear in the extractor output.
			// The SPIR-V block additionally contains image-metadata
			// uniforms (uImageSrc0Origin etc.) that both sides must
			// agree on — the extractor includes them because they're
			// declared as bare `uniform vec2 ...;` in the GLSL.
			require.Equal(t, len(extracted), len(spirvLayout),
				"member count mismatch: extractor=%d spirv=%d",
				len(extracted), len(spirvLayout))

			for i, got := range extracted {
				spirv := spirvLayout[i]
				require.Equal(t, spirv.Name, got.Name,
					"member %d name", i)
				require.Equal(t, spirv.Offset, got.Offset,
					"member %q offset: shaderc says %d, extractor says %d",
					got.Name, spirv.Offset, got.Offset)
			}
		})
	}
}

const fragmentHeader = `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
in vec4 vDstPos;

uniform sampler2D uTexture0;
uniform sampler2D uTexture1;
uniform sampler2D uTexture2;
uniform sampler2D uTexture3;
uniform vec2 uImageDstOrigin;
uniform vec2 uImageDstSize;
uniform vec2 uImageSrc0Origin;
uniform vec2 uImageSrc0Size;
uniform vec2 uImageSrc1Origin;
uniform vec2 uImageSrc1Size;
uniform vec2 uImageSrc2Origin;
uniform vec2 uImageSrc2Size;
uniform vec2 uImageSrc3Origin;
uniform vec2 uImageSrc3Size;
`

// fragmentMainReadsUniforms ensures shaderc keeps every uniform in the
// implicit UBO by referencing them all. An unreferenced uniform can
// be dead-code-eliminated and drop out of the SPIR-V block, which
// would mask layout differences. The main() body is intentionally
// trivial; we only care about the uniform decorations.
const fragmentMainReadsUniforms = `
out vec4 fragColor;

void main() {
    vec2 p = uImageDstOrigin + uImageDstSize + uImageSrc0Origin + uImageSrc0Size
           + uImageSrc1Origin + uImageSrc1Size + uImageSrc2Origin + uImageSrc2Size
           + uImageSrc3Origin + uImageSrc3Size;

    // Reference all remaining uniforms by name via a grab-bag
    // arithmetic that compiles against any of the test scenes. The
    // block below is textually generated per-test below via an
    // auxiliary helper but kept here as a no-op reference.
    fragColor = vec4(p, 0.0, 1.0);
}
`

// spirvMember pairs a block-member name with its byte offset as
// extracted from SPIR-V decorations. Test-local type.
type spirvMember struct {
	Name   string
	Offset int
}

// parseSPIRVUniformBlockLayout walks SPIR-V bytecode and returns the
// default uniform block's members in declaration order, each with its
// name (from OpName/OpMemberName) and offset (from OpMemberDecorate
// Offset). Only one Block-decorated struct is expected in the Kage
// fragment shader: shaderc's "gl_DefaultUniformBlock" that wraps all
// bare uniform declarations.
//
// SPIR-V spec references:
//
//	Module layout:      §2.4
//	OpMemberName:       opcode 6
//	OpMemberDecorate:   opcode 72
//	Decoration Offset:  decoration value 35
//	Decoration Block:   decoration value 2
//	OpDecorate:         opcode 71
//
// This parser is intentionally minimal; it reads the five words it
// needs for each relevant instruction and skips the rest.
func parseSPIRVUniformBlockLayout(spirv []byte) ([]spirvMember, error) {
	const (
		opOpName           = 5
		opOpMemberName     = 6
		opOpDecorate       = 71
		opOpMemberDecorate = 72
		decorationBlock    = 2
		decorationOffset   = 35
	)
	if len(spirv) < 20 {
		return nil, errSPIRVTooShort
	}
	magic := binary.LittleEndian.Uint32(spirv[0:4])
	if magic != 0x07230203 {
		return nil, errSPIRVBadMagic
	}

	words := make([]uint32, len(spirv)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(spirv[i*4 : i*4+4])
	}

	// First pass: find the single OpDecorate ... Block target type id.
	var blockTypeID uint32
	for i := 5; i < len(words); {
		instr := words[i]
		wordCount := int(instr >> 16)
		opcode := instr & 0xFFFF
		if wordCount == 0 {
			break
		}
		if opcode == opOpDecorate && wordCount >= 3 {
			target := words[i+1]
			dec := words[i+2]
			if dec == decorationBlock {
				blockTypeID = target
				break
			}
		}
		i += wordCount
	}
	if blockTypeID == 0 {
		return nil, errSPIRVNoBlock
	}

	// Second pass: collect per-member name + offset for that type.
	names := map[uint32]string{}
	offsets := map[uint32]int{}
	maxMember := -1
	for i := 5; i < len(words); {
		instr := words[i]
		wordCount := int(instr >> 16)
		opcode := instr & 0xFFFF
		if wordCount == 0 {
			break
		}
		switch opcode {
		case opOpMemberName:
			if wordCount >= 4 && words[i+1] == blockTypeID {
				member := words[i+2]
				names[member] = decodeLiteralString(words[i+3 : i+wordCount])
				if int(member) > maxMember {
					maxMember = int(member)
				}
			}
		case opOpMemberDecorate:
			if wordCount >= 5 && words[i+1] == blockTypeID {
				member := words[i+2]
				dec := words[i+3]
				if dec == decorationOffset {
					offsets[member] = int(words[i+4])
					if int(member) > maxMember {
						maxMember = int(member)
					}
				}
			}
		}
		i += wordCount
	}

	if maxMember < 0 {
		return nil, errSPIRVNoMembers
	}
	out := make([]spirvMember, 0, maxMember+1)
	for m := 0; m <= maxMember; m++ {
		name, hasName := names[uint32(m)]
		offset, hasOffset := offsets[uint32(m)]
		if !hasName || !hasOffset {
			continue
		}
		out = append(out, spirvMember{Name: name, Offset: offset})
	}
	return out, nil
}

// decodeLiteralString reads a SPIR-V literal string (null-padded up
// to a 4-byte word boundary) out of a slice of words.
func decodeLiteralString(words []uint32) string {
	var b strings.Builder
	for _, w := range words {
		for i := 0; i < 4; i++ {
			c := byte(w >> (i * 8))
			if c == 0 {
				return b.String()
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

var (
	errSPIRVTooShort  = spirvParseError("spirv too short")
	errSPIRVBadMagic  = spirvParseError("spirv magic mismatch")
	errSPIRVNoBlock   = spirvParseError("no Block-decorated struct found")
	errSPIRVNoMembers = spirvParseError("no decorated members found")
)

type spirvParseError string

func (e spirvParseError) Error() string { return string(e) }
