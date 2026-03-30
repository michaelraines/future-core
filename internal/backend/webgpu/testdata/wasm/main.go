//go:build js

// Program webgpu_probe tests WebGPU device initialization in a browser.
// It sets window.__webgpu_result to a JSON string with the outcome.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
	"github.com/michaelraines/future-core/internal/backend/webgpu"
)

type result struct {
	OK      bool   `json:"ok"`
	Stage   string `json:"stage"`
	Error   string `json:"error,omitempty"`
	Backend string `json:"backend"`
	Caps    string `json:"caps,omitempty"`
}

func main() {
	r := run()
	b, _ := json.Marshal(r)
	js.Global().Set("__webgpu_result", string(b))
}

func run() result {
	dev := webgpu.New()

	err := dev.Init(backend.DeviceConfig{Width: 64, Height: 64})
	if err != nil {
		return result{Stage: "init", Error: err.Error(), Backend: "webgpu"}
	}
	defer dev.Dispose()

	// Test resource creation.
	tex, err := dev.NewTexture(backend.TextureDescriptor{
		Width: 4, Height: 4,
		Format: backend.TextureFormatRGBA8,
		Data:   make([]byte, 4*4*4),
	})
	if err != nil {
		return result{Stage: "texture", Error: err.Error(), Backend: "webgpu"}
	}
	tex.Dispose()

	buf, err := dev.NewBuffer(backend.BufferDescriptor{Data: []byte{1, 2, 3, 4}})
	if err != nil {
		return result{Stage: "buffer", Error: err.Error(), Backend: "webgpu"}
	}
	buf.Dispose()

	shader, err := dev.NewShader(backend.ShaderDescriptor{})
	if err != nil {
		return result{Stage: "shader", Error: err.Error(), Backend: "webgpu"}
	}
	shader.Dispose()

	caps := dev.Capabilities()

	return result{
		OK:      true,
		Stage:   "complete",
		Backend: "webgpu",
		Caps:    capsString(caps),
	}
}

func capsString(c backend.DeviceCapabilities) string {
	b, _ := json.Marshal(c)
	return string(b)
}
