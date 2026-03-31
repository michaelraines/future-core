// Package util provides utility functions for Future Render, matching
// Ebitengine's ebitenutil package API.
package util

import (
	"fmt"
	"image"
	// Register common image decoders.
	_ "image/jpeg"
	_ "image/png"
	"io/fs"

	futurerender "github.com/michaelraines/future-core"
)

// NewImageFromFileSystem creates a new Image from a file in the given fs.FS.
// It returns both the engine Image (for rendering) and the decoded Go
// image.Image (for inspection), matching Ebitengine's ebitenutil API.
//
// The caller must import the appropriate image decoder for the file format
// (e.g., _ "image/png"). PNG and JPEG are registered by this package.
func NewImageFromFileSystem(fsys fs.FS, path string) (*futurerender.Image, image.Image, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("util: open %s: %w", path, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, nil, fmt.Errorf("util: decode %s: %w", path, err)
	}

	eimg := futurerender.NewImageFromImage(img)
	return eimg, img, nil
}
