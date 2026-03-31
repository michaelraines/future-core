// Package futureutil provides utility functions for Future Render, matching
// Ebitengine's ebitenutil package API.
package futureutil

import (
	"fmt"
	goimage "image"
	_ "image/jpeg" // Register JPEG decoder.
	_ "image/png"  // Register PNG decoder.
	"io/fs"

	futurerender "github.com/michaelraines/future-core"
)

// NewImageFromFileSystem creates a new Image from a file in the given fs.FS.
// It returns both the engine Image (for rendering) and the decoded Go
// image.Image (for inspection), matching Ebitengine's ebitenutil API.
//
// The caller must import the appropriate image decoder for the file format
// (e.g., _ "image/png"). PNG and JPEG are registered by this package.
func NewImageFromFileSystem(fsys fs.FS, path string) (eimg *futurerender.Image, img goimage.Image, err error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("futureutil: open %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("futureutil: close %s: %w", path, cerr)
		}
	}()

	img, _, err = goimage.Decode(f)
	if err != nil {
		return nil, nil, fmt.Errorf("futureutil: decode %s: %w", path, err)
	}

	eimg = futurerender.NewImageFromImage(img)
	return eimg, img, nil
}
