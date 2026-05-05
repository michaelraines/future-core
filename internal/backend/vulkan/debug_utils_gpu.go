//go:build (darwin || linux || freebsd || windows || android) && !soft

package vulkan

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/michaelraines/future-core/internal/vk"
)

// debugMessengerCallback is the cgo-style callback purego registers
// with vkCreateDebugUtilsMessengerEXT. The C signature is:
//
//	VKAPI_ATTR VkBool32 VKAPI_CALL fn(
//	    VkDebugUtilsMessageSeverityFlagBitsEXT      severity,
//	    VkDebugUtilsMessageTypeFlagsEXT             types,
//	    const VkDebugUtilsMessengerCallbackDataEXT* pCallbackData,
//	    void*                                       pUserData);
//
// All four args take uintptr in the Go callback so purego's
// register-passing on ARM64 doesn't truncate the smaller scalar
// types. We extract the message text via the known offset in the
// callback data struct and write it to stderr. Always returns
// VK_FALSE — returning VK_TRUE would abort the originating call,
// which is rarely what we want during debugging.
func debugMessengerCallback(severity, _ uintptr, pCallbackData, _ uintptr) uintptr {
	// First-call sentinel so we can confirm purego wired the
	// callback at all, before any validation messages fire.
	if !debugCallbackFired {
		debugCallbackFired = true
		fmt.Fprintln(os.Stderr, "[vulkan-validation] debug callback fired (first invocation)")
	}
	if pCallbackData == 0 {
		return 0
	}
	pMsgPtr := *(*uintptr)(unsafe.Pointer(pCallbackData + vk.DebugUtilsCallbackDataPMessageOffset))
	if pMsgPtr == 0 {
		return 0
	}
	msg := cstrAt(pMsgPtr)

	sev := uint32(severity)
	tag := "info"
	switch {
	case sev&vk.DebugUtilsMessageSeverityErrorEXT != 0:
		tag = "error"
	case sev&vk.DebugUtilsMessageSeverityWarningEXT != 0:
		tag = "warn"
	case sev&vk.DebugUtilsMessageSeverityInfoEXT != 0:
		tag = "info"
	case sev&vk.DebugUtilsMessageSeverityVerboseEXT != 0:
		tag = "verbose"
	}
	fmt.Fprintf(os.Stderr, "[vulkan-validation %s] %s\n", tag, msg)
	return 0
}

var debugCallbackFired bool

// cstrAt reads a NUL-terminated UTF-8 string from a raw C pointer.
// Bounded to 4096 bytes to keep a malformed pointer from running off
// the heap.
func cstrAt(p uintptr) string {
	if p == 0 {
		return ""
	}
	const limit = 4096
	var n int
	for n = 0; n < limit; n++ {
		if *(*byte)(unsafe.Pointer(p + uintptr(n))) == 0 {
			break
		}
	}
	if n == 0 {
		return ""
	}
	buf := make([]byte, n)
	copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
	return string(buf)
}

// installDebugMessenger registers a VK_EXT_debug_utils messenger that
// forwards validation messages to stderr. Returns the messenger handle
// (zero on failure). Caller must destroy via
// vk.DestroyDebugUtilsMessengerEXT in Device.Dispose.
func installDebugMessenger(inst vk.Instance) vk.DebugUtilsMessengerEXT {
	cb := purego.NewCallback(debugMessengerCallback)
	info := vk.DebugUtilsMessengerCreateInfoEXT{
		SType: vk.StructureTypeDebugUtilsMessengerCreateInfoEXT,
		// Catch everything while iterating — ERROR + WARNING is the
		// usual production filter, but the Adreno bug we're chasing
		// might surface as PERFORMANCE / INFO that's normally hidden.
		MessageSeverity: vk.DebugUtilsMessageSeverityErrorEXT |
			vk.DebugUtilsMessageSeverityWarningEXT |
			vk.DebugUtilsMessageSeverityInfoEXT,
		MessageType:     vk.DebugUtilsMessageTypeValidationEXT | vk.DebugUtilsMessageTypePerformanceEXT | vk.DebugUtilsMessageTypeGeneralEXT,
		PfnUserCallback: cb,
	}
	msg, err := vk.CreateDebugUtilsMessengerEXT(inst, &info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[vulkan] failed to create debug-utils messenger: %v\n", err)
		return 0
	}
	return msg
}
