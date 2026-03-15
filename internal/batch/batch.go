package batch

import (
	"math"
	"sort"

	"github.com/michaelraines/future-render/internal/backend"
)

// Vertex2D represents a single vertex in a 2D draw call.
// This is the standard vertex format for Phase 1 2D rendering.
type Vertex2D struct {
	PosX, PosY float32 // position
	TexU, TexV float32 // texture coordinates
	R, G, B, A float32 // vertex color
}

// Vertex2DSize is the byte size of a Vertex2D.
const Vertex2DSize = 32 // 8 float32s × 4 bytes

// Vertex2DFormat returns the VertexFormat for Vertex2D.
func Vertex2DFormat() backend.VertexFormat {
	return backend.VertexFormat{
		Stride: Vertex2DSize,
		Attributes: []backend.VertexAttribute{
			{Name: "position", Format: backend.AttributeFloat2, Offset: 0},
			{Name: "texcoord", Format: backend.AttributeFloat2, Offset: 8},
			{Name: "color", Format: backend.AttributeFloat4, Offset: 16},
		},
	}
}

// DrawCommand represents a single draw command before batching.
type DrawCommand struct {
	Vertices  []Vertex2D
	Indices   []uint16
	TextureID uint32 // opaque texture identifier for sorting
	BlendMode backend.BlendMode
	Filter    backend.TextureFilter // texture filter (nearest or linear)
	FillRule  backend.FillRule      // fill rule (NonZero or EvenOdd)
	ShaderID  uint32                // opaque shader identifier for sorting
	Depth     float32               // sort key for back-to-front or front-to-back ordering
	TargetID  uint32                // render target identifier (0 = screen)

	// ColorBody is the 4x4 body of the color matrix.
	// Identity ([16]float32{1,0,0,0, 0,1,0,0, 0,0,1,0, 0,0,0,1}) means no transform.
	ColorBody [16]float32

	// ColorTranslation is the 4-element translation of the color matrix.
	ColorTranslation [4]float32
}

// Batch represents a group of draw commands that share the same state.
type Batch struct {
	Vertices         []Vertex2D
	Indices          []uint16
	TextureID        uint32
	BlendMode        backend.BlendMode
	Filter           backend.TextureFilter
	FillRule         backend.FillRule
	ShaderID         uint32
	Depth            float32
	TargetID         uint32 // render target identifier (0 = screen)
	ColorBody        [16]float32
	ColorTranslation [4]float32
}

// Batcher accumulates draw commands and produces optimized batches.
type Batcher struct {
	commands    []DrawCommand
	maxVertices int
	maxIndices  int

	// Arena pools for vertex/index data. Draw commands slice into these
	// pre-allocated buffers, eliminating per-draw heap allocations.
	vertexArena []Vertex2D
	indexArena  []uint16
	vertexPos   int
	indexPos    int
}

// Default arena sizes — enough for ~256 quads before needing to grow.
const (
	defaultVertexArena = 1024 // 256 quads × 4 vertices
	defaultIndexArena  = 1536 // 256 quads × 6 indices
)

// NewBatcher creates a new Batcher with the given capacity hints.
func NewBatcher(maxVertices, maxIndices int) *Batcher {
	return &Batcher{
		commands:    make([]DrawCommand, 0, 256),
		maxVertices: maxVertices,
		maxIndices:  maxIndices,
		vertexArena: make([]Vertex2D, defaultVertexArena),
		indexArena:  make([]uint16, defaultIndexArena),
	}
}

// allocVertices returns a slice of n vertices from the arena.
// Grows the arena if needed.
func (b *Batcher) allocVertices(n int) []Vertex2D {
	if b.vertexPos+n > len(b.vertexArena) {
		newSize := len(b.vertexArena) * 2
		for newSize < b.vertexPos+n {
			newSize *= 2
		}
		arena := make([]Vertex2D, newSize)
		copy(arena, b.vertexArena[:b.vertexPos])
		b.vertexArena = arena
	}
	s := b.vertexArena[b.vertexPos : b.vertexPos+n]
	b.vertexPos += n
	return s
}

// allocIndices returns a slice of n indices from the arena.
// Grows the arena if needed.
func (b *Batcher) allocIndices(n int) []uint16 {
	if b.indexPos+n > len(b.indexArena) {
		newSize := len(b.indexArena) * 2
		for newSize < b.indexPos+n {
			newSize *= 2
		}
		arena := make([]uint16, newSize)
		copy(arena, b.indexArena[:b.indexPos])
		b.indexArena = arena
	}
	s := b.indexArena[b.indexPos : b.indexPos+n]
	b.indexPos += n
	return s
}

// Add adds a draw command to be batched. The vertex and index data
// is copied into the batcher's arena to avoid retaining caller memory.
func (b *Batcher) Add(cmd DrawCommand) {
	verts := b.allocVertices(len(cmd.Vertices))
	copy(verts, cmd.Vertices)
	idx := b.allocIndices(len(cmd.Indices))
	copy(idx, cmd.Indices)
	cmd.Vertices = verts
	cmd.Indices = idx
	b.commands = append(b.commands, cmd)
}

// AddQuadDirect adds a quad command by writing vertices directly into the
// arena, avoiding any intermediate slice allocation. This is the
// zero-allocation path for DrawImage, Fill, and DrawRectShader.
func (b *Batcher) AddQuadDirect(v0, v1, v2, v3 Vertex2D, cmd DrawCommand) {
	verts := b.allocVertices(4)
	verts[0] = v0
	verts[1] = v1
	verts[2] = v2
	verts[3] = v3

	idx := b.allocIndices(6)
	idx[0] = 0
	idx[1] = 1
	idx[2] = 2
	idx[3] = 0
	idx[4] = 2
	idx[5] = 3

	cmd.Vertices = verts
	cmd.Indices = idx
	b.commands = append(b.commands, cmd)
}

// AddQuad is a convenience method that adds a textured quad.
func (b *Batcher) AddQuad(
	x, y, w, h float32,
	u0, v0, u1, v1 float32,
	r, g, bl, a float32,
	textureID uint32,
	blendMode backend.BlendMode,
	shaderID uint32,
) {
	verts := b.allocVertices(4)
	verts[0] = Vertex2D{PosX: x, PosY: y, TexU: u0, TexV: v0, R: r, G: g, B: bl, A: a}
	verts[1] = Vertex2D{PosX: x + w, PosY: y, TexU: u1, TexV: v0, R: r, G: g, B: bl, A: a}
	verts[2] = Vertex2D{PosX: x + w, PosY: y + h, TexU: u1, TexV: v1, R: r, G: g, B: bl, A: a}
	verts[3] = Vertex2D{PosX: x, PosY: y + h, TexU: u0, TexV: v1, R: r, G: g, B: bl, A: a}

	idx := b.allocIndices(6)
	idx[0] = 0
	idx[1] = 1
	idx[2] = 2
	idx[3] = 0
	idx[4] = 2
	idx[5] = 3

	b.commands = append(b.commands, DrawCommand{
		Vertices:  verts,
		Indices:   idx,
		TextureID: textureID,
		BlendMode: blendMode,
		ShaderID:  shaderID,
	})
}

// Flush produces batches from accumulated commands and resets the batcher.
// Commands are sorted by (shader, blend mode, texture) to minimize state changes,
// then merged when adjacent commands share the same state.
func (b *Batcher) Flush() []Batch {
	if len(b.commands) == 0 {
		return nil
	}

	// Sort for optimal batching: group by state.
	// TargetID is first so all draws to the same render target are contiguous.
	sort.Slice(b.commands, func(i, j int) bool {
		ci, cj := b.commands[i], b.commands[j]
		if ci.TargetID != cj.TargetID {
			return ci.TargetID < cj.TargetID
		}
		if ci.ShaderID != cj.ShaderID {
			return ci.ShaderID < cj.ShaderID
		}
		if ci.BlendMode != cj.BlendMode {
			return ci.BlendMode < cj.BlendMode
		}
		if ci.Filter != cj.Filter {
			return ci.Filter < cj.Filter
		}
		if ci.FillRule != cj.FillRule {
			return ci.FillRule < cj.FillRule
		}
		if ci.TextureID != cj.TextureID {
			return ci.TextureID < cj.TextureID
		}
		return ci.Depth < cj.Depth
	})

	batches := make([]Batch, 0, 16)
	var current *Batch

	for i := range b.commands {
		cmd := &b.commands[i]

		// Check if we can merge with the current batch
		canMerge := current != nil &&
			current.TargetID == cmd.TargetID &&
			current.TextureID == cmd.TextureID &&
			current.BlendMode == cmd.BlendMode &&
			current.Filter == cmd.Filter &&
			current.FillRule == cmd.FillRule &&
			current.ShaderID == cmd.ShaderID &&
			current.Depth == cmd.Depth &&
			current.ColorBody == cmd.ColorBody &&
			current.ColorTranslation == cmd.ColorTranslation &&
			len(current.Vertices)+len(cmd.Vertices) <= b.maxVertices &&
			len(current.Indices)+len(cmd.Indices) <= b.maxIndices &&
			len(current.Vertices)+len(cmd.Vertices) <= math.MaxUint16

		if canMerge {
			// Merge: adjust indices and append
			vertexOffset := uint16(len(current.Vertices))
			for _, idx := range cmd.Indices {
				current.Indices = append(current.Indices, idx+vertexOffset)
			}
			current.Vertices = append(current.Vertices, cmd.Vertices...)
		} else {
			// Start a new batch
			batches = append(batches, Batch{
				Vertices:         make([]Vertex2D, len(cmd.Vertices)),
				Indices:          make([]uint16, len(cmd.Indices)),
				TextureID:        cmd.TextureID,
				BlendMode:        cmd.BlendMode,
				Filter:           cmd.Filter,
				FillRule:         cmd.FillRule,
				ShaderID:         cmd.ShaderID,
				Depth:            cmd.Depth,
				TargetID:         cmd.TargetID,
				ColorBody:        cmd.ColorBody,
				ColorTranslation: cmd.ColorTranslation,
			})
			current = &batches[len(batches)-1]
			copy(current.Vertices, cmd.Vertices)
			copy(current.Indices, cmd.Indices)
		}
	}

	// Reset for next frame
	b.commands = b.commands[:0]
	b.vertexPos = 0
	b.indexPos = 0

	return batches
}

// Reset clears accumulated commands without producing batches.
func (b *Batcher) Reset() {
	b.commands = b.commands[:0]
	b.vertexPos = 0
	b.indexPos = 0
}

// CommandCount returns the number of pending commands.
func (b *Batcher) CommandCount() int {
	return len(b.commands)
}
