package batch

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michaelraines/future-render/internal/backend"
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

func TestBatcherSortOrder(t *testing.T) {
	b := NewBatcher(65535, 65535)

	// Add commands in reverse order: high shader/blend/texture first
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 3, backend.BlendAdditive, 2)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 0)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 2, backend.BlendSourceOver, 0)
	b.AddQuad(0, 0, 10, 10, 0, 0, 1, 1, 1, 1, 1, 1, 1, backend.BlendSourceOver, 1)

	batches := b.Flush()

	// Should be sorted by shader, then blend, then texture
	// shader=0, blend=SourceOver(1), tex=1
	// shader=0, blend=SourceOver(1), tex=2
	// shader=1, blend=SourceOver(1), tex=1
	// shader=2, blend=Additive(2), tex=3
	require.Len(t, batches, 4)
	require.Equal(t, uint32(0), batches[0].ShaderID)
	require.Equal(t, backend.BlendSourceOver, batches[0].BlendMode)
	require.Equal(t, uint32(1), batches[0].TextureID)

	require.Equal(t, uint32(0), batches[1].ShaderID)
	require.Equal(t, backend.BlendSourceOver, batches[1].BlendMode)
	require.Equal(t, uint32(2), batches[1].TextureID)

	require.Equal(t, uint32(1), batches[2].ShaderID)
	require.Equal(t, backend.BlendSourceOver, batches[2].BlendMode)
	require.Equal(t, uint32(1), batches[2].TextureID)

	require.Equal(t, uint32(2), batches[3].ShaderID)
	require.Equal(t, backend.BlendAdditive, batches[3].BlendMode)
	require.Equal(t, uint32(3), batches[3].TextureID)
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
