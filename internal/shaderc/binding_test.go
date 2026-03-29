//go:build (darwin || linux || freebsd || windows) && !soft

package shaderc

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestSPIRVBindings(t *testing.T) {
	vert := `#version 330 core
layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;
uniform mat4 uProjection;
out vec2 vTexCoord;
out vec4 vColor;
void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`
	frag := `#version 330 core
in vec2 vTexCoord;
in vec4 vColor;
uniform sampler2D uTexture;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() {
    vec4 c = texture(uTexture, vTexCoord) * vColor;
    fragColor = uColorBody * c + uColorTranslation;
}
`
	vspv, err := CompileGLSL(vert, StageVertex)
	if err != nil {
		t.Fatalf("vertex: %v", err)
	}
	fspv, err := CompileGLSL(frag, StageFragment)
	if err != nil {
		t.Fatalf("fragment: %v", err)
	}

	fmt.Println("=== Vertex SPIR-V ===")
	dumpBindings(vspv)
	fmt.Println("=== Fragment SPIR-V ===")
	dumpBindings(fspv)
}

func dumpBindings(spv []byte) {
	words := make([]uint32, len(spv)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(spv[i*4:])
	}
	names := map[uint32]string{}
	i := 5
	for i < len(words) {
		op := words[i] & 0xFFFF
		wc := words[i] >> 16
		if wc == 0 {
			break
		}
		if op == 5 && wc >= 3 {
			id := words[i+1]
			var bs []byte
			for w := i + 2; w < i+int(wc); w++ {
				b := words[w]
				for shift := 0; shift < 32; shift += 8 {
					c := byte(b >> shift)
					if c == 0 {
						goto done
					}
					bs = append(bs, c)
				}
			}
		done:
			names[id] = string(bs)
		}
		if op == 71 && wc >= 4 {
			id := words[i+1]
			dec := words[i+2]
			val := words[i+3]
			dn := ""
			switch dec {
			case 33:
				dn = "Binding"
			case 34:
				dn = "DescriptorSet"
			}
			if dn != "" {
				name := names[id]
				if name == "" {
					name = fmt.Sprintf("%%id%d", id)
				}
				fmt.Printf("  %s: %s = %d\n", name, dn, val)
			}
		}
		i += int(wc)
	}
}
