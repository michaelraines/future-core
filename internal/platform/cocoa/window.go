//go:build darwin

package cocoa

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/michaelraines/future-core/internal/platform"
)

// Window implements platform.Window using macOS Cocoa/AppKit via purego.
// No CGo required — all framework calls go through purego/objc.
type Window struct {
	nsApp       objc.ID
	nsWindow    objc.ID
	glContext   objc.ID
	contentView objc.ID
	delegate    objc.ID

	trackingArea objc.ID

	metalLayer objc.ID

	handler     platform.InputHandler
	shouldClose bool
	fullscreen  bool
	noGL        bool

	// Cursor state.
	cursorHidden bool
	cursorLocked bool

	// Cursor tracking for delta computation.
	prevCursorX, prevCursorY float64
	hasPrevCursor            bool
}

// cls converts an objc.Class to objc.ID for sending messages.
func cls(c objc.Class) objc.ID { return objc.ID(c) }

// New creates a new Cocoa window (uninitialized — call Create to open it).
func New() *Window {
	return &Window{}
}

// Create creates and shows a macOS window with an OpenGL context.
func (w *Window) Create(cfg platform.WindowConfig) error {
	if err := initAPI(); err != nil {
		return fmt.Errorf("cocoa api: %w", err)
	}

	// Initialize NSApplication.
	w.nsApp = cls(classNSApplication).Send(selSharedApplication)
	w.nsApp.Send(selSetActivationPolicy, nsApplicationActivationPolicyRegular)
	w.nsApp.Send(selFinishLaunching)

	// Determine style mask.
	styleMask := uintptr(nsWindowStyleMaskTitled | nsWindowStyleMaskClosable | nsWindowStyleMaskMiniaturizable)
	if cfg.Resizable {
		styleMask |= nsWindowStyleMaskResizable
	}
	if !cfg.Decorated {
		styleMask = 0
	}

	// Create the window.
	contentRect := CGRect{
		Origin: CGPoint{X: 100, Y: 100},
		Size:   CGSize{Width: float64(cfg.Width), Height: float64(cfg.Height)},
	}

	w.nsWindow = cls(classNSWindow).Send(selAlloc).Send(
		selInitWithContentRect,
		contentRect,
		styleMask,
		uintptr(nsBackingStoreBuffered),
		false, // defer
	)
	if w.nsWindow == 0 {
		return fmt.Errorf("failed to create NSWindow")
	}

	// Allow fullscreen toggle.
	w.nsWindow.Send(selSetCollectionBehavior, uintptr(1<<7)) // NSWindowCollectionBehaviorFullScreenPrimary

	// Set title.
	title := nsString(cfg.Title)
	w.nsWindow.Send(selSetTitle, title)
	title.Send(selRelease)

	// Create and set delegate (for windowShouldClose/windowDidResize).
	w.delegate = cls(classFRWindowDelegate).Send(selAlloc).Send(selInit)
	ivarDel := classFRWindowDelegate.InstanceVariable("goWindow")
	w.delegate.SetIvar(ivarDel, objc.ID(unsafe.Pointer(w)))
	w.nsWindow.Send(selSetDelegate, w.delegate)

	// Create content view (FRContentView) for input handling.
	w.contentView = cls(classFRContentView).Send(selAlloc).Send(selInit)
	ivarView := classFRContentView.InstanceVariable("goWindow")
	w.contentView.SetIvar(ivarView, objc.ID(unsafe.Pointer(w)))
	w.nsWindow.Send(selSetContentView, w.contentView)

	// Make the window accept mouse move events.
	w.nsWindow.Send(selSetAcceptsMouseMovedEvents, true)

	// Add a tracking area for mouse moved events.
	trackingOpts := uintptr(nsTrackingMouseMoved | nsTrackingActiveAlways | nsTrackingInVisibleRect | nsTrackingMouseEnteredAndExited)
	trackingRect := CGRect{Size: CGSize{Width: float64(cfg.Width), Height: float64(cfg.Height)}}
	w.trackingArea = cls(classNSTrackingArea).Send(selAlloc).Send(
		selInitWithRect,
		trackingRect,
		trackingOpts,
		w.contentView,
		uintptr(0), // userInfo: nil
	)
	w.contentView.Send(selAddTrackingArea, w.trackingArea)

	w.noGL = cfg.NoGL

	if cfg.NoGL {
		// Set up CAMetalLayer for Vulkan/Metal presentation.
		w.contentView.Send(objc.RegisterName("setWantsLayer:"), true)
		classCAMetalLayer := objc.GetClass("CAMetalLayer")
		if classCAMetalLayer != 0 {
			w.metalLayer = cls(classCAMetalLayer).Send(selAlloc).Send(selInit)
			w.contentView.Send(objc.RegisterName("setLayer:"), w.metalLayer)
		}
	} else {
		// Create OpenGL pixel format.
		attrs := [...]int32{
			nsOpenGLPFADoubleBuffer,
			nsOpenGLPFAOpenGLProfile, nsOpenGLProfileVersion3_2Core,
			nsOpenGLPFAColorSize, 24,
			nsOpenGLPFADepthSize, 24,
			nsOpenGLPFAStencilSize, 8,
			0, // terminator
		}
		pixelFormat := cls(classNSOpenGLPixelFormat).Send(selAlloc).Send(
			selInitWithAttributes,
			uintptr(unsafe.Pointer(&attrs[0])),
		)
		if pixelFormat == 0 {
			return fmt.Errorf("failed to create NSOpenGLPixelFormat")
		}

		// Create OpenGL context.
		w.glContext = cls(classNSOpenGLContext).Send(selAlloc).Send(
			selInitWithFormat,
			pixelFormat,
			uintptr(0), // shareContext: nil
		)
		pixelFormat.Send(selRelease)
		if w.glContext == 0 {
			return fmt.Errorf("failed to create NSOpenGLContext")
		}

		// Attach context to the content view.
		w.glContext.Send(selSetView, w.contentView)
		w.glContext.Send(selMakeCurrentContext)

		// VSync.
		if cfg.VSync {
			selSetValues := objc.RegisterName("setValues:forParameter:")
			swapInterval := int32(1)
			w.glContext.Send(selSetValues, uintptr(unsafe.Pointer(&swapInterval)), uintptr(222)) // NSOpenGLCPSwapInterval = 222
		}
	}

	// Make first responder (so it receives key events).
	w.nsWindow.Send(selMakeFirstResponder, w.contentView)

	// Show and activate.
	w.nsWindow.Send(selMakeKeyAndOrderFront, uintptr(0))
	w.nsApp.Send(selActivateIgnoringOtherApps, true)

	if cfg.Fullscreen {
		w.SetFullscreen(true)
	}

	return nil
}

// Destroy closes the window and releases resources.
func (w *Window) Destroy() {
	if w.glContext != 0 {
		w.glContext.Send(selRelease)
		w.glContext = 0
	}
	if w.trackingArea != 0 {
		if w.contentView != 0 {
			w.contentView.Send(selRemoveTrackingArea, w.trackingArea)
		}
		w.trackingArea.Send(selRelease)
		w.trackingArea = 0
	}
	if w.contentView != 0 {
		w.contentView.Send(selRelease)
		w.contentView = 0
	}
	if w.nsWindow != 0 {
		w.nsWindow.Send(selClose)
		w.nsWindow = 0
	}
	if w.delegate != 0 {
		w.delegate.Send(selRelease)
		w.delegate = 0
	}
}

// ShouldClose returns whether the window close has been requested.
func (w *Window) ShouldClose() bool {
	return w.shouldClose
}

// PollEvents processes pending Cocoa events via nextEventMatchingMask.
func (w *Window) PollEvents() {
	for {
		pool := cls(classNSAutoreleasePool).Send(selAlloc).Send(selInit)

		distantPast := cls(objc.GetClass("NSDate")).Send(objc.RegisterName("distantPast"))
		defaultMode := nsString("kCFRunLoopDefaultMode")

		event := w.nsApp.Send(
			selNextEventMatchingMask,
			nsEventMaskAny,
			distantPast,
			defaultMode,
			true, // dequeue
		)
		defaultMode.Send(selRelease)
		pool.Send(selDrain)

		if event == 0 {
			break
		}

		w.nsApp.Send(selSendEvent, event)
	}
	w.nsApp.Send(selUpdateWindows)

	// Update GL context if needed.
	if w.glContext != 0 {
		w.glContext.Send(selUpdate)
	}
}

// SwapBuffers swaps the OpenGL front and back buffers.
func (w *Window) SwapBuffers() {
	if w.glContext != 0 {
		w.glContext.Send(selFlushBuffer)
	}
}

// Size returns the window content area size in logical (point) units.
func (w *Window) Size() (int, int) {
	if w.contentView == 0 {
		return 0, 0
	}
	frame := objc.Send[CGRect](w.contentView, selFrame)
	return int(frame.Size.Width), int(frame.Size.Height)
}

// FramebufferSize returns the framebuffer size each backend expects.
//
// Two distinct contracts for two distinct presentation paths:
//
//   - GL path (OpenGL, Metal-via-GL-presenter, DX12-via-GL-presenter):
//     returns PHYSICAL pixels via convertRectToBacking. The
//     NSOpenGLContext attached to the FRContentView produces a
//     backbuffer at backingScaleFactor × view-bounds; rendering at
//     that resolution gives Retina-sharp output and a 1:1 blit from
//     the engine's offscreen target into the GL framebuffer.
//   - noGL path (Vulkan, WebGPU): returns LOGICAL pixels (Size()).
//     The CAMetalLayer that backs the FRContentView for these
//     backends is configured (implicitly) so that the swapchain /
//     surface matches the layer bounds in points; configuring at
//     physical pixels produced wildly-magnified interactive output
//     under MoltenVK because the rendered frame was 2× the layer's
//     logical extent and got composited as if it were the layer's
//     full-bounds content.
//
// Cross-backend parity captures will differ in dimensions on Retina
// hosts because of this asymmetry — the parity runner compares
// scaled copies, not the raw captures. Do NOT unify this function
// without also reworking the parity runner AND verifying interactive
// presentation on every backend.
func (w *Window) FramebufferSize() (int, int) {
	if w.contentView == 0 {
		return 0, 0
	}
	if w.noGL {
		return w.Size()
	}
	frame := objc.Send[CGRect](w.contentView, selFrame)
	backing := objc.Send[CGRect](w.contentView, selConvertRectToBacking, frame)
	return int(backing.Size.Width), int(backing.Size.Height)
}

// DevicePixelRatio returns the ratio of physical to logical pixels.
func (w *Window) DevicePixelRatio() float64 {
	if w.nsWindow == 0 {
		return 1.0
	}
	scale := objc.Send[float64](w.nsWindow, selBackingScaleFactor)
	if scale <= 0 {
		return 1.0
	}
	return scale
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) {
	if w.nsWindow == 0 {
		return
	}
	ns := nsString(title)
	w.nsWindow.Send(selSetTitle, ns)
	ns.Send(selRelease)
}

// SetSize sets the window size in logical pixels.
func (w *Window) SetSize(width, height int) {
	if w.nsWindow == 0 {
		return
	}
	// Get current frame to preserve position.
	frame := objc.Send[CGRect](w.nsWindow, selFrame)
	// Build new content rect and convert to frame rect.
	newContentRect := CGRect{
		Size: CGSize{Width: float64(width), Height: float64(height)},
	}
	mask := uintptr(w.nsWindow.Send(selStyleMask))
	newFrame := objc.Send[CGRect](w.nsWindow, selFrameRectForContentRect, newContentRect, mask)
	// Keep origin at same top-left position (macOS uses bottom-left origin).
	newFrame.Origin.X = frame.Origin.X
	newFrame.Origin.Y = frame.Origin.Y + frame.Size.Height - newFrame.Size.Height
	w.nsWindow.Send(selSetFrame, newFrame, true)
}

// SetFullscreen toggles fullscreen mode.
func (w *Window) SetFullscreen(fullscreen bool) {
	if w.nsWindow == 0 || fullscreen == w.fullscreen {
		return
	}
	w.fullscreen = fullscreen
	w.nsWindow.Send(selToggleFullScreen, uintptr(0))
}

// IsFullscreen returns whether the window is in fullscreen mode.
func (w *Window) IsFullscreen() bool {
	if w.nsWindow == 0 {
		return false
	}
	mask := uintptr(w.nsWindow.Send(selStyleMask))
	return mask&nsWindowStyleMaskFullScreen != 0
}

// SetCursorVisible shows or hides the cursor.
func (w *Window) SetCursorVisible(visible bool) {
	if visible == !w.cursorHidden {
		return
	}
	if visible {
		cls(classNSCursor).Send(selUnhide)
		w.cursorHidden = false
	} else {
		cls(classNSCursor).Send(selHide)
		w.cursorHidden = true
	}
}

// SetCursorLocked locks or unlocks the cursor to the window.
func (w *Window) SetCursorLocked(locked bool) {
	if locked == w.cursorLocked {
		return
	}
	w.cursorLocked = locked
	if locked {
		cgAssociateMouseAndMouseCursorPosition(0)
	} else {
		cgAssociateMouseAndMouseCursorPosition(1)
	}
}

// NativeHandle returns the NSWindow pointer as a uintptr.
func (w *Window) NativeHandle() uintptr {
	return uintptr(w.nsWindow)
}

// SetInputHandler sets the handler for input events.
func (w *Window) SetInputHandler(handler platform.InputHandler) {
	w.handler = handler
}

// PollGamepads queries the Game Controller framework for connected controllers.
func (w *Window) PollGamepads() {
	if w.handler == nil {
		return
	}
	pollGamepadsGC(w.handler)
}

// MetalLayer returns the CAMetalLayer pointer for this window.
func (w *Window) MetalLayer() uintptr {
	return uintptr(w.metalLayer)
}

// CreateVulkanSurface creates a VkSurfaceKHR using vkCreateMetalSurfaceEXT
// with the CAMetalLayer from this window's content view. The Vulkan library
// must already be loaded before calling this method.
func (w *Window) CreateVulkanSurface(instance uintptr) (uintptr, error) {
	if w.metalLayer == 0 {
		return 0, fmt.Errorf("cocoa: no CAMetalLayer (was NoGL set?)")
	}
	return createMetalSurface(instance, uintptr(w.metalLayer))
}
