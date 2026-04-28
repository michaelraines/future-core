package backend

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShaderLanguageString(t *testing.T) {
	tests := []struct {
		lang ShaderLanguage
		want string
	}{
		{ShaderLanguageKage, "kage"},
		{ShaderLanguageGLSL, "glsl"},
		{ShaderLanguageGLSLES, "glsles"},
		{ShaderLanguageWGSL, "wgsl"},
		{ShaderLanguageMSL, "msl"},
		{ShaderLanguageSPIRV, "spirv"},
		{ShaderLanguageHLSL, "hlsl"},
		{ShaderLanguage(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.lang.String())
		})
	}
}

// TestNativeShaderDeviceInterface confirms the NativeShaderDevice
// interface contract is well-formed: an implementing type can be
// type-asserted from a Device, and ErrUnsupportedShaderLanguage is a
// sentinel error usable with errors.Is.
func TestNativeShaderDeviceInterface(t *testing.T) {
	var dev Device = &nativeFake{lang: ShaderLanguageWGSL}

	nsd, ok := dev.(NativeShaderDevice)
	require.True(t, ok, "fake device should implement NativeShaderDevice")
	require.Equal(t, ShaderLanguageWGSL, nsd.PreferredShaderLanguage())

	_, err := nsd.NewShaderNative(NativeShaderDescriptor{
		Language: ShaderLanguageMSL, // mismatched on purpose
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnsupportedShaderLanguage),
		"language mismatch must wrap ErrUnsupportedShaderLanguage")

	sh, err := nsd.NewShaderNative(NativeShaderDescriptor{
		Language: ShaderLanguageWGSL,
		Vertex:   []byte("// vertex"),
		Fragment: []byte("// fragment"),
	})
	require.NoError(t, err)
	require.NotNil(t, sh)
}

// TestNonNativeDeviceSkipsInterface confirms that a Device without the
// optional NativeShaderDevice methods cleanly fails the type assertion
// — this is what callers in the future framework rely on to fall back
// to the Kage path.
func TestNonNativeDeviceSkipsInterface(t *testing.T) {
	var dev Device = &nonNativeFake{}
	_, ok := dev.(NativeShaderDevice)
	require.False(t, ok, "non-implementing device must not satisfy NativeShaderDevice")
}

// --- fakes ---

type nativeFake struct {
	nonNativeFake
	lang ShaderLanguage
}

func (f *nativeFake) PreferredShaderLanguage() ShaderLanguage { return f.lang }

func (f *nativeFake) NewShaderNative(desc NativeShaderDescriptor) (Shader, error) {
	if desc.Language != f.lang {
		return nil, ErrUnsupportedShaderLanguage
	}
	return &fakeShader{}, nil
}

type nonNativeFake struct{}

func (nonNativeFake) Init(DeviceConfig) error                       { return nil }
func (nonNativeFake) Dispose()                                      {}
func (nonNativeFake) BeginFrame()                                   {}
func (nonNativeFake) EndFrame()                                     {}
func (nonNativeFake) NewTexture(TextureDescriptor) (Texture, error) { return nil, nil }
func (nonNativeFake) NewBuffer(BufferDescriptor) (Buffer, error)    { return nil, nil }
func (nonNativeFake) NewShader(ShaderDescriptor) (Shader, error)    { return nil, nil }
func (nonNativeFake) NewRenderTarget(RenderTargetDescriptor) (RenderTarget, error) {
	return nil, nil
}
func (nonNativeFake) NewPipeline(PipelineDescriptor) (Pipeline, error) { return nil, nil }
func (nonNativeFake) Capabilities() DeviceCapabilities                 { return DeviceCapabilities{} }
func (nonNativeFake) Encoder() CommandEncoder                          { return nil }
func (nonNativeFake) ReadScreen([]byte) bool                           { return false }

type fakeShader struct{}

func (fakeShader) SetUniformFloat(string, float32)    {}
func (fakeShader) SetUniformVec2(string, [2]float32)  {}
func (fakeShader) SetUniformVec3(string, [3]float32)  {}
func (fakeShader) SetUniformVec4(string, [4]float32)  {}
func (fakeShader) SetUniformMat4(string, [16]float32) {}
func (fakeShader) SetUniformInt(string, int32)        {}
func (fakeShader) SetUniformBlock(string, []byte)     {}
func (fakeShader) PackCurrentUniforms() []byte        { return nil }
func (fakeShader) Dispose()                           {}
