//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-render/internal/backend"
)

// Pipeline implements backend.Pipeline for WebGL2.
// In WebGL2, pipeline state is applied imperatively via GL calls.
type Pipeline struct {
	gl   js.Value
	desc backend.PipelineDescriptor

	// Cached shader program and VAO.
	program js.Value
	vao     js.Value
	ready   bool
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// bind compiles the shader program and sets up vertex attributes.
func (p *Pipeline) bind() {
	if p.ready {
		if !p.program.IsNull() && !p.program.IsUndefined() {
			p.gl.Call("useProgram", p.program)
			if !p.vao.IsNull() && !p.vao.IsUndefined() {
				p.gl.Call("bindVertexArray", p.vao)
			}
		}
		return
	}
	p.ready = true

	shader, ok := p.desc.Shader.(*Shader)
	if !ok || shader == nil {
		return
	}

	if !shader.compile() {
		return
	}

	p.program = shader.program
	p.gl.Call("useProgram", p.program)

	// Create VAO and set up vertex attributes.
	p.vao = p.gl.Call("createVertexArray")
	p.gl.Call("bindVertexArray", p.vao)

	stride := p.desc.VertexFormat.Stride
	for i, attr := range p.desc.VertexFormat.Attributes {
		loc := i
		p.gl.Call("enableVertexAttribArray", loc)
		comps, glType, normalized := glVertexAttrib(p.gl, attr.Format)
		p.gl.Call("vertexAttribPointer", loc, comps, glType, normalized,
			stride, attr.Offset)
	}
}

// glVertexAttrib returns GL parameters for a vertex attribute format.
func glVertexAttrib(gl js.Value, f backend.AttributeFormat) (components int, glType int, normalized bool) {
	switch f {
	case backend.AttributeFloat2:
		return 2, gl.Get("FLOAT").Int(), false
	case backend.AttributeFloat3:
		return 3, gl.Get("FLOAT").Int(), false
	case backend.AttributeFloat4:
		return 4, gl.Get("FLOAT").Int(), false
	case backend.AttributeByte4Norm:
		return 4, gl.Get("UNSIGNED_BYTE").Int(), true
	default:
		return 4, gl.Get("FLOAT").Int(), false
	}
}

// Dispose releases the VAO.
func (p *Pipeline) Dispose() {
	if !p.vao.IsNull() && !p.vao.IsUndefined() {
		p.gl.Call("deleteVertexArray", p.vao)
		p.vao = js.Null()
	}
	// Program is owned by Shader; don't delete here.
}
