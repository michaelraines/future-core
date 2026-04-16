//go:build android

package futurecoreview

/*
#include <android/native_window.h>
#include <android/native_window_jni.h>
#include <jni.h>
#include <stdint.h>
#include <stdlib.h>

// gomobile's Java seq binding provides these as extern symbols in
// the libgojni.so libraries we link against. Declared here so cgo
// resolves them at link time — see x/mobile/bind/java/seq_android.h.
extern JNIEnv* go_seq_push_local_frame(jint cap);
extern void    go_seq_pop_local_frame(JNIEnv* env);

// ---- JNI implementations of the native methods declared on
// ---- FutureCoreSurfaceView. Wired via RegisterNatives at runtime,
// ---- not by JNI name mangling, so the Java package path is
// ---- whatever the AAR consumer picked with -javapkg without
// ---- needing to re-codegen this file.

static jlong fc_native_window_from_surface_jni(JNIEnv* env, jclass clazz, jobject surface) {
    if (surface == NULL) return 0;
    ANativeWindow* win = ANativeWindow_fromSurface(env, surface);
    return (jlong)(intptr_t)win;
}

static void fc_release_native_window_jni(JNIEnv* env, jclass clazz, jlong handle) {
    if (handle == 0) return;
    ANativeWindow* win = (ANativeWindow*)(intptr_t)handle;
    ANativeWindow_release(win);
}

static const JNINativeMethod fc_native_methods[] = {
    { "nativeWindowFromSurface", "(Landroid/view/Surface;)J", (void*)fc_native_window_from_surface_jni },
    { "releaseNativeWindow",     "(J)V",                      (void*)fc_release_native_window_jni },
};

// fc_register_natives looks up the FutureCoreSurfaceView class by
// its JNI-style class path (e.g. "com/example/app/mobile/FutureCoreSurfaceView")
// and binds fc_native_methods to its declared native methods.
// Returns 0 on success, non-zero (with negative codes for our own
// failures, positive for JNI RegisterNatives failures).
static int fc_register_natives(const char* class_name) {
    JNIEnv* env = go_seq_push_local_frame(0);
    if (env == NULL) return -1;

    jclass cls = (*env)->FindClass(env, class_name);
    if (cls == NULL) {
        if ((*env)->ExceptionCheck(env)) {
            (*env)->ExceptionClear(env);
        }
        go_seq_pop_local_frame(env);
        return -2;
    }

    jint rc = (*env)->RegisterNatives(env, cls, fc_native_methods,
        (jint)(sizeof(fc_native_methods)/sizeof(fc_native_methods[0])));
    (*env)->DeleteLocalRef(env, cls);
    go_seq_pop_local_frame(env);
    return (int)rc;
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// Guards RegisterNativeMethods so the Java-side static initializer
// can invoke it idempotently across Activity recreations.
var registerOnce sync.Once
var registerErr error

// RegisterNativeMethods binds the native methods declared on
// FutureCoreSurfaceView (nativeWindowFromSurface, releaseNativeWindow)
// to their Go-side cgo implementations. Called from a static
// initializer in the generated Java FutureCoreSurfaceView class
// immediately after System.loadLibrary("gojni"):
//
//	static {
//	    System.loadLibrary("gojni");
//	    try {
//	        Futurecoreview.registerNativeMethods(
//	            FutureCoreSurfaceView.class.getName().replace('.', '/'));
//	    } catch (Exception e) {
//	        throw new RuntimeException(e);
//	    }
//	}
//
// The classPath argument must use JNI notation (slashes, not dots).
// Safe to call multiple times — only the first call has effect.
// Subsequent calls return the first call's error (if any).
func RegisterNativeMethods(classPath string) error {
	registerOnce.Do(func() {
		cName := C.CString(classPath)
		defer C.free(unsafe.Pointer(cName))
		rc := C.fc_register_natives(cName)
		if rc != 0 {
			registerErr = fmt.Errorf(
				"futurecoreview: RegisterNatives on %q failed with rc=%d",
				classPath, int(rc))
		}
	})
	return registerErr
}
