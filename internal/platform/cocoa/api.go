//go:build darwin

// Package cocoa implements platform.Window using macOS Cocoa/AppKit via purego.
// No CGo required. All frameworks (AppKit, OpenGL, CoreGraphics) are loaded
// at runtime — they are always present on macOS.
package cocoa

import (
	"reflect"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// ---------------------------------------------------------------------------
// CoreGraphics structs (must match C layout)
// ---------------------------------------------------------------------------

// CGPoint is a point in a two-dimensional coordinate system.
type CGPoint struct{ X, Y float64 }

// CGSize is a size in a two-dimensional coordinate system.
type CGSize struct{ Width, Height float64 }

// CGRect is a rectangle in a two-dimensional coordinate system.
type CGRect struct {
	Origin CGPoint
	Size   CGSize
}

// ---------------------------------------------------------------------------
// Framework handles
// ---------------------------------------------------------------------------

var (
	appkit      uintptr
	openglFW    uintptr
	coregraphic uintptr
)

// ---------------------------------------------------------------------------
// Objective-C classes
// ---------------------------------------------------------------------------

var (
	classNSApplication       objc.Class
	classNSWindow            objc.Class
	classNSOpenGLPixelFormat objc.Class
	classNSOpenGLContext     objc.Class
	classNSScreen            objc.Class
	classNSAutoreleasePool   objc.Class
	classNSCursor            objc.Class
	classNSTrackingArea      objc.Class
)

// Custom classes registered at init time.
var (
	classFRWindowDelegate objc.Class
	classFRContentView    objc.Class
)

// ---------------------------------------------------------------------------
// Selectors (cached for performance — RegisterName grabs global lock)
// ---------------------------------------------------------------------------

var (
	selAlloc                      objc.SEL
	selInit                       objc.SEL
	selRelease                    objc.SEL
	selAutorelease                objc.SEL
	selDrain                      objc.SEL
	selSharedApplication          objc.SEL
	selSetActivationPolicy        objc.SEL
	selActivateIgnoringOtherApps  objc.SEL
	selRun                        objc.SEL
	selStop                       objc.SEL
	selFinishLaunching            objc.SEL
	selNextEventMatchingMask      objc.SEL
	selSendEvent                  objc.SEL
	selUpdateWindows              objc.SEL
	selInitWithContentRect        objc.SEL
	selMakeKeyAndOrderFront       objc.SEL
	selSetTitle                   objc.SEL
	selTitle                      objc.SEL
	selClose                      objc.SEL
	selContentView                objc.SEL
	selSetContentView             objc.SEL
	selFrame                      objc.SEL
	selSetFrame                   objc.SEL
	selContentRectForFrameRect    objc.SEL
	selFrameRectForContentRect    objc.SEL
	selStyleMask                  objc.SEL
	selSetStyleMask               objc.SEL
	selMakeFirstResponder         objc.SEL
	selSetDelegate                objc.SEL
	selSetAcceptsMouseMovedEvents objc.SEL
	selToggleFullScreen           objc.SEL
	selIsZoomed                   objc.SEL
	selMiniaturize                objc.SEL
	selWindowNumber               objc.SEL
	selConvertRectToBacking       objc.SEL
	selBackingScaleFactor         objc.SEL
	selInitWithAttributes         objc.SEL
	selInitWithFormat             objc.SEL
	selMakeCurrentContext         objc.SEL
	selFlushBuffer                objc.SEL
	selSetView                    objc.SEL
	selUpdate                     objc.SEL
	selMainScreen                 objc.SEL
	selVisibleFrame               objc.SEL
	selHide                       objc.SEL
	selUnhide                     objc.SEL
	selSetHidden                  objc.SEL
	selType                       objc.SEL
	selKeyCode                    objc.SEL
	selCharacters                 objc.SEL
	selModifierFlags              objc.SEL
	selLocationInWindow           objc.SEL
	selButtonNumber               objc.SEL
	selScrollingDeltaX            objc.SEL
	selScrollingDeltaY            objc.SEL
	selHasPreciseScrollingDeltas  objc.SEL
	selDeltaX                     objc.SEL
	selDeltaY                     objc.SEL
	selUTF8String                 objc.SEL
	selLength                     objc.SEL
	selAcceptsFirstResponder      objc.SEL
	selInitWithRect               objc.SEL
	selAddTrackingArea            objc.SEL
	selRemoveTrackingArea         objc.SEL
	selStringWithUTF8String       objc.SEL
	selSetLevel                   objc.SEL
	selSetCollectionBehavior      objc.SEL
	selContentLayoutRect          objc.SEL
)

// CGAssociateMouseAndMouseCursorPosition from CoreGraphics.
var cgAssociateMouseAndMouseCursorPosition func(connected int32) int32

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// NSApplication activation policy.
	nsApplicationActivationPolicyRegular = 0

	// NSWindow style masks.
	nsWindowStyleMaskTitled         = 1 << 0
	nsWindowStyleMaskClosable       = 1 << 1
	nsWindowStyleMaskMiniaturizable = 1 << 2
	nsWindowStyleMaskResizable      = 1 << 3
	nsWindowStyleMaskFullScreen     = 1 << 14

	// NSBackingStoreType.
	nsBackingStoreBuffered = 2

	// NSEvent types.
	nsEventTypeLeftMouseDown     = 1
	nsEventTypeLeftMouseUp       = 2
	nsEventTypeRightMouseDown    = 3
	nsEventTypeRightMouseUp      = 4
	nsEventTypeMouseMoved        = 5
	nsEventTypeLeftMouseDragged  = 6
	nsEventTypeRightMouseDragged = 7
	nsEventTypeKeyDown           = 10
	nsEventTypeKeyUp             = 11
	nsEventTypeFlagsChanged      = 12
	nsEventTypeScrollWheel       = 22
	nsEventTypeOtherMouseDown    = 25
	nsEventTypeOtherMouseUp      = 26
	nsEventTypeOtherMouseDragged = 27

	// NSEvent modifier flags.
	nsEventModifierFlagShift    = 1 << 17
	nsEventModifierFlagControl  = 1 << 18
	nsEventModifierFlagOption   = 1 << 19
	nsEventModifierFlagCommand  = 1 << 20
	nsEventModifierFlagCapsLock = 1 << 16

	// NSEventMask for nextEvent.
	nsEventMaskAny = ^uint64(0)

	// NSOpenGL pixel format attributes.
	nsOpenGLPFADoubleBuffer       = 5
	nsOpenGLPFAColorSize          = 8
	nsOpenGLPFADepthSize          = 12
	nsOpenGLPFAStencilSize        = 13
	nsOpenGLPFAOpenGLProfile      = 99
	nsOpenGLProfileVersion3_2Core = 0x3200

	// NSTrackingArea options.
	nsTrackingMouseMoved            = 0x02
	nsTrackingActiveAlways          = 0x80
	nsTrackingInVisibleRect         = 0x200
	nsTrackingMouseEnteredAndExited = 0x01

	// NSWindow levels.
	nsNormalWindowLevel = 0
)

// ---------------------------------------------------------------------------
// Init: load frameworks, cache selectors, register custom classes
// ---------------------------------------------------------------------------

func init() {
	runtime.LockOSThread()
}

var apiInitialized bool

func initAPI() error {
	if apiInitialized {
		return nil
	}

	var err error
	appkit, err = purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	openglFW, err = purego.Dlopen("/System/Library/Frameworks/OpenGL.framework/OpenGL", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	coregraphic, err = purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}

	purego.RegisterLibFunc(&cgAssociateMouseAndMouseCursorPosition, coregraphic, "CGAssociateMouseAndMouseCursorPosition")

	// Load Objective-C classes.
	classNSApplication = objc.GetClass("NSApplication")
	classNSWindow = objc.GetClass("NSWindow")
	classNSOpenGLPixelFormat = objc.GetClass("NSOpenGLPixelFormat")
	classNSOpenGLContext = objc.GetClass("NSOpenGLContext")
	classNSScreen = objc.GetClass("NSScreen")
	classNSAutoreleasePool = objc.GetClass("NSAutoreleasePool")
	classNSCursor = objc.GetClass("NSCursor")
	classNSTrackingArea = objc.GetClass("NSTrackingArea")

	// Cache selectors.
	selAlloc = objc.RegisterName("alloc")
	selInit = objc.RegisterName("init")
	selRelease = objc.RegisterName("release")
	selAutorelease = objc.RegisterName("autorelease")
	selDrain = objc.RegisterName("drain")
	selSharedApplication = objc.RegisterName("sharedApplication")
	selSetActivationPolicy = objc.RegisterName("setActivationPolicy:")
	selActivateIgnoringOtherApps = objc.RegisterName("activateIgnoringOtherApps:")
	selRun = objc.RegisterName("run")
	selStop = objc.RegisterName("stop:")
	selFinishLaunching = objc.RegisterName("finishLaunching")
	selNextEventMatchingMask = objc.RegisterName("nextEventMatchingMask:untilDate:inMode:dequeue:")
	selSendEvent = objc.RegisterName("sendEvent:")
	selUpdateWindows = objc.RegisterName("updateWindows")
	selInitWithContentRect = objc.RegisterName("initWithContentRect:styleMask:backing:defer:")
	selMakeKeyAndOrderFront = objc.RegisterName("makeKeyAndOrderFront:")
	selSetTitle = objc.RegisterName("setTitle:")
	selTitle = objc.RegisterName("title")
	selClose = objc.RegisterName("close")
	selContentView = objc.RegisterName("contentView")
	selSetContentView = objc.RegisterName("setContentView:")
	selFrame = objc.RegisterName("frame")
	selSetFrame = objc.RegisterName("setFrame:display:")
	selContentRectForFrameRect = objc.RegisterName("contentRectForFrameRect:")
	selFrameRectForContentRect = objc.RegisterName("frameRectForContentRect:styleMask:")
	selStyleMask = objc.RegisterName("styleMask")
	selSetStyleMask = objc.RegisterName("setStyleMask:")
	selMakeFirstResponder = objc.RegisterName("makeFirstResponder:")
	selSetDelegate = objc.RegisterName("setDelegate:")
	selSetAcceptsMouseMovedEvents = objc.RegisterName("setAcceptsMouseMovedEvents:")
	selToggleFullScreen = objc.RegisterName("toggleFullScreen:")
	selIsZoomed = objc.RegisterName("isZoomed")
	selMiniaturize = objc.RegisterName("miniaturize:")
	selWindowNumber = objc.RegisterName("windowNumber")
	selConvertRectToBacking = objc.RegisterName("convertRectToBacking:")
	selBackingScaleFactor = objc.RegisterName("backingScaleFactor")
	selInitWithAttributes = objc.RegisterName("initWithAttributes:")
	selInitWithFormat = objc.RegisterName("initWithFormat:shareContext:")
	selMakeCurrentContext = objc.RegisterName("makeCurrentContext")
	selFlushBuffer = objc.RegisterName("flushBuffer")
	selSetView = objc.RegisterName("setView:")
	selUpdate = objc.RegisterName("update")
	selMainScreen = objc.RegisterName("mainScreen")
	selVisibleFrame = objc.RegisterName("visibleFrame")
	selHide = objc.RegisterName("hide")
	selUnhide = objc.RegisterName("unhide")
	selSetHidden = objc.RegisterName("setHidden:")
	selType = objc.RegisterName("type")
	selKeyCode = objc.RegisterName("keyCode")
	selCharacters = objc.RegisterName("characters")
	selModifierFlags = objc.RegisterName("modifierFlags")
	selLocationInWindow = objc.RegisterName("locationInWindow")
	selButtonNumber = objc.RegisterName("buttonNumber")
	selScrollingDeltaX = objc.RegisterName("scrollingDeltaX")
	selScrollingDeltaY = objc.RegisterName("scrollingDeltaY")
	selHasPreciseScrollingDeltas = objc.RegisterName("hasPreciseScrollingDeltas")
	selDeltaX = objc.RegisterName("deltaX")
	selDeltaY = objc.RegisterName("deltaY")
	selUTF8String = objc.RegisterName("UTF8String")
	selLength = objc.RegisterName("length")
	selAcceptsFirstResponder = objc.RegisterName("acceptsFirstResponder")
	selInitWithRect = objc.RegisterName("initWithRect:options:owner:userInfo:")
	selAddTrackingArea = objc.RegisterName("addTrackingArea:")
	selRemoveTrackingArea = objc.RegisterName("removeTrackingArea:")
	selStringWithUTF8String = objc.RegisterName("stringWithUTF8String:")
	selSetLevel = objc.RegisterName("setLevel:")
	selSetCollectionBehavior = objc.RegisterName("setCollectionBehavior:")
	selContentLayoutRect = objc.RegisterName("contentLayoutRect")

	// Register custom classes.
	if err := registerCustomClasses(); err != nil {
		return err
	}

	apiInitialized = true
	return nil
}

// registerCustomClasses creates custom NSWindowDelegate and NSView subclasses.
func registerCustomClasses() error {
	var err error

	// FRWindowDelegate: handles windowShouldClose and windowDidResize.
	classFRWindowDelegate, err = objc.RegisterClass(
		"FRWindowDelegate",
		objc.GetClass("NSObject"),
		nil,
		[]objc.FieldDef{
			{Name: "goWindow", Type: reflect.TypeOf(uintptr(0)), Attribute: objc.ReadWrite},
		},
		[]objc.MethodDef{
			{
				Cmd: objc.RegisterName("windowShouldClose:"),
				Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) bool {
					w := getWindowFromDelegate(self)
					if w != nil {
						w.shouldClose = true
					}
					return false // Don't close automatically, let the game loop handle it.
				},
			},
			{
				Cmd: objc.RegisterName("windowDidResize:"),
				Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
					w := getWindowFromDelegate(self)
					if w == nil || w.handler == nil {
						return
					}
					fbW, fbH := w.FramebufferSize()
					w.handler.OnResizeEvent(fbW, fbH)
					// Update OpenGL context for new size.
					if w.glContext != 0 {
						w.glContext.Send(selUpdate)
					}
				},
			},
		},
	)
	if err != nil {
		return err
	}

	// FRContentView: handles keyboard, mouse, scroll input.
	classFRContentView, err = objc.RegisterClass(
		"FRContentView",
		objc.GetClass("NSView"),
		nil,
		[]objc.FieldDef{
			{Name: "goWindow", Type: reflect.TypeOf(uintptr(0)), Attribute: objc.ReadWrite},
		},
		[]objc.MethodDef{
			{
				Cmd: objc.RegisterName("acceptsFirstResponder"),
				Fn: func(_ objc.ID, _ objc.SEL) bool {
					return true
				},
			},
			{
				Cmd: objc.RegisterName("canBecomeKeyView"),
				Fn: func(_ objc.ID, _ objc.SEL) bool {
					return true
				},
			},
			{
				Cmd: objc.RegisterName("keyDown:"),
				Fn:  keyDownHandler,
			},
			{
				Cmd: objc.RegisterName("keyUp:"),
				Fn:  keyUpHandler,
			},
			{
				Cmd: objc.RegisterName("flagsChanged:"),
				Fn:  flagsChangedHandler,
			},
			{
				Cmd: objc.RegisterName("mouseDown:"),
				Fn:  mouseDownHandler,
			},
			{
				Cmd: objc.RegisterName("mouseUp:"),
				Fn:  mouseUpHandler,
			},
			{
				Cmd: objc.RegisterName("rightMouseDown:"),
				Fn:  rightMouseDownHandler,
			},
			{
				Cmd: objc.RegisterName("rightMouseUp:"),
				Fn:  rightMouseUpHandler,
			},
			{
				Cmd: objc.RegisterName("otherMouseDown:"),
				Fn:  otherMouseDownHandler,
			},
			{
				Cmd: objc.RegisterName("otherMouseUp:"),
				Fn:  otherMouseUpHandler,
			},
			{
				Cmd: objc.RegisterName("mouseMoved:"),
				Fn:  mouseMovedHandler,
			},
			{
				Cmd: objc.RegisterName("mouseDragged:"),
				Fn:  mouseMovedHandler,
			},
			{
				Cmd: objc.RegisterName("rightMouseDragged:"),
				Fn:  mouseMovedHandler,
			},
			{
				Cmd: objc.RegisterName("otherMouseDragged:"),
				Fn:  mouseMovedHandler,
			},
			{
				Cmd: objc.RegisterName("scrollWheel:"),
				Fn:  scrollWheelHandler,
			},
		},
	)
	return err
}

// ---------------------------------------------------------------------------
// Helper: map delegate/view ID → Go Window
// ---------------------------------------------------------------------------

// goWindowIvar caches the ivar lookup.
var goWindowIvarDelegate objc.Ivar
var goWindowIvarView objc.Ivar

func getWindowFromDelegate(id objc.ID) *Window {
	if goWindowIvarDelegate == 0 {
		goWindowIvarDelegate = classFRWindowDelegate.InstanceVariable("goWindow")
	}
	ptr := id.GetIvar(goWindowIvarDelegate)
	if ptr == 0 {
		return nil
	}
	return (*Window)(unsafe.Pointer(uintptr(ptr)))
}

func getWindowFromView(id objc.ID) *Window {
	if goWindowIvarView == 0 {
		goWindowIvarView = classFRContentView.InstanceVariable("goWindow")
	}
	ptr := id.GetIvar(goWindowIvarView)
	if ptr == 0 {
		return nil
	}
	return (*Window)(unsafe.Pointer(uintptr(ptr)))
}

// nsString creates an NSString from a Go string. Caller must release.
func nsString(s string) objc.ID {
	cstr := append([]byte(s), 0)
	return objc.ID(objc.GetClass("NSString")).Send(selAlloc).Send( //nolint:govet // ObjC interop
		objc.RegisterName("initWithUTF8String:"),
		uintptr(unsafe.Pointer(&cstr[0])),
	)
}
