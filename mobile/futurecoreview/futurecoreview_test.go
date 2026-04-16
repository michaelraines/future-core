package futurecoreview

import (
	"testing"

	"github.com/stretchr/testify/require"

	futurerender "github.com/michaelraines/future-core"
)

// Phase 0 tests — compile + no-panic smoke coverage of the JNI
// trampolines. The real behavioral tests (surface handoff, frame
// pacing, input dispatch) land in Phase 1+ when the stubs become
// real engine invocations. For now we're locking in the public API
// shape so accidental signature changes surface as test failures.

type mockGame struct{}

func (mockGame) Update() error                          { return nil }
func (mockGame) Draw(*futurerender.Image)               {}
func (mockGame) Layout(w, h int) (screenW, screenH int) { return w, h }

func TestSetGameRecordsGame(t *testing.T) {
	// Isolate state between tests.
	t.Cleanup(resetState)

	g := mockGame{}
	SetGame(g)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, game, "SetGame must record the game")
}

func TestSetGameNilClears(t *testing.T) {
	t.Cleanup(resetState)

	SetGame(mockGame{})
	SetGame(nil)

	mu.Lock()
	defer mu.Unlock()
	require.Nil(t, game, "SetGame(nil) must clear the recorded game")
}

func TestSetOptionsRoundTrip(t *testing.T) {
	t.Cleanup(resetState)

	o := &futurerender.RunGameOptions{InitialWindowWidth: 1234}
	SetOptions(o)

	mu.Lock()
	defer mu.Unlock()
	require.Same(t, o, opts)
}

func TestLayoutUpdatesState(t *testing.T) {
	t.Cleanup(resetState)

	Layout(1080, 1920, 2.75)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1080, widthPx)
	require.Equal(t, 1920, heightPx)
	require.InDelta(t, 2.75, pixelsPerPt, 1e-6)
}

func TestDeviceScaleReflectsLayout(t *testing.T) {
	t.Cleanup(resetState)

	Layout(100, 100, 3.0)
	require.InDelta(t, 3.0, DeviceScale(), 1e-6)
}

func TestTickWithoutGameIsNoOp(t *testing.T) {
	t.Cleanup(resetState)
	// No SetGame call → Tick returns nil without dereferencing game.
	require.NoError(t, Tick())
}

// The remaining JNI trampolines (Suspend, Resume, OnContextLost,
// surface hooks, input dispatch) are all no-op stubs in Phase 0. We
// assert they don't panic when called on fresh state — that's the
// real invariant for "AAR loads cleanly before the host has done any
// setup".
func TestJNITrampolinesAreSafeBeforeSetup(t *testing.T) {
	t.Cleanup(resetState)

	require.NotPanics(t, func() {
		SetSurface(0)
		ClearSurface()
		require.NoError(t, Suspend())
		require.NoError(t, Resume())
		OnContextLost()
		UpdateTouchesOnAndroid(0, 0, 0, 0)
		OnKeyDownOnAndroid(0, 0, 0, 0)
		OnKeyUpOnAndroid(0, 0, 0)
		OnGamepadAxisChanged(0, 0, 0)
		OnGamepadHatChanged(0, 0, 0, 0)
		OnGamepadButton(0, 0, false)
		OnGamepadAdded(0, "", 0, 0, "", 0, 0, 0, 0)
		OnInputDeviceRemoved(0)
	})
}

// resetState clears package-level mutables between tests. Required
// because every test in this package exercises the same global
// state.
func resetState() {
	mu.Lock()
	defer mu.Unlock()
	game = nil
	opts = nil
	widthPx = 0
	heightPx = 0
	pixelsPerPt = 0
}
