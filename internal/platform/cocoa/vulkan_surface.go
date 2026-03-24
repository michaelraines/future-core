//go:build darwin

package cocoa

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// createMetalSurface creates a VkSurfaceKHR from a CAMetalLayer using
// vkCreateMetalSurfaceEXT. The Vulkan library (MoltenVK) is loaded lazily
// on first call.
func createMetalSurface(instance uintptr, metalLayer uintptr) (uintptr, error) {
	fn, err := loadCreateMetalSurfaceEXT()
	if err != nil {
		return 0, err
	}

	// VkMetalSurfaceCreateInfoEXT
	type metalSurfaceCreateInfo struct {
		sType  uint32
		pNext  uintptr
		flags  uint32
		pLayer uintptr
	}
	const structureTypeMetalSurfaceCreateInfoEXT = 1000217000

	info := metalSurfaceCreateInfo{
		sType:  structureTypeMetalSurfaceCreateInfoEXT,
		pLayer: metalLayer,
	}

	var surface uintptr
	result := fn(instance, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&surface)))
	if result != 0 {
		return 0, fmt.Errorf("vkCreateMetalSurfaceEXT: VkResult(%d)", result)
	}
	return surface, nil
}

// vkCreateMetalSurfaceEXT function type.
type fnCreateMetalSurface func(instance uintptr, pCreateInfo uintptr, pAllocator uintptr, pSurface uintptr) int32

var (
	createMetalSurfaceFn   fnCreateMetalSurface
	createMetalSurfaceOnce sync.Once
	createMetalSurfaceErr  error
)

func loadCreateMetalSurfaceEXT() (fnCreateMetalSurface, error) {
	createMetalSurfaceOnce.Do(func() {
		// Try MoltenVK first, then libvulkan.
		names := []string{
			"/opt/homebrew/lib/libMoltenVK.dylib",
			"/usr/local/lib/libMoltenVK.dylib",
			"libMoltenVK.dylib",
			"libvulkan.1.dylib",
			"libvulkan.dylib",
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
			createMetalSurfaceErr = fmt.Errorf("cocoa: failed to load Vulkan library")
			return
		}

		// Load vkGetInstanceProcAddr to resolve the extension function.
		var getInstanceProcAddr func(instance uintptr, pName uintptr) uintptr
		addr, err := purego.Dlsym(lib, "vkGetInstanceProcAddr")
		if err != nil {
			createMetalSurfaceErr = fmt.Errorf("cocoa: vkGetInstanceProcAddr: %w", err)
			return
		}
		purego.RegisterFunc(&getInstanceProcAddr, addr)

		// We can't resolve the extension function until we have an instance,
		// so we store getInstanceProcAddr and resolve lazily in the fn wrapper.
		createMetalSurfaceFn = func(instance uintptr, pCreateInfo uintptr, pAllocator uintptr, pSurface uintptr) int32 {
			name := []byte("vkCreateMetalSurfaceEXT\x00")
			fnAddr := getInstanceProcAddr(instance, uintptr(unsafe.Pointer(&name[0])))
			if fnAddr == 0 {
				return -1 // VK_ERROR_EXTENSION_NOT_PRESENT
			}
			var realFn func(instance uintptr, pCreateInfo uintptr, pAllocator uintptr, pSurface uintptr) int32
			purego.RegisterFunc(&realFn, fnAddr)
			// Replace ourselves with the resolved function for future calls.
			createMetalSurfaceFn = realFn
			return realFn(instance, pCreateInfo, pAllocator, pSurface)
		}
	})
	return createMetalSurfaceFn, createMetalSurfaceErr
}
