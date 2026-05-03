//go:build js

package webgl

import (
	"syscall/js"

	"github.com/michaelraines/future-core/internal/backend"
)

// Pipeline implements backend.Pipeline for WebGL2.
//
// WebGL2 has no compiled "pipeline state object". The pipeline holds
// the shader program reference + a VAO that captures the enabled
// vertex attribute arrays, plus the vertex format used to lay out
// the per-attribute pointers. The actual `vertexAttribPointer` calls
// happen in Encoder.SetVertexBuffer — when the buffer is finally
// bound. Calling them earlier (e.g. at pipeline creation) raises
// `INVALID_OPERATION: vertexAttribPointer: no ARRAY_BUFFER is bound
// and offset is non-zero`, because the VAO captures the buffer
// currently bound to ARRAY_BUFFER at the moment of the pointer call,
// and at pipeline creation no buffer is bound yet.
type Pipeline struct {
	gl   js.Value
	desc backend.PipelineDescriptor

	// Cached shader program (owned by the Shader) and VAO. Both lazy.
	program js.Value
	vao     js.Value
	ready   bool
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// bind compiles the shader program (if needed), creates the VAO on
// first use, and binds both. Vertex attribute pointers are NOT set
// here — see Encoder.SetVertexBuffer.
func (p *Pipeline) bind() {
	if !p.ready {
		shader, ok := p.desc.Shader.(*Shader)
		if !ok || shader == nil {
			return
		}
		if !shader.compile() {
			return
		}
		p.program = shader.program
		p.vao = p.gl.Call("createVertexArray")
		p.gl.Call("bindVertexArray", p.vao)
		// Enable each attribute array up-front. Enabling is sticky in
		// the VAO state; the per-buffer vertexAttribPointer call still
		// runs every SetVertexBuffer because the engine rotates
		// through ring-buffer offsets and the pointer call is what
		// records the current ARRAY_BUFFER + offset into the VAO.
		for i := range p.desc.VertexFormat.Attributes {
			p.gl.Call("enableVertexAttribArray", i)
		}
		p.ready = true
	}

	if !p.program.IsNull() && !p.program.IsUndefined() {
		p.gl.Call("useProgram", p.program)
	}
	if !p.vao.IsNull() && !p.vao.IsUndefined() {
		p.gl.Call("bindVertexArray", p.vao)
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
