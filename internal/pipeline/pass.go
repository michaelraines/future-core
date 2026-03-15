package pipeline

import (
	"github.com/michaelraines/future-render/internal/backend"
)

// Pass represents a single render pass in the pipeline.
// Each pass has declared inputs, outputs, and a deterministic execution function.
type Pass interface {
	// Name returns a human-readable name for debugging and profiling.
	Name() string

	// Execute runs the pass using the provided command encoder.
	Execute(enc backend.CommandEncoder, ctx *PassContext)
}

// PassContext provides shared state and resources to passes during execution.
type PassContext struct {
	// FramebufferWidth is the current framebuffer width.
	FramebufferWidth int

	// FramebufferHeight is the current framebuffer height.
	FramebufferHeight int

	// DefaultTarget is the screen render target (nil for default framebuffer).
	DefaultTarget backend.RenderTarget

	// Resources holds named resources shared between passes.
	Resources map[string]any
}

// NewPassContext creates a new PassContext.
func NewPassContext(fbWidth, fbHeight int) *PassContext {
	return &PassContext{
		FramebufferWidth:  fbWidth,
		FramebufferHeight: fbHeight,
		Resources:         make(map[string]any),
	}
}

// Pipeline manages an ordered sequence of render passes.
type Pipeline struct {
	passes []Pass
}

// New creates a new empty Pipeline.
func New() *Pipeline {
	return &Pipeline{}
}

// AddPass appends a pass to the pipeline.
func (p *Pipeline) AddPass(pass Pass) {
	p.passes = append(p.passes, pass)
}

// InsertPass inserts a pass at the given index.
// The index must be in the range [0, len(passes)].
func (p *Pipeline) InsertPass(index int, pass Pass) {
	if index < 0 || index > len(p.passes) {
		return
	}
	p.passes = append(p.passes, nil)
	copy(p.passes[index+1:], p.passes[index:])
	p.passes[index] = pass
}

// RemovePass removes the pass with the given name.
func (p *Pipeline) RemovePass(name string) {
	for i, pass := range p.passes {
		if pass.Name() == name {
			p.passes = append(p.passes[:i], p.passes[i+1:]...)
			return
		}
	}
}

// Execute runs all passes in order.
func (p *Pipeline) Execute(enc backend.CommandEncoder, ctx *PassContext) {
	for _, pass := range p.passes {
		pass.Execute(enc, ctx)
	}
}

// Passes returns the current pass list (read-only view).
func (p *Pipeline) Passes() []Pass {
	result := make([]Pass, len(p.passes))
	copy(result, p.passes)
	return result
}
