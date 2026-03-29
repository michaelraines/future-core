//go:build (darwin || linux || freebsd || windows) && !soft

package shaderc

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestSPIRVBindingsNoTexture(t *testing.T) {
	frag := `#version 330 core
in vec4 vColor;
uniform mat4 uColorBody;
uniform vec4 uColorTranslation;
out vec4 fragColor;
void main() { fragColor = uColorBody * vColor + uColorTranslation; }
`
	fspv, err := CompileGLSL(frag, StageFragment)
	if err != nil {
		t.Fatalf("fragment: %v", err)
	}
	fmt.Println("=== Fragment SPIR-V (no texture) ===")
	dumpBindings2(fspv)
}

func dumpBindings2(spv []byte) {
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
