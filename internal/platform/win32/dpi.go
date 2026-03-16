//go:build windows

package win32

import (
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// DPI Awareness
// ---------------------------------------------------------------------------
//
// Windows DPI scaling has evolved through several APIs:
// 1. SetProcessDPIAware (Vista+) — system-wide DPI
// 2. SetProcessDpiAwareness (8.1+) — per-monitor DPI v1
// 3. SetProcessDpiAwarenessContext (10 1703+) — per-monitor DPI v2
//
// We try the newest first and fall back.

var (
	shcore = syscall.NewLazyDLL("shcore.dll")

	procSetProcessDpiAwareness    = shcore.NewProc("SetProcessDpiAwareness")
	procGetDpiForMonitor          = shcore.NewProc("GetDpiForMonitor")
	procSetProcessDPIAware        = user32.NewProc("SetProcessDPIAware")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow           = user32.NewProc("GetDpiForWindow")
)

const (
	// SetProcessDpiAwareness values (shcore.dll).
	processPerMonitorDPIAware = 2

	// DPI_AWARENESS_CONTEXT values (user32.dll, Win10 1703+).
	// These are pseudo-handles, not real pointers.
	dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2

	// MDT_EFFECTIVE_DPI for GetDpiForMonitor.
	mdtEffectiveDPI = 0

	// Default DPI.
	defaultDPI = 96
)

// dpiInitialized tracks whether we've already set DPI awareness.
var dpiInitialized bool

// initDPIAwareness sets the process DPI awareness mode.
// Must be called before any window creation.
func initDPIAwareness() {
	if dpiInitialized {
		return
	}
	dpiInitialized = true

	// Try Per-Monitor V2 (Windows 10 1703+).
	if procSetProcessDpiAwarenessCtx.Find() == nil {
		ret, _, _ := procSetProcessDpiAwarenessCtx.Call(dpiAwarenessContextPerMonitorAwareV2)
		if ret != 0 {
			return
		}
	}

	// Try Per-Monitor V1 (Windows 8.1+).
	if procSetProcessDpiAwareness.Find() == nil {
		ret, _, _ := procSetProcessDpiAwareness.Call(processPerMonitorDPIAware)
		if ret == 0 { // S_OK
			return
		}
	}

	// Fallback: System DPI Aware (Vista+).
	if procSetProcessDPIAware.Find() == nil {
		procSetProcessDPIAware.Call()
	}
}

// getDPIForWindow returns the DPI for the window.
// Falls back to monitor DPI, then system DPI (96).
func getDPIForWindow(hwnd uintptr) uint32 {
	// Try GetDpiForWindow (Win10 1607+).
	if procGetDpiForWindow.Find() == nil {
		dpi, _, _ := procGetDpiForWindow.Call(hwnd)
		if dpi != 0 {
			return uint32(dpi)
		}
	}

	// Try GetDpiForMonitor (Win8.1+).
	if procGetDpiForMonitor.Find() == nil {
		monitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
		if monitor != 0 {
			var dpiX, dpiY uint32
			ret, _, _ := procGetDpiForMonitor.Call(
				monitor,
				mdtEffectiveDPI,
				uintptr(unsafe.Pointer(&dpiX)),
				uintptr(unsafe.Pointer(&dpiY)),
			)
			if ret == 0 && dpiX != 0 { // S_OK
				return dpiX
			}
		}
	}

	return defaultDPI
}
