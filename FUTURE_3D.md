# Future 3D Integration Plan

This document captures how 3D rendering will eventually integrate into Future Render
and the specific constraints this imposes on Phase 1 (2D) architecture decisions.

---

## 3D Integration Strategy

3D is not a bolt-on — it is the natural extension of the existing architecture. Phase 1
is designed so that 3D support is **additive**: new passes, new vertex formats, new
shader types — but no rewrites of existing 2D functionality.

### Principle: 2D is 3D with an Orthographic Camera

Following Raylib's proven approach, 2D rendering in Future Render is a specialized case
of 3D rendering:

- 2D sprites use the same pipeline as textured 3D quads, but with an orthographic
  projection matrix and z=0.
- The same `CommandEncoder` interface serves both 2D and 3D draw calls.
- The same `Pipeline` objects work for both — only the shader and vertex format differ.

This unification means 2D and 3D can be freely mixed in the same frame, following
Godot's Viewport model where 3D is rendered first and 2D is composited on top.

---

## Phase 1 Constraints for 3D Compatibility

### Backend Abstraction (internal/backend/)

The following Phase 1 design decisions anticipate 3D:

| Feature | Phase 1 Status | 3D Requirement |
|---|---|---|
| Depth buffer support | `RenderTargetDescriptor.HasDepth` exists | Required for Z-ordering |
| 3D vertex formats | `AttributeFloat3` format defined | Position XYZ, normals |
| Perspective projection | Not used but `Mat4Perspective` exists | Camera projection |
| Depth test/write | `PipelineDescriptor.DepthTest/DepthWrite` exists | Per-pipeline state |
| Face culling | `PipelineDescriptor.CullMode` exists | Back-face culling |
| Compare functions | `CompareFunc` enum defined | Depth/stencil comparison |
| Texture formats | Float16/Float32/Depth formats defined | HDR, shadow maps |

**Constraint**: No Phase 1 change may remove or modify these 3D-ready fields.

### Math Package (math/)

Phase 1 implements the full 3D math suite:

- `Vec3`, `Vec4` — 3D/4D vectors
- `Mat4` — 4x4 transformation matrices with `Perspective`, `LookAt`, `Ortho`
- `Quat` — Quaternion rotations with SLERP
- `AABB` — Axis-aligned bounding boxes
- `Frustum` — View frustum with plane extraction from VP matrix
- `Plane` — 3D plane with distance-to-point
- `Ray` — Ray with AABB and plane intersection

**Constraint**: These types are stable. Their API must not change when 3D rendering is added.

### Render Pipeline (internal/pipeline/)

The `Pipeline` type accepts passes in order. Phase 1 uses 2D passes only. 3D integration
adds new passes without modifying existing ones:

```
Phase 1:                    Phase 2+ (3D):
┌─────────────┐             ┌─────────────┐
│ Clear Pass  │             │ Clear Pass  │
├─────────────┤             ├─────────────┤
│ Sprite Pass │             │ Shadow Pass │  ← NEW
├─────────────┤             ├─────────────┤
│ Present     │             │ Geometry    │  ← NEW
└─────────────┘             ├─────────────┤
                            │ Lighting    │  ← NEW
                            ├─────────────┤
                            │ Sprite Pass │  (existing, unchanged)
                            ├─────────────┤
                            │ Post-FX     │  ← NEW
                            ├─────────────┤
                            │ UI Pass     │  ← NEW
                            ├─────────────┤
                            │ Present     │  (existing, unchanged)
                            └─────────────┘
```

**Constraint**: `Pass` interface must remain stable. New passes implement the same interface.

### Shader System

Phase 1 shaders are vertex + fragment pairs in GLSL. The `ShaderDescriptor` already
separates vertex and fragment source. For 3D:

- Material properties (albedo, metallic, roughness, normal map) will be passed as uniforms.
- A `Material` abstraction will wrap shader + uniform bindings.
- The shader system must eventually support a material graph or PBR preset pipeline.

**Constraint**: `ShaderDescriptor` must not be 2D-specific. Uniform types must include
`mat4` and `vec3` (already present via `SetUniformMat4`, `SetUniformVec4`).

### GLSL→WGSL Translator 2D Assumptions

The shader translator (`internal/shadertranslate/wgsl.go`) has several
2D-specific assumptions that must be addressed for 3D:

| Assumption | Current (2D) | Needed for 3D |
|---|---|---|
| Texture sampling | `textureSampleLevel(..., 0.0)` — hardcoded LOD=0 | `textureSample` (auto LOD from derivatives) for mipmapped textures, or explicit LOD computation |
| Texture types | All textures are `texture_2d<f32>` | `texture_3d`, `texture_cube`, `texture_depth_2d` for shadows/environment maps |
| Vertex format | Hardcoded `Vertex2D` (pos, uv, color) | Normals, tangents, bone weights, multiple UV channels |
| Kage image builtins | `imageSrcNAt` emits bounds-checked 2D sample | May need 3D variants or be replaced by a material system |

The `textureSampleLevel` choice was forced by WebGPU's uniform control flow
requirement — `textureSample` (which supports automatic mip selection) fails
validation in any shader with non-uniform branches. For 3D, options include:
1. Restructure shaders to keep `textureSample` calls in uniform flow
2. Compute LOD explicitly via `textureSampleLevel` with `textureNumLevels`
3. Use `textureSampleGrad` with explicit screen-space derivatives

---

## Planned 3D Features (Phase 2+)

### Scene Graph

A transform hierarchy where each node has:
- Local transform (position, rotation, scale as `Vec3`, `Quat`, `Vec3`)
- Computed world transform (`Mat4`)
- Parent/children relationships
- Optional mesh, light, or camera component

The scene graph is in the Scene/Graph layer — above the pipeline, below the game API.
Users can use it or bypass it with direct draw calls.

### Mesh Rendering

- `Mesh` type with vertex buffers (position, normal, UV, tangent, color)
- Index buffers (uint16 and uint32)
- Multiple sub-meshes per mesh (each with its own material)
- glTF 2.0 as the primary import format

### Lighting

- Directional, point, and spot lights
- Shadow mapping (cascaded shadow maps for directional lights)
- Forward+ with clustered light culling (Godot's approach)
- Ambient/environment lighting via cubemaps

### Materials

- PBR metallic-roughness workflow (glTF-compatible)
- Albedo, metallic, roughness, normal, emissive, occlusion maps
- Custom shader materials for advanced users
- Material presets: unlit, basic lit, standard PBR

### Camera

- Perspective and orthographic projection
- Look-at, orbit, and free-fly camera controllers
- Frustum culling using the `Frustum` type

### Post-Processing

- Render to off-screen target, apply full-screen shader passes
- Common effects: tone mapping, bloom, FXAA, vignette
- User-defined post-processing passes

---

## 3D Vertex Format

Phase 2 will add a `Vertex3D` alongside the existing `Vertex2D`:

```go
type Vertex3D struct {
    PosX, PosY, PosZ    float32  // position
    NormX, NormY, NormZ float32  // normal
    TexU, TexV          float32  // texture coordinates
    TanX, TanY, TanZ    float32  // tangent (for normal mapping)
    R, G, B, A          float32  // vertex color
}
```

The `VertexFormat` descriptor system already supports this — just a new format constant.

---

## What Phase 1 Must NOT Do

1. **Must not hardcode orthographic projection** in the pipeline layer. The projection
   matrix must be a parameter, not a constant.
2. **Must not assume Vertex2D** as the only vertex format. The batcher and pipeline must
   work with any `VertexFormat`.
3. **Must not tie render targets to screen size**. Off-screen targets of arbitrary size
   are needed for shadow maps, reflections, post-processing.
4. **Must not remove depth-related fields** from backend types even though Phase 1 doesn't
   use them.
5. **Must not merge the pipeline and backend layers**. The separation is essential for
   adding 3D passes without touching backend code.

---

## Timeline (Approximate)

| Phase | Scope |
|---|---|
| **Phase 1** (current) | 2D parity with Ebitengine. All foundational architecture. |
| **Phase 2** | 3D mesh rendering, basic lighting, scene graph, camera. |
| **Phase 3** | PBR materials, shadow mapping, post-processing. |
| **Phase 4** | Advanced: instancing, LOD, terrain, skeletal animation. |

Each phase is additive. Phase 1 code should require zero modifications for Phase 2.
