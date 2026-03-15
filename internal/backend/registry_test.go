package backend

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// dummyDevice is a minimal Device for registry testing.
type dummyDevice struct{}

func (d *dummyDevice) Init(_ DeviceConfig) error                       { return nil }
func (d *dummyDevice) Dispose()                                        {}
func (d *dummyDevice) BeginFrame()                                     {}
func (d *dummyDevice) EndFrame()                                       {}
func (d *dummyDevice) NewTexture(_ TextureDescriptor) (Texture, error) { return nil, nil }
func (d *dummyDevice) NewBuffer(_ BufferDescriptor) (Buffer, error)    { return nil, nil }
func (d *dummyDevice) NewShader(_ ShaderDescriptor) (Shader, error)    { return nil, nil }
func (d *dummyDevice) NewRenderTarget(_ RenderTargetDescriptor) (RenderTarget, error) {
	return nil, nil
}
func (d *dummyDevice) NewPipeline(_ PipelineDescriptor) (Pipeline, error) { return nil, nil }
func (d *dummyDevice) Capabilities() DeviceCapabilities                   { return DeviceCapabilities{} }

func TestRegisterAndCreate(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("test-backend", func() Device { return &dummyDevice{} })

	dev, err := Create("test-backend")
	require.NoError(t, err)
	require.NotNil(t, dev)
	require.IsType(t, &dummyDevice{}, dev)
}

func TestCreateUnknownBackend(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	dev, err := Create("nonexistent")
	require.Error(t, err)
	require.Nil(t, dev)
	require.Contains(t, err.Error(), "nonexistent")
}

func TestRegisterDuplicate(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("dup", func() Device { return &dummyDevice{} })
	require.Panics(t, func() {
		Register("dup", func() Device { return &dummyDevice{} })
	})
}

func TestAvailable(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("alpha", func() Device { return &dummyDevice{} })
	Register("beta", func() Device { return &dummyDevice{} })

	names := Available()
	sort.Strings(names)
	require.Equal(t, []string{"alpha", "beta"}, names)
}

func TestIsRegistered(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("exists", func() Device { return &dummyDevice{} })

	require.True(t, IsRegistered("exists"))
	require.False(t, IsRegistered("nope"))
}
