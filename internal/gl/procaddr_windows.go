//go:build windows

package gl

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	opengl32           *windows.LazyDLL
	procWglGetProcAddr *windows.LazyProc
)

// openGLLib loads opengl32.dll and prepares wglGetProcAddress for symbol resolution.
func openGLLib() error {
	opengl32 = windows.NewLazySystemDLL("opengl32.dll")
	if err := opengl32.Load(); err != nil {
		return fmt.Errorf("failed to load opengl32.dll: %w", err)
	}
	procWglGetProcAddr = opengl32.NewProc("wglGetProcAddress")
	return nil
}

// getProcAddr resolves an OpenGL function symbol. It tries wglGetProcAddress
// first (for GL 1.2+ extension functions), then falls back to GetProcAddress
// on opengl32.dll (for GL 1.0/1.1 core functions).
func getProcAddr(name string) (uintptr, error) {
	cname, err := windows.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}

	// wglGetProcAddress returns NULL (0) or small sentinel values (1, 2, 3, -1)
	// when it cannot find the symbol.
	addr, _, _ := procWglGetProcAddr.Call(uintptr(unsafe.Pointer(cname)))
	if addr != 0 && addr != 1 && addr != 2 && addr != 3 && addr != ^uintptr(0) {
		return addr, nil
	}

	// Fall back to GetProcAddress for GL 1.0/1.1 exported functions.
	proc := opengl32.NewProc(name)
	if err := proc.Find(); err != nil {
		return 0, fmt.Errorf("symbol %s not found: %w", name, err)
	}
	return proc.Addr(), nil
}
