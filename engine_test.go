package futurerender

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// stubGame is a minimal Game implementation for testing.
type stubGame struct{}

func (g *stubGame) Update() error                                   { return nil }
func (g *stubGame) Draw(_ *Image)                                   {}
func (g *stubGame) Layout(_, _ int) (screenWidth, screenHeight int) { return 320, 240 }

func withNilEngine(t *testing.T) {
	t.Helper()
	old := getEngine()
	setEngine(nil)
	t.Cleanup(func() { setEngine(old) })
}

func TestSetWindowSizeNilEngine(t *testing.T) {
	withNilEngine(t)
	// Should not panic with nil engine.
	SetWindowSize(1024, 768)
	// Verify pending state was updated.
	require.Equal(t, 1024, pendingWindowWidth)
	require.Equal(t, 768, pendingWindowHeight)
	// Restore defaults.
	t.Cleanup(func() {
		pendingWindowWidth = 800
		pendingWindowHeight = 600
	})
}

func TestSetWindowTitleNilEngine(t *testing.T) {
	withNilEngine(t)
	SetWindowTitle("Test Title")
	require.Equal(t, "Test Title", pendingWindowTitle)
	t.Cleanup(func() {
		pendingWindowTitle = "Future Render"
	})
}

func TestSetFullscreenNilEngine(t *testing.T) {
	withNilEngine(t)
	// Should not panic.
	SetFullscreen(true)
}

func TestIsFullscreenNilEngine(t *testing.T) {
	withNilEngine(t)
	require.False(t, IsFullscreen())
}

func TestMaxTPSDefault(t *testing.T) {
	require.Equal(t, 60, MaxTPS())
}

func TestSetMaxTPS(t *testing.T) {
	old := MaxTPS()
	defer SetMaxTPS(old)

	SetMaxTPS(120)
	require.Equal(t, 120, MaxTPS())
}

func TestSetMaxTPSNegativeClampsToZero(t *testing.T) {
	old := MaxTPS()
	defer SetMaxTPS(old)

	SetMaxTPS(-10)
	require.Equal(t, 0, MaxTPS())
}

func TestIsVsyncEnabledNilEngine(t *testing.T) {
	withNilEngine(t)
	require.True(t, IsVsyncEnabled())
}

func TestSetVsyncEnabledNilEngine(t *testing.T) {
	withNilEngine(t)
	// Should not panic.
	SetVsyncEnabled(false)
}

func TestCurrentFPSNilEngine(t *testing.T) {
	withNilEngine(t)
	require.InDelta(t, 0.0, CurrentFPS(), 1e-6)
}

func TestCurrentTPSNilEngine(t *testing.T) {
	withNilEngine(t)
	require.InDelta(t, 0.0, CurrentTPS(), 1e-6)
}

func TestSetCursorModeNilEngine(t *testing.T) {
	withNilEngine(t)
	// Should not panic.
	SetCursorMode(CursorModeHidden)
}

func TestDeviceScaleFactorNilEngine(t *testing.T) {
	withNilEngine(t)
	require.InDelta(t, 1.0, DeviceScaleFactor(), 1e-6)
}

func TestNewPlatformEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	e := newPlatformEngine(game)
	require.NotNil(t, e)
	require.Equal(t, game, e.game)
}

func TestEngineRunReturnsError(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	e := newPlatformEngine(game)
	err := e.run()
	require.NotNil(t, err, "run() should return an error without platform backend")
}

func TestNewEngineSetGlobalEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	e := newEngine(game)
	require.Equal(t, e, getEngine())
}

func TestErrTermination(t *testing.T) {
	require.NotNil(t, ErrTermination)
	require.Equal(t, "game terminated", ErrTermination.Error())
}

func TestSetWindowSizeWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	SetWindowSize(640, 480)
	require.Equal(t, 640, pendingWindowWidth)
	require.Equal(t, 480, pendingWindowHeight)
	t.Cleanup(func() {
		pendingWindowWidth = 800
		pendingWindowHeight = 600
	})
}

func TestSetWindowTitleWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	SetWindowTitle("Hello")
	require.Equal(t, "Hello", pendingWindowTitle)
	t.Cleanup(func() {
		pendingWindowTitle = "Future Render"
	})
}

func TestIsFullscreenWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	require.False(t, IsFullscreen())
	SetFullscreen(true)
	// Stub engine isFullscreen always returns false.
	require.False(t, IsFullscreen())
}

func TestIsVsyncEnabledWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	require.True(t, IsVsyncEnabled())
	SetVsyncEnabled(false)
	// Stub engine isVSync always returns true.
	require.True(t, IsVsyncEnabled())
}

func TestCurrentFPSWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	require.InDelta(t, 0.0, CurrentFPS(), 1e-6)
}

func TestCurrentTPSWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	require.InDelta(t, 0.0, CurrentTPS(), 1e-6)
}

func TestDeviceScaleFactorWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	require.InDelta(t, 1.0, DeviceScaleFactor(), 1e-6)
}

func TestSetCursorModeWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	setEngine(newPlatformEngine(game))

	// Should not panic.
	SetCursorMode(CursorModeCaptured)
}

func TestCursorModeConstants(t *testing.T) {
	require.Equal(t, CursorMode(0), CursorModeVisible)
	require.Equal(t, CursorMode(1), CursorModeHidden)
	require.Equal(t, CursorMode(2), CursorModeCaptured)
}

func TestScreenClearedEveryFrameDefault(t *testing.T) {
	require.True(t, IsScreenClearedEveryFrame())
}

func TestBackendDefault(t *testing.T) {
	t.Setenv("FUTURE_RENDER_BACKEND", "")
	require.Equal(t, "auto", Backend())
}

func TestBackendEnvVar(t *testing.T) {
	t.Setenv("FUTURE_RENDER_BACKEND", "opengl")
	require.Equal(t, "opengl", Backend())
}

func TestSetScreenClearedEveryFrame(t *testing.T) {
	defer SetScreenClearedEveryFrame(true)

	SetScreenClearedEveryFrame(false)
	require.False(t, IsScreenClearedEveryFrame())

	SetScreenClearedEveryFrame(true)
	require.True(t, IsScreenClearedEveryFrame())
}
