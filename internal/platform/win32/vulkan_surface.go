//go:build windows

package win32

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// CreateVulkanSurface creates a VkSurfaceKHR using vkCreateWin32SurfaceKHR
// with the HWND and HINSTANCE from this window.
func (w *Window) CreateVulkanSurface(instance uintptr) (uintptr, error) {
	if w.hwnd == 0 {
		return 0, fmt.Errorf("win32: window not created")
	}
	fn, err := loadCreateWin32SurfaceKHR()
	if err != nil {
		return 0, err
	}

	// VkWin32SurfaceCreateInfoKHR
	type win32SurfaceCreateInfo struct {
		sType     uint32
		pNext     uintptr
		flags     uint32
		hinstance uintptr
		hwnd      uintptr
	}
	const structureTypeWin32SurfaceCreateInfoKHR = 1000009000

	info := win32SurfaceCreateInfo{
		sType:     structureTypeWin32SurfaceCreateInfoKHR,
		hinstance: w.hInstance,
		hwnd:      w.hwnd,
	}

	var surface uintptr
	r, _, _ := fn.Call(instance, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&surface)))
	if int32(r) != 0 {
		return 0, fmt.Errorf("vkCreateWin32SurfaceKHR: VkResult(%d)", int32(r))
	}
	return surface, nil
}

var (
	vkCreateWin32SurfaceProc *syscall.LazyProc
	vkSurfaceOnce            sync.Once
	vkSurfaceErr             error
)

func loadCreateWin32SurfaceKHR() (*syscall.LazyProc, error) {
	vkSurfaceOnce.Do(func() {
		vkDLL := syscall.NewLazyDLL("vulkan-1.dll")
		if err := vkDLL.Load(); err != nil {
			vkSurfaceErr = fmt.Errorf("win32: failed to load vulkan-1.dll: %w", err)
			return
		}
		vkCreateWin32SurfaceProc = vkDLL.NewProc("vkCreateWin32SurfaceKHR")
		if err := vkCreateWin32SurfaceProc.Find(); err != nil {
			vkSurfaceErr = fmt.Errorf("win32: vkCreateWin32SurfaceKHR: %w", err)
			vkCreateWin32SurfaceProc = nil
		}
	})
	return vkCreateWin32SurfaceProc, vkSurfaceErr
}
