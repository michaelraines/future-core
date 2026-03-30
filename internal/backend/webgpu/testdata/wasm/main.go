//go:build js

// Program webgpu_probe tests WebGPU rendering in a browser by drawing
// a vertex-colored triangle directly to a visible canvas using
// GPUCanvasContext. The result is visible in the page and captured by
// Playwright screenshot.
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"syscall/js"
)

type result struct {
	OK    bool   `json:"ok"`
	Stage string `json:"stage"`
	Error string `json:"error,omitempty"`
}

func main() {
	r := run()
	b, _ := json.Marshal(r)
	js.Global().Set("__webgpu_result", string(b))

	// Keep the Go runtime alive so the WebGPU instance isn't destroyed.
	// The browser will handle the process lifecycle.
	select {}
}

func run() result {
	// Check WebGPU availability.
	gpu := js.Global().Get("navigator").Get("gpu")
	if gpu.IsUndefined() || gpu.IsNull() {
		return fail("init", "navigator.gpu not available")
	}

	// Request adapter.
	adapter, err := await(gpu.Call("requestAdapter"))
	if err != nil {
		return fail("adapter", err.Error())
	}
	if adapter.IsNull() || adapter.IsUndefined() {
		return fail("adapter", "no suitable adapter")
	}

	// Request device.
	device, err := await(adapter.Call("requestDevice"))
	if err != nil {
		return fail("device", err.Error())
	}
	queue := device.Get("queue")

	// Create canvas and configure context.
	doc := js.Global().Get("document")
	canvas := doc.Call("createElement", "canvas")
	canvas.Set("width", 400)
	canvas.Set("height", 400)
	canvas.Set("id", "render-canvas")
	canvas.Get("style").Set("border", "1px solid #666")
	canvas.Get("style").Set("display", "block")
	canvas.Get("style").Set("margin", "10px auto")
	doc.Get("body").Call("appendChild", canvas)

	ctx := canvas.Call("getContext", "webgpu")
	if ctx.IsUndefined() || ctx.IsNull() {
		return fail("context", "canvas.getContext('webgpu') failed")
	}

	format := gpu.Call("getPreferredCanvasFormat")
	configObj := newObj()
	configObj.Set("device", device)
	configObj.Set("format", format)
	configObj.Set("alphaMode", "opaque")
	ctx.Call("configure", configObj)

	// WGSL vertex shader.
	vertWGSL := `
struct VertexInput {
    @location(0) position: vec2<f32>,
    @location(1) color: vec4<f32>,
};

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) color: vec4<f32>,
};

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    out.position = vec4<f32>(in.position, 0.0, 1.0);
    out.color = in.color;
    return out;
}
`
	// WGSL fragment shader.
	fragWGSL := `
struct FragmentInput {
    @location(0) color: vec4<f32>,
};

@fragment
fn fs_main(in: FragmentInput) -> @location(0) vec4<f32> {
    return in.color;
}
`
	// Create shader modules.
	vsDesc := newObj()
	vsDesc.Set("code", vertWGSL)
	vsMod := device.Call("createShaderModule", vsDesc)

	fsDesc := newObj()
	fsDesc.Set("code", fragWGSL)
	fsMod := device.Call("createShaderModule", fsDesc)

	// Vertex buffer layout.
	posAttr := newObj()
	posAttr.Set("format", "float32x2")
	posAttr.Set("offset", 0)
	posAttr.Set("shaderLocation", 0)

	colorAttr := newObj()
	colorAttr.Set("format", "float32x4")
	colorAttr.Set("offset", 8)
	colorAttr.Set("shaderLocation", 1)

	bufLayout := newObj()
	bufLayout.Set("arrayStride", 24) // 2 + 4 floats
	bufLayout.Set("stepMode", "vertex")
	bufLayout.Set("attributes", js.Global().Get("Array").New(posAttr, colorAttr))

	// Color target with no blending.
	colorTarget := newObj()
	colorTarget.Set("format", format)

	// Fragment state.
	fragment := newObj()
	fragment.Set("module", fsMod)
	fragment.Set("entryPoint", "fs_main")
	fragment.Set("targets", js.Global().Get("Array").New(colorTarget))

	// Pipeline descriptor.
	pipeDesc := newObj()
	pipeDesc.Set("layout", "auto")
	vertex := newObj()
	vertex.Set("module", vsMod)
	vertex.Set("entryPoint", "vs_main")
	vertex.Set("buffers", js.Global().Get("Array").New(bufLayout))
	pipeDesc.Set("vertex", vertex)
	pipeDesc.Set("fragment", fragment)
	prim := newObj()
	prim.Set("topology", "triangle-list")
	pipeDesc.Set("primitive", prim)

	pipeline := device.Call("createRenderPipeline", pipeDesc)
	if pipeline.IsNull() || pipeline.IsUndefined() {
		return fail("pipeline", "createRenderPipeline returned null")
	}

	// Vertex data: position (x,y) + color (r,g,b,a).
	vertices := encodeFloats([]float32{
		// Top — red
		0.0, 0.8, 1.0, 0.0, 0.0, 1.0,
		// Bottom-left — green
		-0.8, -0.6, 0.0, 1.0, 0.0, 1.0,
		// Bottom-right — blue
		0.8, -0.6, 0.0, 0.0, 1.0, 1.0,
	})

	vbufDesc := newObj()
	vbufDesc.Set("size", len(vertices))
	vbufDesc.Set("usage", js.Global().Get("GPUBufferUsage").Get("VERTEX").Int()|js.Global().Get("GPUBufferUsage").Get("COPY_DST").Int())
	vbuf := device.Call("createBuffer", vbufDesc)

	arr := js.Global().Get("Uint8Array").New(len(vertices))
	js.CopyBytesToJS(arr, vertices)
	queue.Call("writeBuffer", vbuf, 0, arr)

	// Get the current texture from the canvas context.
	surfaceTex := ctx.Call("getCurrentTexture")
	texView := surfaceTex.Call("createView")

	// Create command encoder and render pass.
	enc := device.Call("createCommandEncoder")

	colorAttach := newObj()
	colorAttach.Set("view", texView)
	colorAttach.Set("loadOp", "clear")
	colorAttach.Set("storeOp", "store")
	clearColor := newObj()
	clearColor.Set("r", 0.12)
	clearColor.Set("g", 0.12)
	clearColor.Set("b", 0.18)
	clearColor.Set("a", 1.0)
	colorAttach.Set("clearValue", clearColor)

	rpDesc := newObj()
	rpDesc.Set("colorAttachments", js.Global().Get("Array").New(colorAttach))

	pass := enc.Call("beginRenderPass", rpDesc)
	pass.Call("setPipeline", pipeline)
	pass.Call("setVertexBuffer", 0, vbuf)
	pass.Call("draw", 3, 1, 0, 0)
	pass.Call("end")

	cmdBuf := enc.Call("finish")
	queue.Call("submit", js.Global().Get("Array").New(cmdBuf))

	return result{OK: true, Stage: "complete"}
}

func fail(stage, err string) result {
	return result{Stage: stage, Error: err}
}

func newObj() js.Value {
	return js.Global().Get("Object").New()
}

func await(promise js.Value) (js.Value, error) {
	ch := make(chan js.Value, 1)
	errCh := make(chan error, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			ch <- args[0]
		} else {
			ch <- js.Undefined()
		}
		return nil
	})
	catch := js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		msg := "unknown"
		if len(args) > 0 {
			msg = args[0].Call("toString").String()
		}
		errCh <- fmt.Errorf("%s", msg)
		return nil
	})
	defer then.Release()
	defer catch.Release()
	promise.Call("then", then).Call("catch", catch)
	select {
	case v := <-ch:
		return v, nil
	case e := <-errCh:
		return js.Undefined(), e
	}
}

func encodeFloats(f []float32) []byte {
	buf := make([]byte, len(f)*4)
	for i, v := range f {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}
