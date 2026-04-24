// Copyright 2026 future-core contributors
// SPDX-License-Identifier: Apache-2.0

package {{.JavaPkg}}.{{.PrefixLower}};

import android.content.Context;
import android.hardware.input.InputManager;
import android.os.Handler;
import android.os.Looper;
import android.util.AttributeSet;
import android.util.Log;
import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;

import {{.JavaPkg}}.futurecoreview.Futurecoreview;

/**
 * FutureCoreView is the public Android View that a host Activity
 * embeds to host a future-core game. It wraps an inner
 * FutureCoreSurfaceView that owns the render surface + render thread,
 * and dispatches input events (touch, key, gamepad) into the Go engine.
 *
 * Intended usage in a host Activity:
 *
 *   protected void onCreate(Bundle savedInstanceState) {
 *       super.onCreate(savedInstanceState);
 *       FutureCoreView view = new FutureCoreView(this);
 *       setContentView(view);
 *   }
 *
 *   protected void onPause()   { super.onPause();   view.suspendGame(); }
 *   protected void onResume()  { super.onResume();  view.resumeGame();  }
 *   protected void onDestroy() { view.shutdown(); super.onDestroy(); }
 *
 * Input events are routed through package-level static methods on the
 * generated Futurecoreview class (see gomobile bind convention):
 * Futurecoreview.updateTouchesOnAndroid, onKeyDownOnAndroid, etc.
 */
public class FutureCoreView extends FrameLayout
        implements InputManager.InputDeviceListener {

    private static final String TAG = "FutureCoreView";

    private final FutureCoreSurfaceView surfaceView;
    private InputManager inputManager;
    private Handler mainHandler;

    public FutureCoreView(Context context) {
        super(context);
        surfaceView = new FutureCoreSurfaceView(context);
        // MATCH_PARENT so FrameLayout stretches the SurfaceView to our
        // full bounds, which in turn gives SurfaceView a non-zero measured
        // size. Without it (and without a superclass that forwards
        // MeasureSpecs to children), SurfaceView's BLAST surface stays
        // 0x0 at SurfaceFlinger, the ANativeWindow is unrenderable, and
        // the engine produces nothing.
        addView(surfaceView, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT));
        initInput(context);
    }

    public FutureCoreView(Context context, AttributeSet attrs) {
        super(context, attrs);
        surfaceView = new FutureCoreSurfaceView(context);
        addView(surfaceView, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT));
        initInput(context);
    }

    private void initInput(Context context) {
        // Make the group focusable so key events land here before the
        // Activity's default handler.
        setFocusable(true);
        setFocusableInTouchMode(true);
        setDescendantFocusability(FOCUS_BLOCK_DESCENDANTS);
        requestFocus();

        mainHandler = new Handler(Looper.getMainLooper());
        inputManager = (InputManager) context.getSystemService(Context.INPUT_SERVICE);
        if (inputManager != null) {
            inputManager.registerInputDeviceListener(this, mainHandler);
            // Seed the engine with any already-connected gamepads.
            int[] ids = inputManager.getInputDeviceIds();
            if (ids != null) {
                for (int id : ids) {
                    notifyDeviceAdded(id);
                }
            }
        }
    }

    /** Called from host Activity.onPause. Suspends the render loop. */
    public void suspendGame() {
        surfaceView.suspendGame();
    }

    /** Called from host Activity.onResume. Resumes the render loop. */
    public void resumeGame() {
        surfaceView.resumeGame();
    }

    /** Called from host Activity.onDestroy. Cleans up resources. */
    public void shutdown() {
        if (inputManager != null) {
            inputManager.unregisterInputDeviceListener(this);
            inputManager = null;
        }
        surfaceView.shutdown();
    }

    // --- Touch dispatch ---------------------------------------------------

    @Override
    public boolean dispatchTouchEvent(MotionEvent ev) {
        int action = ev.getActionMasked();
        int pointerCount = ev.getPointerCount();

        switch (action) {
            case MotionEvent.ACTION_DOWN:
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL: {
                int pid = ev.getPointerId(0);
                try {
                    Futurecoreview.updateTouchesOnAndroid(action, pid, ev.getX(0), ev.getY(0));
                } catch (Exception e) {
                    Log.e(TAG, "updateTouchesOnAndroid", e);
                }
                break;
            }
            case MotionEvent.ACTION_POINTER_DOWN:
            case MotionEvent.ACTION_POINTER_UP: {
                int idx = ev.getActionIndex();
                int pid = ev.getPointerId(idx);
                try {
                    Futurecoreview.updateTouchesOnAndroid(action, pid, ev.getX(idx), ev.getY(idx));
                } catch (Exception e) {
                    Log.e(TAG, "updateTouchesOnAndroid", e);
                }
                break;
            }
            case MotionEvent.ACTION_MOVE: {
                // Each pointer's current coordinates.
                for (int i = 0; i < pointerCount; i++) {
                    int pid = ev.getPointerId(i);
                    try {
                        Futurecoreview.updateTouchesOnAndroid(
                                MotionEvent.ACTION_MOVE, pid, ev.getX(i), ev.getY(i));
                    } catch (Exception e) {
                        Log.e(TAG, "updateTouchesOnAndroid", e);
                    }
                }
                break;
            }
            default:
                break;
        }

        // Consume the event — the engine handles all touch input.
        return true;
    }

    // --- Key dispatch -----------------------------------------------------

    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        try {
            Futurecoreview.onKeyDownOnAndroid(
                    keyCode,
                    event.getUnicodeChar(event.getMetaState()),
                    event.getMetaState(),
                    event.getSource(),
                    event.getDeviceId());
        } catch (Exception e) {
            Log.e(TAG, "onKeyDownOnAndroid", e);
        }
        // Let the Activity handle system keys (back, home, volume).
        // KEYCODE_BACK is forwarded to Go but not consumed, so the host
        // Activity's onBackPressed still runs.
        if (keyCode == KeyEvent.KEYCODE_BACK
                || keyCode == KeyEvent.KEYCODE_HOME
                || keyCode == KeyEvent.KEYCODE_VOLUME_UP
                || keyCode == KeyEvent.KEYCODE_VOLUME_DOWN) {
            return false;
        }
        return true;
    }

    @Override
    public boolean onKeyUp(int keyCode, KeyEvent event) {
        try {
            Futurecoreview.onKeyUpOnAndroid(
                    keyCode,
                    event.getMetaState(),
                    event.getSource(),
                    event.getDeviceId());
        } catch (Exception e) {
            Log.e(TAG, "onKeyUpOnAndroid", e);
        }
        if (keyCode == KeyEvent.KEYCODE_BACK
                || keyCode == KeyEvent.KEYCODE_HOME
                || keyCode == KeyEvent.KEYCODE_VOLUME_UP
                || keyCode == KeyEvent.KEYCODE_VOLUME_DOWN) {
            return false;
        }
        return true;
    }

    // --- Gamepad analog / trigger dispatch --------------------------------

    @Override
    public boolean onGenericMotionEvent(MotionEvent event) {
        int source = event.getSource();
        boolean isJoystick = (source & InputDevice.SOURCE_JOYSTICK) == InputDevice.SOURCE_JOYSTICK
                || (source & InputDevice.SOURCE_GAMEPAD) == InputDevice.SOURCE_GAMEPAD;
        if (!isJoystick || event.getAction() != MotionEvent.ACTION_MOVE) {
            return super.onGenericMotionEvent(event);
        }

        int deviceId = event.getDeviceId();
        // Forward the six canonical axes: sticks (X, Y, Z, RZ) + triggers
        // (LTRIGGER, RTRIGGER). HAT axes are reported via the same path
        // at indices 15/16 on the Go side.
        dispatchAxis(deviceId, MotionEvent.AXIS_X, event.getAxisValue(MotionEvent.AXIS_X));
        dispatchAxis(deviceId, MotionEvent.AXIS_Y, event.getAxisValue(MotionEvent.AXIS_Y));
        dispatchAxis(deviceId, MotionEvent.AXIS_Z, event.getAxisValue(MotionEvent.AXIS_Z));
        dispatchAxis(deviceId, MotionEvent.AXIS_RZ, event.getAxisValue(MotionEvent.AXIS_RZ));
        dispatchAxis(deviceId, MotionEvent.AXIS_LTRIGGER,
                event.getAxisValue(MotionEvent.AXIS_LTRIGGER));
        dispatchAxis(deviceId, MotionEvent.AXIS_RTRIGGER,
                event.getAxisValue(MotionEvent.AXIS_RTRIGGER));
        dispatchAxis(deviceId, MotionEvent.AXIS_HAT_X,
                event.getAxisValue(MotionEvent.AXIS_HAT_X));
        dispatchAxis(deviceId, MotionEvent.AXIS_HAT_Y,
                event.getAxisValue(MotionEvent.AXIS_HAT_Y));
        return true;
    }

    private void dispatchAxis(int deviceId, int axisId, float value) {
        try {
            Futurecoreview.onGamepadAxisChanged(deviceId, axisId, value);
        } catch (Exception e) {
            Log.e(TAG, "onGamepadAxisChanged", e);
        }
    }

    // --- InputManager.InputDeviceListener ---------------------------------

    @Override
    public void onInputDeviceAdded(int deviceId) {
        notifyDeviceAdded(deviceId);
    }

    @Override
    public void onInputDeviceRemoved(int deviceId) {
        try {
            Futurecoreview.onInputDeviceRemoved(deviceId);
        } catch (Exception e) {
            Log.e(TAG, "onInputDeviceRemoved", e);
        }
    }

    @Override
    public void onInputDeviceChanged(int deviceId) {
        // A reconfiguration. Treat as a re-add so the engine refreshes
        // any cached descriptor.
        notifyDeviceAdded(deviceId);
    }

    private void notifyDeviceAdded(int deviceId) {
        InputDevice dev = InputDevice.getDevice(deviceId);
        if (dev == null) {
            return;
        }
        int sources = dev.getSources();
        boolean isGamepad =
                (sources & InputDevice.SOURCE_GAMEPAD) == InputDevice.SOURCE_GAMEPAD
                || (sources & InputDevice.SOURCE_JOYSTICK) == InputDevice.SOURCE_JOYSTICK;
        if (!isGamepad) {
            return;
        }
        try {
            Futurecoreview.onGamepadAdded(
                    deviceId,
                    String.valueOf(dev.getName()),
                    /*axisCount*/ 0,
                    /*hatCount*/ 0,
                    String.valueOf(dev.getDescriptor()),
                    dev.getVendorId(),
                    dev.getProductId(),
                    /*buttonMask*/ 0,
                    /*axisMask*/ 0);
        } catch (Exception e) {
            Log.e(TAG, "onGamepadAdded", e);
        }
    }
}
