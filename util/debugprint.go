package futureutil

import (
	"bytes"
	_ "embed" // for //go:embed text.png
	"image"
	_ "image/png" // registers the PNG decoder used by image.Decode at runtime
	"strings"
	"sync"

	futurerender "github.com/michaelraines/future-core"
)

//go:embed text.png
var textPNG []byte

var (
	debugPrintAtlasOnce sync.Once
	debugPrintAtlas     *futurerender.Image
	debugPrintSubImages = map[rune]*futurerender.Image{}
)

func initDebugPrintAtlas() {
	debugPrintAtlasOnce.Do(func() {
		img, _, err := image.Decode(bytes.NewReader(textPNG))
		if err != nil {
			panic("futureutil: failed to decode debug font: " + err.Error())
		}
		debugPrintAtlas = futurerender.NewImageFromImage(img)
	})
}

// DebugPrint draws msg on the image at the default position (1, 1).
// It uses the same embedded bitmap font as Ebitengine's ebitenutil.DebugPrint.
func DebugPrint(img *futurerender.Image, msg string) {
	DebugPrintAt(img, msg, 1, 1)
}

// DebugPrintAt draws msg on the image at position (ox, oy).
// It uses the same embedded bitmap font and layout as Ebitengine's
// ebitenutil.DebugPrintAt for pixel-identical output.
func DebugPrintAt(img *futurerender.Image, msg string, ox, oy int) {
	if img == nil || msg == "" {
		return
	}

	initDebugPrintAtlas()

	const (
		cw = 6
		ch = 16
	)

	atlasW := debugPrintAtlas.Bounds().Dx()

	op := &futurerender.DrawImageOptions{}
	x := 0
	y := 0
	for _, line := range strings.Split(msg, "\n") {
		for _, c := range line {
			s, ok := debugPrintSubImages[c]
			if !ok {
				n := atlasW / cw
				sx := (int(c) % n) * cw
				sy := (int(c) / n) * ch
				s = debugPrintAtlas.SubImage(image.Rect(sx, sy, sx+cw, sy+ch))
				debugPrintSubImages[c] = s
			}
			op.GeoM.Reset()
			op.GeoM.Translate(float64(x), float64(y))
			op.GeoM.Translate(float64(ox+1), float64(oy))
			img.DrawImage(s, op)
			x += cw
		}
		x = 0
		y += ch
	}
}
