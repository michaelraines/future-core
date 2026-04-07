//go:build (linux || freebsd) && systemglfw

// link_system.go provides linker flags for consumers who use "go mod vendor"
// and cannot compile the vendored GLFW source (the cglfw/ subdirectory is
// stripped by the Go module vendoring tool).
//
// Usage:
//
//	# Install system GLFW development headers
//	sudo apt-get install libglfw3-dev          # Debian/Ubuntu
//	sudo dnf install glfw-devel                # Fedora
//	sudo pacman -S glfw                        # Arch
//
//	# Build with the systemglfw tag
//	go build -tags systemglfw ./...
package glfw

// #cgo pkg-config: glfw3
// #cgo linux LDFLAGS: -lm -ldl -lpthread -lrt
// #cgo freebsd LDFLAGS: -lm -lpthread
import "C"
