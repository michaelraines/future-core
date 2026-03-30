//go:build !windows || soft

package dx12

import "github.com/michaelraines/future-core/internal/backend"

// Shader implements backend.Shader for DirectX 12.
// Models compiled HLSL bytecode (DXBC or DXIL) for vertex and pixel shaders.
type Shader struct {
	backend.Shader // delegates all Shader methods to inner
}
