# `mobile/futurecoreview/` — Android JNI Bridge

The `futurecoreview` package exposes the JNI-callable surface a host
Android Activity invokes to drive a future-core engine. It is the Go
half of the embedded-mode Android path; the Java half lives in
`cmd/futurecoremobile/_files/`.

## Layering

```
Host Activity (Kotlin/Java)
   │
   ▼
FutureCoreView (ViewGroup)  ──── touch / key / gamepad dispatch ───▶
   │                                                                 │
   ▼                                                                 ▼
FutureCoreSurfaceView (SurfaceView + Choreographer + render thread) │
   │                                                                 │
   ├─ nativeWindowFromSurface(Surface) → ANativeWindow*              │
   ├─ setSurface(long)                                               │
   ├─ layout(width, height, density)                                 │
   ├─ tick() ────────────────────────────────────────────────────────┤
   ├─ suspend() / resume()                                           │
   └─ clearSurface() + releaseNativeWindow(long)                     │
                                                                     │
   gomobile-bind trampoline class Futurecoreview.*  ◀────────────────┘
   │
   ▼
futurecoreview (this package) — package-level Go functions exported
   via `gomobile bind`; each one is a thin wrapper over
   futurerender.Android* in future-core's top-level package.
   │
   ▼
engine_android_embedded_api.go (AndroidBootstrap, AndroidSetSurface,
  AndroidTick, AndroidDispatchTouch, …) — locks embeddedMu and
  delegates to the `*engine` singleton.
   │
   ▼
engine (shared helpers in engine_android.go + embedded/nativeactivity
  build-tagged drivers).
```

## Files

| File | Build tag | Purpose |
|------|-----------|---------|
| `futurecoreview.go` | (all) | Shared package-level state: `mu sync.Mutex`, `game`, `opts`, `widthPx`, `heightPx`, `pixelsPerPt`. |
| `futurecoreview_android.go` | `android` | Real implementations. Wires SetGame / SetSurface / Tick / Suspend / Resume / input dispatch to `futurerender.Android*`. Owns `bootstrapped bool` for one-shot AndroidBootstrap. |
| `futurecoreview_stub.go` | `!android` | No-op versions with matching signatures so host tooling (macOS/Linux), gomobile's own compile step, and `go test ./…` all succeed. |
| `jni_android.go` | `android` + cgo | `RegisterNativeMethods(classPath)` — binds `nativeWindowFromSurface` / `releaseNativeWindow` to cgo functions that wrap NDK `ANativeWindow_fromSurface`/`_release`. Uses gomobile's `go_seq_push_local_frame` to obtain `JNIEnv*` without duplicating `JNI_OnLoad`. `sync.Once`-guarded. |
| `futurecoreview_test.go` | (host) | Pure-stub smoke tests: "trampolines don't panic on fresh state". |

## Public API (gomobile-exported)

All functions here are reachable from Java as `Futurecoreview.<method>`
on the trampoline class gomobile generates. gomobile's Go-to-Java
type mapping: `int → long`, `float32 → float`, `string → String`,
`bool → boolean`. `uintptr` is **not** supported — use `int64` for
pointer payloads.

### Lifecycle

- `SetGame(Game)` / `SetOptions(*RunGameOptions)` — host→engine
  config. Must precede `SetSurface`.
- `SetSurface(int64 nativeWindow)` — host→engine surface handoff.
  The first call triggers `AndroidBootstrap`.
- `ClearSurface()` — host→engine surface release. Host Java MUST
  wait for this to return before calling `ANativeWindow_release`,
  otherwise Vulkan presents against a freed surface and segfaults.
- `Layout(widthPx, heightPx int, pixelsPerPt float32)` — size + DPI
  update. Calls `AndroidEnsureDevice` best-effort; first call can
  fail if surface isn't bound yet, next Layout/SetSurface retries.
- `Tick() error` — one frame (Update + Draw + Present). Called from
  the host render thread once per Choreographer vsync.
- `Suspend()` / `Resume()` — pause/resume for `onPause` / `onResume`.
- `OnContextLost()` — drop GPU device; next `Layout` rebuilds.
- `DeviceScale() float64` — current pixels-per-point.

### Input dispatch (Phase 2)

- `UpdateTouchesOnAndroid(action, id int, x, y float32)` — MotionEvent
  sample. `action` uses Android's masked action codes
  (`ACTION_DOWN=0`, `UP=1`, `MOVE=2`, `CANCEL=3`, `POINTER_DOWN=5`,
  `POINTER_UP=6`). Multi-touch: call once per pointer.
- `OnKeyDownOnAndroid(keyCode, unicodeChar, meta, source, deviceID int)`
  / `OnKeyUpOnAndroid(keyCode, meta, source, deviceID int)` —
  KeyEvent. `source` is used to split gamepad-sourced events into
  the gamepad state machine (see `internal/platform/android/gamepad.go`).
- `OnGamepadAxisChanged(deviceID, axisID int, value float32)` —
  one MotionEvent axis. `axisID` is Android's `MotionEvent.AXIS_*`.
- `OnGamepadHatChanged(deviceID, hatID, xValue, yValue int)` — HAT
  as two axis dispatches (AXIS_HAT_X=15, AXIS_HAT_Y=16).
- `OnGamepadButton(deviceID, keyCode int, pressed bool)` — button
  event; routes through the raw-key path with `SOURCE_GAMEPAD`.
- `OnGamepadAdded(deviceID, name, axisCount, hatCount, descriptor,
  vendorID, productID, buttonMask, axisMask)` /
  `OnInputDeviceRemoved(deviceID int)` — from
  `InputManager.InputDeviceListener`.

### Native-method registration

- `RegisterNativeMethods(classPath string) error` — called once from
  `FutureCoreSurfaceView`'s static initializer. Binds the two JNI
  entry points (`nativeWindowFromSurface`, `releaseNativeWindow`) via
  `RegisterNatives`. Guarded by `sync.Once`.

## Known pitfalls

- **uintptr doesn't cross the JVM boundary.** gomobile bind drops
  `uintptr` parameters silently — the generated Java class won't
  expose the method. Always use `int64` for pointer payloads.
- **bootstrapped flag lives in `futurecoreview_android.go`, not the
  shared file.** Lint will flag it as unused on host builds
  otherwise.
- **JNIEnv* access uses gomobile's private symbol.** The cgo extern
  `go_seq_push_local_frame(jint)` is undocumented but stable (it's
  part of every gomobile-bind Android binary). Don't duplicate
  `JNI_OnLoad` — gomobile already defines it.
- **Stub file must stay in lockstep.** Any public-API signature
  change has to be mirrored in both `_android.go` and `_stub.go`.
  Host-side `go test` will catch drift; `go build` on android
  won't (it only sees the android file).

## Engine build-tag split

The `*engine` the JNI bridge drives is defined across three files
in future-core's top level:

| File | Build tag | Contains |
|------|-----------|----------|
| `engine_android.go` | `android` | `type engine`, shared helpers (`initDevice`, `initRenderResources`, `disposeRenderResources`, `setWindowSize`, …), and `TickOnce()`. |
| `engine_android_embedded.go` | `android && !futurecore_nativeactivity` | Methods `(*engine).Bootstrap / EnsureDevice / HandleSurface / ClearSurface / Suspend / Resume / OnContextLost / Dispose`. No `app.Main`. |
| `engine_android_embedded_api.go` | same | Package-level `AndroidBootstrap`, `AndroidSetSurface`, `AndroidTick`, `AndroidDispatchTouch`, etc. Owns the `embeddedEngine *engine` singleton. |
| `engine_android_nativeactivity.go` | `android && futurecore_nativeactivity` | Preserved pure-Go NativeActivity path (`app.Main` + `runAndroid`). Opt-in escape hatch for standalone-binary builds. |

To build against the NativeActivity path:
`go build -tags 'android futurecore_nativeactivity'`. Default
(unset) is embedded mode.
