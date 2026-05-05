# `cmd/futurecoremobile/` — Android AAR Builder

`futurecoremobile bind` is a thin wrapper around `gomobile bind` that
produces an Android AAR containing both the gomobile-generated JNI
trampolines **and** future-core's hand-authored Java view classes
(`FutureCoreView`, `FutureCoreSurfaceView`).

## Install

```
go install github.com/michaelraines/future-core/cmd/futurecoremobile@latest
```

## Usage

```
futurecoremobile bind \
    -target=android \
    -androidapi=21 \
    -javapkg=com.example.app \
    -tags=futurecore_mobile \
    -o out.aar \
    ./path/to/mobile/package
```

All `gomobile bind` flags pass through unchanged. `futurecoremobile`
auto-appends `github.com/michaelraines/future-core/mobile/futurecoreview`
to the bind target list — users don't need to list it.

## What it does

```
1. Forward all args to `gomobile bind` + futurecoreview pkg
       → produces a gomobile-native AAR at -o <path>
2. If the output is an .aar, call overlayAndroidJava():
    a. Extract the AAR shallowly (classes.jar left zipped)
    b. Template _files/FutureCoreView.java + FutureCoreSurfaceView.java
       with the user's -javapkg + the fixed "futurecoreview" subpkg
    c. Compile them with javac against android.jar + classes.jar
    d. Merge the compiled .class files into classes.jar IN MEMORY
    e. Rebuild the AAR from the extracted tree
```

## Why an in-memory classes.jar merge

**Do not extract `classes.jar` to the filesystem and rebuild from
there.** macOS APFS (and NTFS) are case-insensitive by default, so
`Futurecoreview.class` (the gomobile trampoline) and `FutureCoreView.class`
(our view) collide on disk — the compiled view overwrites the gomobile
trampoline and the final AAR silently loses half its methods. The
JAR format itself is case-sensitive; the corruption only happens if
the filesystem touches both filenames.

`mergeClassesJar` in `main.go` opens the existing `classes.jar` with
`archive/zip`, walks its entries in memory, and writes a new JAR
that combines them with the compiled output directory. Compiled
files are keyed by archive-relative path so name collisions within
`compiled/` still work, but the filesystem case-insensitivity that
bit us during the unzip-merge-rezip flow is no longer a factor.

## Debugging an AAR build

### List the archive

```
unzip -l out.aar
unzip -l classes.jar   # after extracting the AAR
```

Expected entries for a `com.example.app` `-javapkg`:

```
com/example/app/futurecoreview/Futurecoreview.class          ← gomobile trampoline
com/example/app/futurecoreview/FutureCoreView.class          ← our view (Phase 2)
com/example/app/futurecoreview/FutureCoreSurfaceView.class   ← SurfaceView wrapper
com/example/app/futurecoreview/FutureCoreSurfaceView$*.class ← inner classes
go/Seq*.class                                                ← gomobile runtime
```

### Disambiguate case-collided classes when inspecting

macOS `javap` and `unzip` interact with the case-insensitive FS, so
both classes extract into the same filename slot and you see
`FutureCoreView`'s methods when asking for `Futurecoreview`. Force
case-distinct output paths:

```
unzip -p classes.jar 'com/example/app/futurecoreview/Futurecoreview.class' > /tmp/trampoline/T.class
unzip -p classes.jar 'com/example/app/futurecoreview/FutureCoreView.class' > /tmp/view/V.class
javap -p /tmp/trampoline/T.class
javap -p /tmp/view/V.class
```

### Expected trampoline signatures (post-Phase 2)

```
public static native void clearSurface();
public static native double deviceScale();
public static native void layout(long, long, float);
public static native void onContextLost();
public static native void onGamepadAdded(long, String, long, long, String, long, long, long, long);
public static native void onGamepadAxisChanged(long, long, float);
public static native void onGamepadButton(long, long, boolean);
public static native void onGamepadHatChanged(long, long, long, long);
public static native void onInputDeviceRemoved(long);
public static native void onKeyDownOnAndroid(long, long, long, long, long);
public static native void onKeyUpOnAndroid(long, long, long, long);
public static native void registerNativeMethods(String) throws Exception;
public static native void resume() throws Exception;
public static native void setSurface(long);
public static native void suspend() throws Exception;
public static native void tick() throws Exception;
public static native void updateTouchesOnAndroid(long, long, float, float);
```

If you see `setSurface` missing or with a different signature, the
Go-side `futurecoreview_android.go` signature got out of sync with
`futurecoreview_stub.go` or used an unsupported type (e.g. `uintptr`,
which gomobile drops silently).

## Prerequisites

- `gomobile` on PATH (the standard `golang.org/x/mobile/cmd/gomobile`).
- Android NDK via `ANDROID_NDK_HOME` or discoverable from `ANDROID_HOME`.
- `javac` on PATH (any modern JDK). The overlay compiles the view
  classes against a located `android.jar` + the gomobile-generated
  classes.jar.
- `ANDROID_HOME` / `ANDROID_SDK_ROOT` set — `findAndroidJar()` walks
  `$ANDROID_HOME/platforms/android-*/android.jar` to pick the highest
  installed API.

## NDK API level

Always pass `-androidapi=21` (or higher). NDK r25+ dropped API 16
support; gomobile's default (16) fails preflight with
`unsupported API version 16 (not in 19..33)`.

## Editing the embedded Java sources

`_files/FutureCoreView.java` and `_files/FutureCoreSurfaceView.java`
are loaded via `//go:embed` in `main.go`. Edits to those files only
take effect after `go install ./cmd/futurecoremobile/` to rebuild the
binary on `$GOPATH/bin`. The AAR build invokes the *installed*
binary, not the source tree. Symptom of forgetting this: your Java
changes don't appear in the produced AAR's `classes.jar` even though
the diff looks right locally.

## gomobile bind type mappings (recap)

| Go | Java |
|----|------|
| `int`, `int64` | `long` (cast back at call site: `(int) Futurecoreview.foo()`) |
| `float32` | `float` |
| `string` | `String` |
| `bool` | `boolean` |
| `uintptr` | **silently dropped** — use `int64` for pointer payloads |

The `int → long` mapping bites every time you forget to cast — javac
errors with `incompatible types: possible lossy conversion from long
to int`.
