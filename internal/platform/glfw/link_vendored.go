//go:build (linux || freebsd) && !systemglfw

// link_vendored.go provides linker flags for the vendored GLFW source build
// (the default). The vendored C source in cglfw/ is compiled directly into
// the binary; only X11 and system libraries need to be linked.
//
// When consuming this package via "go mod vendor", use -tags systemglfw
// instead (see link_system.go).
package glfw

// #cgo linux LDFLAGS: -lm -ldl -lpthread -lrt -lX11 -lXrandr -lXi -lXcursor -lXinerama
// #cgo freebsd LDFLAGS: -lm -lpthread -lX11 -lXrandr -lXi -lXcursor -lXinerama
import "C"
