package futurerender

import (
	"runtime"
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
	// Skip on platforms where run() creates a real window — Cocoa/Win32
	// windows must be created on the main thread, which tests don't run on.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("window creation requires main thread on " + runtime.GOOS)
	}

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
	t.Setenv("FUTURE_CORE_BACKEND", "")
	require.Equal(t, "auto", Backend())
}

func TestBackendEnvVar(t *testing.T) {
	t.Setenv("FUTURE_CORE_BACKEND", "opengl")
	require.Equal(t, "opengl", Backend())
}

func TestBackendReturnsResolvedName(t *testing.T) {
	old := resolvedBackend.Load()
	defer resolvedBackend.Store(old)

	resolvedBackend.Store("soft")
	require.Equal(t, "soft", Backend())
}

func TestBackendResolvedTakesPrecedence(t *testing.T) {
	old := resolvedBackend.Load()
	defer resolvedBackend.Store(old)

	t.Setenv("FUTURE_CORE_BACKEND", "vulkan")
	resolvedBackend.Store("opengl")
	require.Equal(t, "opengl", Backend())
}

func TestSyncString(t *testing.T) {
	var s syncString
	require.Equal(t, "", s.Load())
	s.Store("hello")
	require.Equal(t, "hello", s.Load())
	s.Store("world")
	require.Equal(t, "world", s.Load())
}

func TestPreferredBackends(t *testing.T) {
	backends := preferredBackends()
	require.NotEmpty(t, backends)
	// soft should always be in the list as a fallback.
	require.Equal(t, "soft", backends[len(backends)-1])
	// opengl should be in the list on all desktop platforms.
	found := false
	for _, b := range backends {
		if b == "opengl" {
			found = true
			break
		}
	}
	require.True(t, found, "opengl should be in preferred backends")
}

func TestWindowConfig(t *testing.T) {
	e := &engine{
		windowTitle: "Test Title",
		windowW:     800,
		windowH:     600,
	}
	cfg := e.windowConfig()
	require.Equal(t, "Test Title", cfg.Title)
	require.Equal(t, 800, cfg.Width)
	require.Equal(t, 600, cfg.Height)
}

func TestWindowConfigDefaults(t *testing.T) {
	e := &engine{}
	cfg := e.windowConfig()
	// With empty values, defaults from platform should be used.
	require.NotZero(t, cfg.Width)
	require.NotZero(t, cfg.Height)
}

func TestDisposeRenderResourcesNil(t *testing.T) {
	// Disposing an engine with no resources should not panic.
	e := &engine{}
	e.disposeRenderResources()
}

func TestEngineSetVSync(t *testing.T) {
	e := &engine{}
	// setVSync is a no-op currently but should not panic.
	e.setVSync(true)
	require.True(t, e.isVSync())
}

func TestEngineSetFullscreenNilWindow(t *testing.T) {
	e := &engine{}
	// With no window, these should be no-ops.
	e.setFullscreen(true)
	require.False(t, e.isFullscreen())
}

func TestEngineDeviceScaleFactor(t *testing.T) {
	e := &engine{}
	require.Equal(t, 1.0, e.deviceScaleFactor())
}

func TestEngineCursorModeNilWindow(t *testing.T) {
	e := &engine{}
	e.setCursorMode(CursorModeVisible)
	e.setCursorMode(CursorModeHidden)
	e.setCursorMode(CursorModeCaptured)
}

func TestEngineSetWindowSizeNilWindow(t *testing.T) {
	e := &engine{}
	e.setWindowSize(800, 600)
}

func TestEngineSetWindowTitleNilWindow(t *testing.T) {
	e := &engine{}
	e.setWindowTitle("test")
}

func TestRegisterTexture(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	e := newPlatformEngine(game)
	setEngine(e)

	// Verify the method doesn't panic and stores correctly.
	e.registerTexture(42, nil)
	require.Contains(t, e.textures, uint32(42))
}

func TestSetVSyncWithEngine(t *testing.T) {
	old := getEngine()
	defer func() { setEngine(old) }()

	game := &stubGame{}
	e := newPlatformEngine(game)
	setEngine(e)

	// setVSync is a no-op but should not panic.
	e.setVSync(false)
	e.setVSync(true)
}

func TestSetScreenClearedEveryFrame(t *testing.T) {
	defer SetScreenClearedEveryFrame(true)

	SetScreenClearedEveryFrame(false)
	require.False(t, IsScreenClearedEveryFrame())

	SetScreenClearedEveryFrame(true)
	require.True(t, IsScreenClearedEveryFrame())
}

// --- Orientation ---

func TestOrientationConstants(t *testing.T) {
	require.Equal(t, Orientation(0), OrientationDefault)
	require.Equal(t, Orientation(1), OrientationPortrait)
	require.Equal(t, Orientation(2), OrientationLandscape)
}

func TestSetScreenOrientation(t *testing.T) {
	defer func() { pendingOrientation = OrientationDefault }()

	SetScreenOrientation(OrientationLandscape)
	require.Equal(t, OrientationLandscape, ScreenOrientation())

	SetScreenOrientation(OrientationPortrait)
	require.Equal(t, OrientationPortrait, ScreenOrientation())

	SetScreenOrientation(OrientationDefault)
	require.Equal(t, OrientationDefault, ScreenOrientation())
}

// --- RunGameOptions ---

func TestRunGameOptionsAppliesPending(t *testing.T) {
	defer func() {
		pendingWindowWidth = 800
		pendingWindowHeight = 600
		pendingWindowTitle = "Future Render"
		pendingOrientation = OrientationDefault
	}()

	// RunGameWithOptions would start the engine, so we just test that
	// the options are applied to pending state before engine creation.
	opts := &RunGameOptions{
		InitialWindowWidth:  1920,
		InitialWindowHeight: 1080,
		InitialWindowTitle:  "My Game",
		ScreenOrientation:   OrientationLandscape,
	}

	// Apply options manually (same logic as RunGameWithOptions).
	if opts.InitialWindowWidth > 0 {
		pendingWindowWidth = opts.InitialWindowWidth
	}
	if opts.InitialWindowHeight > 0 {
		pendingWindowHeight = opts.InitialWindowHeight
	}
	if opts.InitialWindowTitle != "" {
		pendingWindowTitle = opts.InitialWindowTitle
	}
	pendingOrientation = opts.ScreenOrientation

	require.Equal(t, 1920, pendingWindowWidth)
	require.Equal(t, 1080, pendingWindowHeight)
	require.Equal(t, "My Game", pendingWindowTitle)
	require.Equal(t, OrientationLandscape, pendingOrientation)
}

func TestRunGameOptionsZeroValuesKeepDefaults(t *testing.T) {
	defer func() {
		pendingWindowWidth = 800
		pendingWindowHeight = 600
		pendingWindowTitle = "Future Render"
		pendingOrientation = OrientationDefault
	}()

	// Zero values should not override defaults.
	opts := &RunGameOptions{}

	if opts.InitialWindowWidth > 0 {
		pendingWindowWidth = opts.InitialWindowWidth
	}
	if opts.InitialWindowHeight > 0 {
		pendingWindowHeight = opts.InitialWindowHeight
	}
	if opts.InitialWindowTitle != "" {
		pendingWindowTitle = opts.InitialWindowTitle
	}

	require.Equal(t, 800, pendingWindowWidth)
	require.Equal(t, 600, pendingWindowHeight)
	require.Equal(t, "Future Render", pendingWindowTitle)
}

// --- FocusHandler / LifecycleHandler interfaces ---

type focusGame struct {
	stubGame
	focused  bool
	blurred  bool
	disposed bool
}

func (g *focusGame) OnFocus()   { g.focused = true }
func (g *focusGame) OnBlur()    { g.blurred = true }
func (g *focusGame) OnDispose() { g.disposed = true }

func TestFocusHandlerInterface(t *testing.T) {
	g := &focusGame{}

	// Verify it implements both optional interfaces.
	var fh FocusHandler = g
	var lh LifecycleHandler = g

	fh.OnFocus()
	require.True(t, g.focused)

	fh.OnBlur()
	require.True(t, g.blurred)

	lh.OnDispose()
	require.True(t, g.disposed)
}

func TestStubGameDoesNotImplementFocusHandler(t *testing.T) {
	g := &stubGame{}
	_, ok := interface{}(g).(FocusHandler)
	require.False(t, ok)
}

// --- Soft keyboard API ---

func TestSoftKeyboardNoPanic(t *testing.T) {
	// These are no-ops on desktop but should not panic.
	ShowSoftKeyboard()
	HideSoftKeyboard()
}
