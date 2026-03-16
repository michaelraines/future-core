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
}

// InnerPipeline returns nil for GPU pipelines (no soft delegation).
func (p *Pipeline) InnerPipeline() backend.Pipeline { return nil }

// Dispose is a no-op — WebGL2 pipeline state is ephemeral.
func (p *Pipeline) Dispose() {}
