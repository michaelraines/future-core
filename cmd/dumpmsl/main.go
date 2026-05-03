package main

import (
	"fmt"

	"github.com/michaelraines/future-core/internal/shadertranslate"
)

const vertexSource = `#version 330 core
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

func main() {
	result, err := shadertranslate.GLSLToMSLVertex(vertexSource)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(result.Source)
	fmt.Println("--- Uniforms ---")
	for _, u := range result.Uniforms {
		fmt.Printf("  %+v\n", u)
	}
}
