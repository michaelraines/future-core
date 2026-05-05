// Copyright 2026 future-core contributors
// SPDX-License-Identifier: Apache-2.0

package {{.JavaPkg}}.{{.PrefixLower}};

import android.app.Activity;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.pm.ActivityInfo;
import android.util.AttributeSet;
import android.util.Log;
import android.view.Choreographer;
import android.view.Surface;
import android.view.SurfaceHolder;
import android.view.SurfaceView;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.Looper;
import android.os.Message;

import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

import {{.JavaPkg}}.futurecoreview.Futurecoreview;

/**
 * FutureCoreSurfaceView hosts the future-core engine inside an Android
 * Activity. It owns:
 *
 *   1. An ANativeWindow reference (acquired via the native method
 *      nativeWindowFromSurface, registered by JNI_OnLoad on the Go side).
 *   2. A dedicated render thread (HandlerThread) — Vulkan commands
 *      must not run on the UI thread.
 *   3. A Choreographer frame-callback registered on the UI thread that
 *      posts TICK messages to the render thread at vsync cadence.
 *
 * The surface lifecycle (surfaceCreated / Changed / Destroyed) is
 * handled via SurfaceHolder.Callback. surfaceDestroyed blocks on a
 * CountDownLatch until the render thread has processed the CLEAR
 * message and released the ANativeWindow — otherwise Vulkan would
 * present against a freed surface and segfault.
 *
 * Why SurfaceView + Choreographer instead of GLSurfaceView:
 *   - GLSurfaceView's built-in render thread is GL-specific.
 *   - Vulkan requires the ANativeWindow directly; GLSurfaceView hides
 *     it behind EGL.
 *   - Choreographer gives us vsync pacing for free on any SurfaceView.
 */
class FutureCoreSurfaceView extends SurfaceView
        implements SurfaceHolder.Callback, Choreographer.FrameCallback {

    private static final String TAG = "FutureCoreSurfaceView";

    // Register our native methods on class load. Gomobile's
    // libgojni.so contains the cgo implementations; we bind them to
    // the Java method declarations below via JNI RegisterNatives.
    // Called once per process — the Go side guards with sync.Once so
    // activity recreations are safe.
    static {
        try {
            Futurecoreview.registerNativeMethods(
                FutureCoreSurfaceView.class.getName().replace('.', '/'));
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }

    // Render-thread message types.
    private static final int MSG_SET_SURFACE = 1;
    private static final int MSG_CLEAR_SURFACE = 2;
    private static final int MSG_LAYOUT = 3;
    private static final int MSG_TICK = 4;
    private static final int MSG_SUSPEND = 5;
    private static final int MSG_RESUME = 6;
    private static final int MSG_SHUTDOWN = 7;

    private HandlerThread renderThread;
    private Handler renderHandler;

    private long nativeWindowHandle = 0;       // ANativeWindow*
    private volatile boolean surfaceReady = false;
    private volatile boolean paused = false;
    // Last orientation we asked the host Activity for. Updated only
    // when the engine's RequestedOrientation changes, to avoid posting
    // a setRequestedOrientation Runnable every tick.
    private int lastAppliedOrientation = -1;

    public FutureCoreSurfaceView(Context context) {
        super(context);
        init();
    }

    public FutureCoreSurfaceView(Context context, AttributeSet attrs) {
        super(context, attrs);
        init();
    }

    private void init() {
        Log.i(TAG, "init: registering SurfaceHolder callback + starting render thread");
        getHolder().addCallback(this);

        renderThread = new HandlerThread("FutureCoreRenderThread");
        renderThread.start();
        renderHandler = new Handler(renderThread.getLooper(), new RenderCallback());
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        Log.i(TAG, "onAttachedToWindow");
    }

    // --- Public lifecycle surface (called by FutureCoreView / host) ---

    /** Pause rendering. Host should call from Activity.onPause. */
    public void suspendGame() {
        paused = true;
        Choreographer.getInstance().removeFrameCallback(this);
        renderHandler.sendEmptyMessage(MSG_SUSPEND);
    }

    /** Resume rendering. Host should call from Activity.onResume. */
    public void resumeGame() {
        paused = false;
        renderHandler.sendEmptyMessage(MSG_RESUME);
        if (surfaceReady) {
            Choreographer.getInstance().postFrameCallback(this);
        }
    }

    /** Clean shutdown. Host should call from Activity.onDestroy. */
    public void shutdown() {
        Choreographer.getInstance().removeFrameCallback(this);
        // Drain and quit the render thread.
        CountDownLatch latch = new CountDownLatch(1);
        renderHandler.post(new Runnable() {
            @Override
            public void run() {
                latch.countDown();
            }
        });
        try {
            latch.await(2, TimeUnit.SECONDS);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        renderThread.quitSafely();
    }

    // --- SurfaceHolder.Callback ---

    @Override
    public void surfaceCreated(SurfaceHolder holder) {
        Log.i(TAG, "surfaceCreated");
        Surface surface = holder.getSurface();
        // nativeWindowFromSurface is registered via JNI_OnLoad in the
        // Go side's mobile/futurecoreview package. Returns an
        // ANativeWindow* (as a long) that the engine owns until
        // surfaceDestroyed.
        long nw = nativeWindowFromSurface(surface);
        if (nw == 0) {
            Log.e(TAG, "nativeWindowFromSurface returned null");
            return;
        }
        nativeWindowHandle = nw;
        surfaceReady = true;

        Message m = renderHandler.obtainMessage(MSG_SET_SURFACE);
        m.obj = Long.valueOf(nw);
        renderHandler.sendMessage(m);

        if (!paused) {
            Choreographer.getInstance().postFrameCallback(this);
        }
    }

    @Override
    public void surfaceChanged(SurfaceHolder holder, int format, int width, int height) {
        Log.i(TAG, "surfaceChanged format=" + format + " " + width + "x" + height);
        Message m = renderHandler.obtainMessage(MSG_LAYOUT);
        m.arg1 = width;
        m.arg2 = height;
        renderHandler.sendMessage(m);
    }

    @Override
    public void surfaceDestroyed(SurfaceHolder holder) {
        surfaceReady = false;
        Choreographer.getInstance().removeFrameCallback(this);

        // Block on the render thread acknowledging that it's stopped
        // using the ANativeWindow. Without this, Vulkan's present
        // could run against a freed surface and the process would
        // segfault.
        final CountDownLatch drained = new CountDownLatch(1);
        Message m = renderHandler.obtainMessage(MSG_CLEAR_SURFACE);
        m.obj = drained;
        renderHandler.sendMessage(m);
        try {
            if (!drained.await(1, TimeUnit.SECONDS)) {
                Log.w(TAG, "render thread did not acknowledge surfaceDestroyed within 1s");
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }

        // Safe to release now — render thread has stopped using it.
        if (nativeWindowHandle != 0) {
            releaseNativeWindow(nativeWindowHandle);
            nativeWindowHandle = 0;
        }
    }

    // maybeApplyRequestedOrientation polls the engine's current
    // orientation preference (set via the scene JSON's "orientation"
    // field) and, when it changes, asks the host Activity to lock
    // accordingly. Runs from the render-thread tick handler so the
    // engine driver doesn't need a Go→Java callback path (gomobile
    // bind doesn't support those). Mapping:
    //   1 (portrait)  -> ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
    //   2 (landscape) -> ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
    //   anything else -> SCREEN_ORIENTATION_UNSPECIFIED (system default)
    private void maybeApplyRequestedOrientation() {
        // gomobile maps Go's `int` to Java `long`; cast back so we
        // can compare against the int field cheaply.
        final int requested = (int) Futurecoreview.requestedOrientation();
        if (requested == lastAppliedOrientation) {
            return;
        }
        lastAppliedOrientation = requested;
        final int orientation;
        switch (requested) {
            case 1: orientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT; break;
            case 2: orientation = ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE; break;
            default: orientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED; break;
        }
        // setRequestedOrientation must run on the UI thread.
        post(new Runnable() {
            @Override
            public void run() {
                Activity activity = findHostActivity(getContext());
                if (activity == null) {
                    Log.w(TAG, "no host Activity for setRequestedOrientation");
                    return;
                }
                activity.setRequestedOrientation(orientation);
            }
        });
    }

    private static Activity findHostActivity(Context ctx) {
        while (ctx instanceof ContextWrapper) {
            if (ctx instanceof Activity) {
                return (Activity) ctx;
            }
            ctx = ((ContextWrapper) ctx).getBaseContext();
        }
        return null;
    }

    // --- Choreographer.FrameCallback ---

    @Override
    public void doFrame(long frameTimeNanos) {
        if (!surfaceReady || paused) {
            return;
        }
        renderHandler.sendEmptyMessage(MSG_TICK);
        // Schedule next vsync. The render-thread's TICK handler
        // re-arms on completion; see RenderCallback below.
    }

    // --- Native methods (registered by Go's JNI_OnLoad) ---

    /**
     * Wraps NDK ANativeWindow_fromSurface. Returns the ANativeWindow*
     * as a long. The caller owns one reference that must be released
     * via releaseNativeWindow.
     */
    private static native long nativeWindowFromSurface(Surface surface);

    /** Wraps NDK ANativeWindow_release. */
    private static native void releaseNativeWindow(long nativeWindow);

    // --- Render-thread message handler ---

    private class RenderCallback implements Handler.Callback {
        @Override
        public boolean handleMessage(Message msg) {
            switch (msg.what) {
                case MSG_SET_SURFACE: {
                    long nw = ((Long) msg.obj).longValue();
                    try {
                        Futurecoreview.setSurface(nw);
                    } catch (Throwable e) {
                        Log.e(TAG, "setSurface threw", e);
                    }
                    return true;
                }
                case MSG_CLEAR_SURFACE: {
                    Futurecoreview.clearSurface();
                    ((CountDownLatch) msg.obj).countDown();
                    return true;
                }
                case MSG_LAYOUT: {
                    float density = getResources().getDisplayMetrics().density;
                    try {
                        Futurecoreview.layout(msg.arg1, msg.arg2, density);
                    } catch (Throwable e) {
                        Log.e(TAG, "layout threw", e);
                    }
                    return true;
                }
                case MSG_TICK: {
                    try {
                        Futurecoreview.tick();
                    } catch (Exception e) {
                        Log.e(TAG, "tick", e);
                        return true;
                    }
                    // Apply any orientation change the engine requested
                    // since the previous tick. Cheap (one int compare on
                    // the steady-state path) so it's safe per-tick.
                    maybeApplyRequestedOrientation();
                    // Re-arm vsync from the UI thread.
                    if (surfaceReady && !paused) {
                        post(new Runnable() {
                            @Override
                            public void run() {
                                Choreographer.getInstance().postFrameCallback(FutureCoreSurfaceView.this);
                            }
                        });
                    }
                    return true;
                }
                case MSG_SUSPEND: {
                    try {
                        Futurecoreview.suspend();
                    } catch (Exception e) {
                        Log.e(TAG, "suspend", e);
                    }
                    return true;
                }
                case MSG_RESUME: {
                    try {
                        Futurecoreview.resume();
                    } catch (Exception e) {
                        Log.e(TAG, "resume", e);
                    }
                    return true;
                }
                case MSG_SHUTDOWN: {
                    Looper.myLooper().quit();
                    return true;
                }
            }
            return false;
        }
    }
}
