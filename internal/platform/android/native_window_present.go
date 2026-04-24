//go:build android

package android

import (
	"fmt"
	"log"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ANativeWindow present path — uploads a CPU-rasterized RGBA8 frame to
// the host SurfaceView by locking the window's BufferQueue slot,
// copying pixels in, and posting. This sidesteps the Vulkan stack
// entirely, which is how we work around the Android emulator's
// gfxstream QSRI sync-fd bug (a known-broken guest Vulkan code path
// that hangs any sync primitive after the first submit). The same
// path is what GLES-based libraries like Ebitengine end up on, via
// higher layers; we talk directly to libandroid.so and skip the GL
// step since we don't need acceleration for the presenter blit.
//
// Binding surface (loaded via purego from libandroid.so):
//
//	int32_t ANativeWindow_setBuffersGeometry(ANativeWindow*, w, h, format)
//	int32_t ANativeWindow_lock(ANativeWindow*, ANativeWindow_Buffer*, ARect*)
//	int32_t ANativeWindow_unlockAndPost(ANativeWindow*)
//	void    ANativeWindow_acquire(ANativeWindow*)
//	void    ANativeWindow_release(ANativeWindow*)
//
// The struct layout of ANativeWindow_Buffer matches NDK
// <android/native_window.h>:
//
//	typedef struct ANativeWindow_Buffer {
//	    int32_t  width;   // pixels visible
//	    int32_t  height;  // pixels visible
//	    int32_t  stride;  // pixels per row in memory; may exceed width
//	    int32_t  format;  // AHARDWAREBUFFER_FORMAT_*
//	    void*    bits;    // writable pixel backing
//	    uint32_t reserved[6];
//	} ANativeWindow_Buffer;

// WINDOW_FORMAT_RGBA_8888 is the legacy alias that maps to
// AHARDWAREBUFFER_FORMAT_R8G8B8A8_UNORM. NDK headers define:
//
//	WINDOW_FORMAT_RGBA_8888 = 1
//
// which matches AHARDWAREBUFFER_FORMAT_R8G8B8A8_UNORM == 1. Using
// the legacy value keeps the binding simple and portable across NDK
// levels.
const windowFormatRGBA8888 = 1

// ANativeWindowBuffer mirrors the NDK ANativeWindow_Buffer struct.
// Passed by pointer to ANativeWindow_lock; the NDK fills in width,
// height, stride, format, and bits. Reserved must be preserved.
type ANativeWindowBuffer struct {
	Width    int32
	Height   int32
	Stride   int32 // pixels, not bytes
	Format   int32
	Bits     unsafe.Pointer
	Reserved [6]uint32
}

// aRect mirrors NDK ARect. We pass a nil *ARect to ANativeWindow_lock
// to request a full-window dirty region.
type aRect struct {
	left, top, right, bottom int32
}

var (
	nwLoadOnce sync.Once
	nwLoadErr  error

	nwSetBuffersGeometry func(window uintptr, w, h, fmt_ int32) int32
	nwLock               func(window uintptr, outBuf *ANativeWindowBuffer, inOutDirty *aRect) int32
	nwUnlockAndPost      func(window uintptr) int32
	nwAcquire            func(window uintptr)
	nwRelease            func(window uintptr)
)

// ensureANativeWindowFns dlopens libandroid.so and binds the
// ANativeWindow_* functions via purego. Idempotent: repeat calls
// after the first successful load are free.
func ensureANativeWindowFns() error {
	nwLoadOnce.Do(func() {
		h, err := purego.Dlopen("libandroid.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			nwLoadErr = fmt.Errorf("android: dlopen libandroid.so: %w", err)
			return
		}
		purego.RegisterLibFunc(&nwSetBuffersGeometry, h, "ANativeWindow_setBuffersGeometry")
		purego.RegisterLibFunc(&nwLock, h, "ANativeWindow_lock")
		purego.RegisterLibFunc(&nwUnlockAndPost, h, "ANativeWindow_unlockAndPost")
		purego.RegisterLibFunc(&nwAcquire, h, "ANativeWindow_acquire")
		purego.RegisterLibFunc(&nwRelease, h, "ANativeWindow_release")
	})
	return nwLoadErr
}

// SoftPresenter owns the ANativeWindow→pixel-blit path. One instance
// per Window. Safe to call Present from the render thread only — the
// Android SurfaceHolder contract requires surfaceCreated /
// surfaceDestroyed callbacks to bracket the native window's lifetime,
// and our Window.SetNativeWindow is driven off those callbacks on
// the UI thread, so we take w.mu inside each Present to read a
// consistent handle.
type SoftPresenter struct {
	win *Window

	mu          sync.Mutex
	geomW       int32 // width last passed to setBuffersGeometry, 0 = not yet set
	geomH       int32
	scratchLine []byte // reusable row-swap buffer for stride mismatch

	loggedBufShape bool // one-shot log of dst ANW buffer shape for diagnosis
}

// NewSoftPresenter constructs a presenter bound to the given Window.
// The Window's ANativeWindow pointer may change at any time via
// SetNativeWindow (surfaceDestroyed / surfaceCreated); Present reads
// the current pointer each call.
func NewSoftPresenter(w *Window) (*SoftPresenter, error) {
	if err := ensureANativeWindowFns(); err != nil {
		return nil, err
	}
	return &SoftPresenter{win: w}, nil
}

// Present uploads widthPx × heightPx bytes of tightly packed RGBA8
// pixel data to the ANativeWindow. Returns nil if the pixels were
// successfully queued, or an error describing which stage failed
// (geometry mismatch, lock failure, no native window bound). A
// returned error means zero pixels reached SurfaceFlinger; the
// engine can skip or retry next frame.
//
// Idempotent on a no-op-sized frame (returns nil immediately when
// pixels is empty or dimensions are zero).
func (p *SoftPresenter) Present(widthPx, heightPx int, pixels []byte) error {
	if widthPx <= 0 || heightPx <= 0 || len(pixels) == 0 {
		return nil
	}
	need := widthPx * heightPx * 4
	if len(pixels) < need {
		return fmt.Errorf("android: present buffer too small: have %d bytes, need %d", len(pixels), need)
	}

	handle := p.win.NativeHandle()
	if handle == 0 {
		return fmt.Errorf("android: no native window bound (surfaceDestroyed?)")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// setBuffersGeometry is sticky until the ANativeWindow is destroyed
	// and recreated, so we only call it on a true size change. Calling
	// it every frame is legal but adds a cross-process round-trip.
	if p.geomW != int32(widthPx) || p.geomH != int32(heightPx) {
		if rc := nwSetBuffersGeometry(handle, int32(widthPx), int32(heightPx), windowFormatRGBA8888); rc != 0 {
			return fmt.Errorf("android: ANativeWindow_setBuffersGeometry(%dx%d) = %d", widthPx, heightPx, rc)
		}
		p.geomW = int32(widthPx)
		p.geomH = int32(heightPx)
	}

	var buf ANativeWindowBuffer
	if rc := nwLock(handle, &buf, nil); rc != 0 {
		return fmt.Errorf("android: ANativeWindow_lock = %d", rc)
	}
	if !p.loggedBufShape {
		log.Printf("SoftPresenter: ANativeWindow buffer shape: w=%d h=%d stride=%d format=%d (requested %dx%d)",
			buf.Width, buf.Height, buf.Stride, buf.Format, widthPx, heightPx)
		p.loggedBufShape = true
	}

	// Direct Y-down row copy. ANativeWindow surfaces are Y-down
	// (pixel 0 = top-left). stride is in pixels, may exceed width.
	srcRowBytes := int(buf.Width) * 4
	dstStrideBytes := int(buf.Stride) * 4
	dst := unsafe.Slice((*byte)(buf.Bits), int(buf.Height)*dstStrideBytes)
	for y := 0; y < int(buf.Height); y++ {
		srcOff := y * srcRowBytes
		dstOff := y * dstStrideBytes
		copy(dst[dstOff:dstOff+srcRowBytes], pixels[srcOff:srcOff+srcRowBytes])
	}

	if rc := nwUnlockAndPost(handle); rc != 0 {
		return fmt.Errorf("android: ANativeWindow_unlockAndPost = %d", rc)
	}
	return nil
}

// InvalidateGeometry forces the next Present to re-run
// setBuffersGeometry. Called from surfaceChanged when the
// SurfaceView's dimensions change so we don't keep writing into a
// now-stale BufferQueue configuration.
func (p *SoftPresenter) InvalidateGeometry() {
	p.mu.Lock()
	p.geomW = 0
	p.geomH = 0
	p.mu.Unlock()
	_ = p.scratchLine // keep field reachable for future stride-alignment use
}
