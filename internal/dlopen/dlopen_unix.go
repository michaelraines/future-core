//go:build darwin || linux || freebsd || android

package dlopen

import (
	"github.com/ebitengine/purego"
)

// Open loads a shared library by filename and returns its handle.
// Uses purego.Dlopen with RTLD_LAZY|RTLD_GLOBAL — the GLOBAL flag is
// required for libraries that load each other transitively (e.g.
// libvulkan.so loading the per-vendor ICD).
func Open(name string) (uintptr, error) {
	return purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
}

// Sym resolves a symbol from a previously-Opened library.
func Sym(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}

// Close releases a handle returned by Open. Errors are typically
// non-actionable on Unix; callers usually ignore them.
func Close(handle uintptr) error {
	return purego.Dlclose(handle)
}
