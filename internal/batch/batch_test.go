package batch

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-core/internal/backend"
)

func TestBatcherMerge(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Two quads with the same state should merge into one batch
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(20, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Len(t, batches[0].Vertices, 8)
	require.Len(t, batches[0].Indices, 12)
}

func TestBatcherSplit(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Different textures should produce separate batches
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 2, backend.BlendSourceOver, 0)

	batches := b.Flush()
	require.Len(t, batches, 2)
}

func TestBatcherBlendModeSplit(t *testing.T) {
	b := NewBatcher(65535, 65535)

	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendAdditive, 0)

	batches := b.Flush()
	require.Len(t, batches, 2)
}

func TestBatcherFilterSplit(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Same texture but different filters should produce separate batches
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}, {PosX: 10, PosY: 0}, {PosX: 10, PosY: 10}, {PosX: 0, PosY: 10}},
		Indices:   []uint16{0, 1, 2, 0, 2, 3},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Filter:    backend.FilterNearest,
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 20, PosY: 0}, {PosX: 30, PosY: 0}, {PosX: 30, PosY: 10}, {PosX: 20, PosY: 10}},
		Indices:   []uint16{0, 1, 2, 0, 2, 3},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Filter:    backend.FilterLinear,
	})

	batches := b.Flush()
	require.Len(t, batches, 2)
	require.Equal(t, backend.FilterNearest, batches[0].Filter)
	require.Equal(t, backend.FilterLinear, batches[1].Filter)
}

func TestBatcherFilterMerge(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Same texture and same filter should merge
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}, {PosX: 10, PosY: 0}, {PosX: 10, PosY: 10}, {PosX: 0, PosY: 10}},
		Indices:   []uint16{0, 1, 2, 0, 2, 3},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Filter:    backend.FilterLinear,
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 20, PosY: 0}, {PosX: 30, PosY: 0}, {PosX: 30, PosY: 10}, {PosX: 20, PosY: 10}},
		Indices:   []uint16{0, 1, 2, 0, 2, 3},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Filter:    backend.FilterLinear,
	})

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FilterLinear, batches[0].Filter)
	require.Len(t, batches[0].Vertices, 8)
}

func TestBatcherFillRuleSplit(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Same texture but different fill rules should produce separate batches
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}, {PosX: 10, PosY: 0}, {PosX: 10, PosY: 10}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleNonZero,
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 20, PosY: 0}, {PosX: 30, PosY: 0}, {PosX: 30, PosY: 10}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleEvenOdd,
	})

	batches := b.Flush()
	require.Len(t, batches, 2)
	require.Equal(t, backend.FillRuleNonZero, batches[0].FillRule)
	require.Equal(t, backend.FillRuleEvenOdd, batches[1].FillRule)
}

func TestBatcherFillRuleMerge(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Same fill rule should merge
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}, {PosX: 10, PosY: 0}, {PosX: 10, PosY: 10}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleEvenOdd,
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 20, PosY: 0}, {PosX: 30, PosY: 0}, {PosX: 30, PosY: 10}},
		Indices:   []uint16{0, 1, 2},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		FillRule:  backend.FillRuleEvenOdd,
	})

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Equal(t, backend.FillRuleEvenOdd, batches[0].FillRule)
	require.Len(t, batches[0].Vertices, 6)
}

func TestBatcherDepthSplit(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Two commands with identical state except different Depth values
	// should produce separate batches (Depth prevents merging).
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}, {PosX: 10, PosY: 0}, {PosX: 10, PosY: 10}, {PosX: 0, PosY: 10}},
		Indices:   []uint16{0, 1, 2, 0, 2, 3},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Depth:     0.0,
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 20, PosY: 0}, {PosX: 30, PosY: 0}, {PosX: 30, PosY: 10}, {PosX: 20, PosY: 10}},
		Indices:   []uint16{0, 1, 2, 0, 2, 3},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		Depth:     1.0,
	})

	batches := b.Flush()
	require.Len(t, batches, 2)
	require.InDelta(t, 0.0, float64(batches[0].Depth), 1e-9)
	require.InDelta(t, 1.0, float64(batches[1].Depth), 1e-9)
}

func TestBatcherReset(t *testing.T) {
	b := NewBatcher(65535, 65535)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.Reset()
	require.Equal(t, 0, b.CommandCount())
}

func TestBatcherIndexOffset(t *testing.T) {
	b := NewBatcher(65535, 65535)

	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(20, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)

	batches := b.Flush()
	indices := batches[0].Indices
	// Second quad indices should be offset by 4 (first quad's vertex count)
	require.Equal(t, uint16(4), indices[6])
	require.Equal(t, uint16(5), indices[7])
	require.Equal(t, uint16(6), indices[8])
}

func TestVertex2DFormat(t *testing.T) {
	f := Vertex2DFormat()
	require.Equal(t, Vertex2DSize, f.Stride)
	require.Len(t, f.Attributes, 3)

	require.Equal(t, "position", f.Attributes[0].Name)
	require.Equal(t, backend.AttributeFloat2, f.Attributes[0].Format)
	require.Equal(t, 0, f.Attributes[0].Offset)

	require.Equal(t, "texcoord", f.Attributes[1].Name)
	require.Equal(t, backend.AttributeFloat2, f.Attributes[1].Format)
	require.Equal(t, 8, f.Attributes[1].Offset)

	require.Equal(t, "color", f.Attributes[2].Name)
	require.Equal(t, backend.AttributeFloat4, f.Attributes[2].Format)
	require.Equal(t, 16, f.Attributes[2].Offset)
}

func TestBatcherFlushEmpty(t *testing.T) {
	b := NewBatcher(65535, 65535)
	batches := b.Flush()
	require.Equal(t, []Batch(nil), batches)
}

// TestBatcherPreservesInsertionOrderWithinTarget verifies that the batcher
// does NOT reorder commands within a target by state (shader/blend/texture).
// Reordering would break Painter's-algorithm semantics for any feature that
// draws to a target with multiple textures where draw order matters — for
// example, bigOffscreenBuffer anti-alias composites interleaved with glyph
// draws.
//
// State-based batching is recovered via the adjacent-merge pass, but only
// merges CONSECUTIVE commands that happen to share state at enqueue time.
func TestBatcherPreservesInsertionOrderWithinTarget(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Interleave two textures in A,B,A,B order. All commands target 0
	// (screen) so TargetID doesn't affect ordering.
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 2, backend.BlendSourceOver, 0)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 2, backend.BlendSourceOver, 0)

	batches := b.Flush()

	// Expected: four separate batches in insertion order (tex 1, 2, 1, 2).
	// A state-sorting batcher would produce two batches (1,1 then 2,2).
	require.Len(t, batches, 4, "insertion order must be preserved; state-based reordering is forbidden")
	require.Equal(t, uint32(1), batches[0].TextureID)
	require.Equal(t, uint32(2), batches[1].TextureID)
	require.Equal(t, uint32(1), batches[2].TextureID)
	require.Equal(t, uint32(2), batches[3].TextureID)
}

// TestBatcherPreservesInsertionOrderAcrossTargets verifies that commands
// targeting different targets interleave in insertion order at the batch
// level. There is NO global TargetID sort. Callers are responsible for
// queuing commands in dependency order — if a command reads from a target
// that was written to earlier, the writes must have been queued first.
//
// This matches Ebitengine's command queue: strict insertion order with no
// reordering. Allocation order can't express dependency order (a parent may
// be allocated before its children or vice versa), so any numeric sort by
// TargetID would break one pattern or the other.
func TestBatcherPreservesInsertionOrderAcrossTargets(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Alternate screen and offscreen targets in insertion order:
	// screen, offscreen, screen, offscreen. Expect 4 separate batches
	// in that same order.
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}},
		Indices:   []uint16{0},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		TargetID:  0, // screen
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}},
		Indices:   []uint16{0},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		TargetID:  5, // offscreen
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}},
		Indices:   []uint16{0},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		TargetID:  0, // screen
	})
	b.Add(DrawCommand{
		Vertices:  []Vertex2D{{PosX: 0, PosY: 0}},
		Indices:   []uint16{0},
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
		TargetID:  5, // offscreen
	})

	batches := b.Flush()

	// Insertion order: 0, 5, 0, 5 — four batches (target switches break
	// merging) in insertion order.
	require.Len(t, batches, 4)
	require.Equal(t, uint32(0), batches[0].TargetID)
	require.Equal(t, uint32(5), batches[1].TargetID)
	require.Equal(t, uint32(0), batches[2].TargetID)
	require.Equal(t, uint32(5), batches[3].TargetID)
}

// TestBatcherAdjacentMergeWithinTarget verifies that the adjacent-merge
// pass still collapses consecutive commands that share state, even though
// the sort no longer groups by state. This is the "recovered" batching
// benefit — runs of state-coherent draws (the common case) still merge.
func TestBatcherAdjacentMergeWithinTarget(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Three consecutive draws with identical state: should merge into 1.
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(10, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(20, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)

	// One draw with different texture: new batch.
	b.AddQuad(30, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 2, backend.BlendSourceOver, 0)

	// Two more consecutive draws with the original state: since they are
	// no longer adjacent to the first run in the command list, they form
	// their own merged batch of 2.
	b.AddQuad(40, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(50, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)

	batches := b.Flush()

	require.Len(t, batches, 3, "consecutive matching state should merge; non-adjacent should not")
	require.Equal(t, uint32(1), batches[0].TextureID)
	require.Len(t, batches[0].Vertices, 12, "first three quads (12 vertices) should merge")
	require.Equal(t, uint32(2), batches[1].TextureID)
	require.Len(t, batches[1].Vertices, 4)
	require.Equal(t, uint32(1), batches[2].TextureID)
	require.Len(t, batches[2].Vertices, 8, "last two quads (8 vertices) should merge")
}

func TestAddQuadDirect(t *testing.T) {
	b := NewBatcher(65535, 65535)

	v0 := Vertex2D{PosX: 0, PosY: 0, TexU: 0, TexV: 0, R: 1, G: 1, B: 1, A: 1}
	v1 := Vertex2D{PosX: 10, PosY: 0, TexU: 1, TexV: 0, R: 1, G: 1, B: 1, A: 1}
	v2 := Vertex2D{PosX: 10, PosY: 10, TexU: 1, TexV: 1, R: 1, G: 1, B: 1, A: 1}
	v3 := Vertex2D{PosX: 0, PosY: 10, TexU: 0, TexV: 1, R: 1, G: 1, B: 1, A: 1}

	b.AddQuadDirect(v0, v1, v2, v3, DrawCommand{
		TextureID: 1,
		BlendMode: backend.BlendSourceOver,
	})

	require.Equal(t, 1, b.CommandCount())

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Len(t, batches[0].Vertices, 4)
	require.Len(t, batches[0].Indices, 6)
	require.Equal(t, float32(0), batches[0].Vertices[0].PosX)
	require.Equal(t, float32(10), batches[0].Vertices[1].PosX)
}

func TestAddQuadDirectMerge(t *testing.T) {
	b := NewBatcher(65535, 65535)

	v0 := Vertex2D{PosX: 0, PosY: 0, R: 1, G: 1, B: 1, A: 1}
	v1 := Vertex2D{PosX: 10, PosY: 0, R: 1, G: 1, B: 1, A: 1}
	v2 := Vertex2D{PosX: 10, PosY: 10, R: 1, G: 1, B: 1, A: 1}
	v3 := Vertex2D{PosX: 0, PosY: 10, R: 1, G: 1, B: 1, A: 1}

	cmd := DrawCommand{TextureID: 1, BlendMode: backend.BlendSourceOver}

	// Two quads with same state should merge.
	b.AddQuadDirect(v0, v1, v2, v3, cmd)
	b.AddQuadDirect(v0, v1, v2, v3, cmd)

	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Len(t, batches[0].Vertices, 8)
	require.Len(t, batches[0].Indices, 12)
}

func TestArenaGrowth(t *testing.T) {
	// Create a batcher with a very small arena to force growth.
	b := NewBatcher(65535, 65535)
	b.vertexArena = make([]Vertex2D, 4)
	b.indexArena = make([]uint16, 6)

	// First quad fits.
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	require.Equal(t, 4, b.vertexPos)
	require.Equal(t, 6, b.indexPos)

	// Second quad forces growth.
	b.AddQuad(20, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	require.Equal(t, 8, b.vertexPos)
	require.Equal(t, 12, b.indexPos)
	require.GreaterOrEqual(t, len(b.vertexArena), 8)
	require.GreaterOrEqual(t, len(b.indexArena), 12)

	// Verify data integrity after growth.
	batches := b.Flush()
	require.Len(t, batches, 1)
	require.Len(t, batches[0].Vertices, 8)
	require.Len(t, batches[0].Indices, 12)
}

func TestArenaResetOnFlush(t *testing.T) {
	b := NewBatcher(65535, 65535)

	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	require.Greater(t, b.vertexPos, 0)
	require.Greater(t, b.indexPos, 0)

	b.Flush()
	require.Equal(t, 0, b.vertexPos)
	require.Equal(t, 0, b.indexPos)
}

func TestArenaResetOnReset(t *testing.T) {
	b := NewBatcher(65535, 65535)

	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.Reset()
	require.Equal(t, 0, b.vertexPos)
	require.Equal(t, 0, b.indexPos)
}

func BenchmarkBatcherFlush100Quads(b *testing.B) {
	batcher := NewBatcher(65535, 65535)
	for b.Loop() {
		for i := 0; i < 100; i++ {
			batcher.AddQuad(
				float32(i*12), 0, 10, 10,
				0, 0, 1, 1,
				1, 1, 1, 1,
				1, backend.BlendSourceOver, 0,
			)
		}
		_ = batcher.Flush()
	}
}

func BenchmarkBatcherFlush1000QuadsMixed(b *testing.B) {
	batcher := NewBatcher(65535, 65535)
	for b.Loop() {
		for i := 0; i < 1000; i++ {
			texID := uint32(i % 8)
			batcher.AddQuad(
				float32(i*12), 0, 10, 10,
				0, 0, 1, 1,
				1, 1, 1, 1,
				texID, backend.BlendSourceOver, 0,
			)
		}
		_ = batcher.Flush()
	}
}
