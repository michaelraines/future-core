// Copyright 2026 future-core contributors
// SPDX-License-Identifier: Apache-2.0

package {{.JavaPkg}}.{{.PrefixLower}};

import android.content.Context;
import android.util.AttributeSet;
import android.view.View;
import android.view.ViewGroup;

/**
 * FutureCoreView is the public Android View that a host Activity
 * embeds to host a future-core game. It wraps an inner
 * FutureCoreSurfaceView that owns the render surface + render thread.
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
 * Phase 1 scope: surface hosting + frame pacing only. Input dispatch
 * (touch, key, gamepad) lands in Phase 2 as overrides on this class.
 */
public class FutureCoreView extends ViewGroup {

    private final FutureCoreSurfaceView surfaceView;

    public FutureCoreView(Context context) {
        super(context);
        surfaceView = new FutureCoreSurfaceView(context);
        addView(surfaceView);
    }

    public FutureCoreView(Context context, AttributeSet attrs) {
        super(context, attrs);
        surfaceView = new FutureCoreSurfaceView(context);
        addView(surfaceView);
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
        surfaceView.shutdown();
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        surfaceView.layout(0, 0, right - left, bottom - top);
        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child != surfaceView) {
                child.layout(0, 0, right - left, bottom - top);
            }
        }
    }
}
