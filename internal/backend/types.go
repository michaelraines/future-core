package backend

// VertexFormat describes the layout of vertex attributes in a vertex buffer.
type VertexFormat struct {
	Attributes []VertexAttribute
	Stride     int // bytes per vertex
}

// VertexAttribute describes a single vertex attribute.
type VertexAttribute struct {
	Name   string
	Format AttributeFormat
	Offset int // byte offset within the vertex
}

// AttributeFormat specifies the data type and component count of an attribute.
type AttributeFormat int

// AttributeFormat constants.
const (
	// AttributeFloat2 is a 2-component float32 attribute.
	AttributeFloat2 AttributeFormat = iota // 2x float32
	AttributeFloat3                        // 3x float32
	AttributeFloat4                        // 4x float32

	// AttributeByte4Norm is a 4-component normalized uint8 attribute.
	AttributeByte4Norm // 4x uint8, normalized to [0,1]
)

// AttributeFormatSize returns the byte size of an attribute format.
func AttributeFormatSize(f AttributeFormat) int {
	switch f {
	case AttributeFloat2:
		return 8
	case AttributeFloat3:
		return 12
	case AttributeFloat4:
		return 16
	case AttributeByte4Norm:
		return 4
	default:
		return 0
	}
}

// BlendFactor specifies a per-channel source or destination blend factor.
type BlendFactor uint8

// BlendFactor constants. The set mirrors the GLSL/GPU vocabulary used by
// every target API (WebGPU, Metal, Vulkan, OpenGL, WebGL2, DX12).
const (
	BlendFactorZero             BlendFactor = iota // 0
	BlendFactorOne                                 // 1
	BlendFactorSrcAlpha                            // srcA
	BlendFactorOneMinusSrcAlpha                    // 1 - srcA
	BlendFactorDstAlpha                            // dstA
	BlendFactorOneMinusDstAlpha                    // 1 - dstA
	BlendFactorSrcColor                            // srcRGB
	BlendFactorOneMinusSrcColor                    // 1 - srcRGB
	BlendFactorDstColor                            // dstRGB
	BlendFactorOneMinusDstColor                    // 1 - dstRGB
)

// BlendOperation specifies how the weighted source and destination components
// are combined to produce the final pixel value.
type BlendOperation uint8

// BlendOperation constants.
const (
	BlendOpAdd             BlendOperation = iota // src*srcF + dst*dstF
	BlendOpSubtract                              // src*srcF - dst*dstF
	BlendOpReverseSubtract                       // dst*dstF - src*srcF
	BlendOpMin                                   // min(src, dst) (factors ignored)
	BlendOpMax                                   // max(src, dst) (factors ignored)
)

// BlendMode specifies how source and destination colors are combined.
//
// A BlendMode is a value type. Zero value == BlendNone (no blending, opaque
// write). Preset values (BlendSourceOver, BlendAdditive, BlendMultiplicative,
// BlendPremultiplied) are defined below, and any custom combination of
// factors/operations is permitted — no information is lost between the
// public Blend struct and the pipeline builder.
type BlendMode struct {
	// Enabled is false for BlendNone. A pipeline with Enabled=false
	// writes source pixels directly to the framebuffer, ignoring all
	// factor/operation fields.
	Enabled bool

	SrcFactorRGB   BlendFactor
	SrcFactorAlpha BlendFactor
	DstFactorRGB   BlendFactor
	DstFactorAlpha BlendFactor

	OpRGB   BlendOperation
	OpAlpha BlendOperation
}

// Preset BlendMode values. These remain usable as named constants and as
// comparable values in switch statements (structs of comparable fields are
// comparable in Go).
var (
	// BlendNone disables blending entirely. Source pixels overwrite dst.
	BlendNone = BlendMode{Enabled: false}

	// BlendSourceOver is standard alpha blending with premultiplied source:
	//   result = src + dst*(1 - srcA)
	BlendSourceOver = BlendMode{
		Enabled:        true,
		SrcFactorRGB:   BlendFactorOne,
		SrcFactorAlpha: BlendFactorOne,
		DstFactorRGB:   BlendFactorOneMinusSrcAlpha,
		DstFactorAlpha: BlendFactorOneMinusSrcAlpha,
		OpRGB:          BlendOpAdd,
		OpAlpha:        BlendOpAdd,
	}

	// BlendAdditive adds source to destination: result = src + dst.
	BlendAdditive = BlendMode{
		Enabled:        true,
		SrcFactorRGB:   BlendFactorOne,
		SrcFactorAlpha: BlendFactorOne,
		DstFactorRGB:   BlendFactorOne,
		DstFactorAlpha: BlendFactorOne,
		OpRGB:          BlendOpAdd,
		OpAlpha:        BlendOpAdd,
	}

	// BlendMultiplicative multiplies source by destination: result = src*dst.
	BlendMultiplicative = BlendMode{
		Enabled:        true,
		SrcFactorRGB:   BlendFactorDstColor,
		SrcFactorAlpha: BlendFactorDstAlpha,
		DstFactorRGB:   BlendFactorZero,
		DstFactorAlpha: BlendFactorZero,
		OpRGB:          BlendOpAdd,
		OpAlpha:        BlendOpAdd,
	}

	// BlendPremultiplied is the same factors as BlendSourceOver — kept
	// as a named alias for legibility in call sites that explicitly
	// want to signal they are working in premultiplied color space.
	BlendPremultiplied = BlendSourceOver
)

// CompareFunc specifies a depth/stencil comparison function.
type CompareFunc int

// CompareFunc constants.
const (
	CompareNever CompareFunc = iota
	CompareLess
	CompareLessEqual
	CompareEqual
	CompareGreaterEqual
	CompareGreater
	CompareNotEqual
	CompareAlways
)

// CullMode specifies which faces to cull.
type CullMode int

// CullMode constants.
const (
	CullNone  CullMode = iota
	CullFront          // Cull front-facing triangles
	CullBack           // Cull back-facing triangles
)

// PrimitiveType specifies the type of primitives to draw.
type PrimitiveType int

// PrimitiveType constants.
const (
	PrimitiveTriangles     PrimitiveType = iota // Triangle list
	PrimitiveTriangleStrip                      // Triangle strip
	PrimitiveLines                              // Line list
	PrimitiveLineStrip                          // Line strip
	PrimitivePoints                             // Point list
)

// TextureFormat specifies the pixel format of a texture.
type TextureFormat int

// TextureFormat constants.
const (
	TextureFormatRGBA8    TextureFormat = iota // 4x uint8, standard
	TextureFormatRGB8                          // 3x uint8, no alpha
	TextureFormatR8                            // 1x uint8 (grayscale/alpha)
	TextureFormatRGBA16F                       // 4x float16, HDR
	TextureFormatRGBA32F                       // 4x float32, HDR
	TextureFormatDepth24                       // 24-bit depth
	TextureFormatDepth32F                      // 32-bit float depth
)

// TextureFilter specifies the filtering mode for texture sampling.
type TextureFilter int

// TextureFilter constants.
const (
	FilterNearest TextureFilter = iota
	FilterLinear
)

// TextureWrap specifies how texture coordinates outside [0,1] are handled.
type TextureWrap int

// TextureWrap constants.
const (
	WrapClamp TextureWrap = iota
	WrapRepeat
	WrapMirror
)

// FillRule specifies how overlapping triangles are composited.
type FillRule int

// FillRule constants.
const (
	FillRuleNonZero FillRule = iota // Default: all fragments drawn
	FillRuleEvenOdd                 // Odd-overlap regions visible (stencil-based XOR)
)

// StencilOp specifies what happens to stencil buffer values.
type StencilOp int

// StencilOp constants.
const (
	StencilKeep     StencilOp = iota // Keep current value
	StencilZero                      // Set to zero
	StencilReplace                   // Set to reference value
	StencilIncr                      // Increment (clamp)
	StencilDecr                      // Decrement (clamp)
	StencilInvert                    // Bitwise invert
	StencilIncrWrap                  // Increment (wrap)
	StencilDecrWrap                  // Decrement (wrap)
)

// StencilDescriptor describes stencil test configuration.
type StencilDescriptor struct {
	Func      CompareFunc
	Ref       int
	Mask      uint32
	SFail     StencilOp // stencil test fails
	DPFail    StencilOp // depth test fails
	DPPass    StencilOp // both pass
	WriteMask uint32
}

// IndexFormat specifies the data type of index buffer elements.
type IndexFormat int

// IndexFormat constants.
const (
	IndexUint16 IndexFormat = iota
	IndexUint32
)

// Viewport defines the rendering viewport.
type Viewport struct {
	X, Y          int
	Width, Height int
}

// ScissorRect defines the scissor test rectangle.
type ScissorRect struct {
	X, Y          int
	Width, Height int
}
