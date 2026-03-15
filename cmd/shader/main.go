//go:build glfw

// Command shader demonstrates custom shader rendering: a fullscreen quad
// with a time-varying gradient effect using DrawRectShader and raw GLSL.
//
// Build: go build -tags glfw ./cmd/shader
// Run:   ./shader
package main

import (
	"log"
	"time"

	futurerender "github.com/michaelraines/future-render"
)

const (
	screenW = 640
	screenH = 480
)

const vertexShader = `#version 330 core

layout(location = 0) in vec2 aPosition;
layout(location = 1) in vec2 aTexCoord;
layout(location = 2) in vec4 aColor;

uniform mat4 uProjection;

out vec2 vTexCoord;
out vec4 vColor;

void main() {
    vTexCoord = aTexCoord;
    vColor = aColor;
    gl_Position = uProjection * vec4(aPosition, 0.0, 1.0);
}
`

const fragmentShader = `#version 330 core

in vec2 vTexCoord;
in vec4 vColor;

uniform sampler2D uTexture;
uniform float uTime;

out vec4 fragColor;

void main() {
    // Create a gradient based on texture coordinates modulated by time.
    float r = 0.5 + 0.5 * sin(uTime + vTexCoord.x * 6.2831);
    float g = 0.5 + 0.5 * sin(uTime * 1.3 + vTexCoord.y * 6.2831);
    float b = 0.5 + 0.5 * sin(uTime * 0.7 + (vTexCoord.x + vTexCoord.y) * 3.1415);
    fragColor = vec4(r, g, b, 1.0);
}
`

type shaderGame struct {
	shader    *futurerender.Shader
	startTime time.Time
}

func (g *shaderGame) Update() error {
	if futurerender.IsKeyPressed(futurerender.KeyEscape) {
		return futurerender.ErrTermination
	}
	return nil
}

func (g *shaderGame) Draw(screen *futurerender.Image) {
	if g.shader == nil {
		screen.Fill(futurerender.ColorFromRGBA(0, 0, 0, 1))
		return
	}

	elapsed := float32(time.Since(g.startTime).Seconds())

	screen.DrawRectShader(screenW, screenH, g.shader, &futurerender.DrawRectShaderOptions{
		Uniforms: map[string]any{
			"uTime": elapsed,
		},
	})
}

func (g *shaderGame) Layout(_, _ int) (int, int) {
	return screenW, screenH
}

func main() {
	futurerender.SetWindowSize(screenW, screenH)
	futurerender.SetWindowTitle("Future Render \u2014 Shader Example")

	game := &shaderGame{}
	wrapper := &lazyInitGame{game: game}

	if err := futurerender.RunGame(wrapper); err != nil {
		log.Fatal(err)
	}
}

// lazyInitGame wraps shaderGame to compile the shader on first Update,
// after the GPU device is ready.
type lazyInitGame struct {
	game   *shaderGame
	inited bool
}

func (g *lazyInitGame) Update() error {
	if !g.inited {
		var err error
		g.game.shader, err = futurerender.NewShaderFromGLSL(
			[]byte(vertexShader),
			[]byte(fragmentShader),
		)
		if err != nil {
			return err
		}
		g.game.startTime = time.Now()
		g.inited = true
	}
	return g.game.Update()
}

func (g *lazyInitGame) Draw(screen *futurerender.Image) {
	g.game.Draw(screen)
}

func (g *lazyInitGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.game.Layout(outsideWidth, outsideHeight)
}
