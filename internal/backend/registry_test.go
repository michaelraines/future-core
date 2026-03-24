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
func (d *dummyDevice) ReadScreen(_ []byte) bool                        { return false }
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
func (d *dummyDevice) Encoder() CommandEncoder                            { return nil }

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

func TestResolveExplicitName(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("mybackend", func() Device { return &dummyDevice{} })

	dev, name, err := Resolve("mybackend", []string{"other"})
	require.NoError(t, err)
	require.NotNil(t, dev)
	require.Equal(t, "mybackend", name)
}

func TestResolveExplicitNameNotRegistered(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	_, _, err := Resolve("missing", []string{"other"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
}

func TestResolveAutoFirstPreferred(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("first", func() Device { return &dummyDevice{} })
	Register("second", func() Device { return &dummyDevice{} })

	dev, name, err := Resolve("auto", []string{"first", "second"})
	require.NoError(t, err)
	require.NotNil(t, dev)
	require.Equal(t, "first", name)
}

func TestResolveAutoSkipsUnregistered(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	Register("second", func() Device { return &dummyDevice{} })

	dev, name, err := Resolve("auto", []string{"first", "second"})
	require.NoError(t, err)
	require.NotNil(t, dev)
	require.Equal(t, "second", name)
}

func TestResolveAutoNoneAvailable(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	_, _, err := Resolve("auto", []string{"a", "b"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no preferred backend available")
}
