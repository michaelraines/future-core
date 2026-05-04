//go:build windows

package dlopen

import "syscall"

// Open loads a DLL by name and returns its module handle as a
// uintptr. Windows-native: uses syscall.LoadLibrary (LoadLibraryW
// under the hood).
//
// Windows has no RTLD_GLOBAL equivalent — symbol lookup is per-DLL —
// but the binding packages that consume this all expose their
// entry points directly from the named DLL, so the lack of GLOBAL
// semantics isn't load-bearing here.
func Open(name string) (uintptr, error) {
	h, err := syscall.LoadLibrary(name)
	if err != nil {
		return 0, err
	}
	return uintptr(h), nil
}

// Sym resolves a function pointer from a previously-Opened DLL.
// Uses syscall.GetProcAddress.
func Sym(handle uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(handle), name)
}

// Close releases a handle returned by Open via syscall.FreeLibrary.
// Most callers in this codebase keep the library loaded for the
// lifetime of the process, so this rarely fires.
func Close(handle uintptr) error {
	return syscall.FreeLibrary(syscall.Handle(handle))
}
