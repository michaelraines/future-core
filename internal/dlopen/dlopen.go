// Package dlopen wraps platform-specific dynamic library loading
// behind a single Open / Sym / Close API.
//
// purego ships POSIX-only Dlopen / Dlsym entry points implemented via
// libc's dlopen, so the binding packages (vk, wgpu, shaderc, d3d12)
// that compile on both Windows and Unix can't use purego directly for
// library loading. This package picks the right loader at compile
// time so callers can stay platform-agnostic.
//
// On Unix (darwin, linux, freebsd, android): purego.Dlopen with
// RTLD_LAZY|RTLD_GLOBAL — RTLD_GLOBAL matters for libraries that
// dlopen each other transitively (e.g. libvulkan.so loading the
// per-vendor ICD).
//
// On Windows: syscall.LoadLibrary (LoadLibraryW under the hood).
// Windows has no RTLD_GLOBAL equivalent — symbol lookup is per-DLL
// — but Vulkan / WebGPU / D3D12 / shaderc all expose their entry
// points directly from the named DLL, so RTLD_GLOBAL semantics
// aren't load-bearing for these consumers.
//
// The Sym function maps to dlsym / GetProcAddress respectively. The
// returned uintptr is suitable for purego.RegisterFunc and
// purego.SyscallN — both work cross-platform.
package dlopen
