//go:build android

package android

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// CreateVulkanSurface creates a VkSurfaceKHR from the ANativeWindow using
// vkCreateAndroidSurfaceKHR. Implements platform.VulkanSurfaceCreator.
func (w *Window) CreateVulkanSurface(instance uintptr) (uintptr, error) {
	handle := w.NativeHandle()
	if handle == 0 {
		return 0, fmt.Errorf("android: no native window available")
	}
	fn, err := loadCreateAndroidSurfaceKHR()
	if err != nil {
		return 0, err
	}

	// VkAndroidSurfaceCreateInfoKHR
	type androidSurfaceCreateInfo struct {
		sType  uint32
		pNext  uintptr
		flags  uint32
		window uintptr // ANativeWindow*
	}
	const structureTypeAndroidSurfaceCreateInfoKHR = 1000008000

	info := androidSurfaceCreateInfo{
		sType:  structureTypeAndroidSurfaceCreateInfoKHR,
		window: handle,
	}

	var surface uintptr
	result := fn(instance, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&surface)))
	if result != 0 {
		return 0, fmt.Errorf("vkCreateAndroidSurfaceKHR: VkResult(%d)", result)
	}
	return surface, nil
}

// vkCreateAndroidSurfaceKHR function type.
type fnCreateAndroidSurface func(instance uintptr, pCreateInfo uintptr, pAllocator uintptr, pSurface uintptr) int32

var (
	createAndroidSurfaceFn   fnCreateAndroidSurface
	createAndroidSurfaceOnce sync.Once
	createAndroidSurfaceErr  error
)

func loadCreateAndroidSurfaceKHR() (fnCreateAndroidSurface, error) {
	createAndroidSurfaceOnce.Do(func() {
		// Android ships libvulkan.so as a system library.
		names := []string{
			"libvulkan.so",
			"libvulkan.so.1",
		}
		var lib uintptr
		for _, name := range names {
			h, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
			if err == nil {
				lib = h
				break
			}
		}
		if lib == 0 {
			createAndroidSurfaceErr = fmt.Errorf("android: failed to load Vulkan library")
			return
		}

		// Load vkGetInstanceProcAddr to resolve the extension function.
		var getInstanceProcAddr func(instance uintptr, pName uintptr) uintptr
		addr, err := purego.Dlsym(lib, "vkGetInstanceProcAddr")
		if err != nil {
			createAndroidSurfaceErr = fmt.Errorf("android: vkGetInstanceProcAddr: %w", err)
			return
		}
		purego.RegisterFunc(&getInstanceProcAddr, addr)

		// Lazy resolution: resolve vkCreateAndroidSurfaceKHR on first call
		// when we have a valid instance.
		createAndroidSurfaceFn = func(instance uintptr, pCreateInfo uintptr, pAllocator uintptr, pSurface uintptr) int32 {
			name := []byte("vkCreateAndroidSurfaceKHR\x00")
			fnAddr := getInstanceProcAddr(instance, uintptr(unsafe.Pointer(&name[0])))
			if fnAddr == 0 {
				return -7 // VK_ERROR_EXTENSION_NOT_PRESENT
			}
			var realFn func(instance uintptr, pCreateInfo uintptr, pAllocator uintptr, pSurface uintptr) int32
			purego.RegisterFunc(&realFn, fnAddr)
			// Replace ourselves with the resolved function for future calls.
			createAndroidSurfaceFn = realFn
			return realFn(instance, pCreateInfo, pAllocator, pSurface)
		}
	})
	return createAndroidSurfaceFn, createAndroidSurfaceErr
}
