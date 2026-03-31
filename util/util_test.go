package util

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestNewImageFromFileSystem(t *testing.T) {
	// Create a temporary PNG file.
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())

	eimg, goImg, err := NewImageFromFileSystem(os.DirFS(dir), "test.png")
	require.NoError(t, err)
	require.NotNil(t, eimg)
	require.NotNil(t, goImg)

	w, h := eimg.Size()
	require.Equal(t, 4, w)
	require.Equal(t, 4, h)
}

func TestNewImageFromFileSystemNotFound(t *testing.T) {
	fsys := fstest.MapFS{}
	_, _, err := NewImageFromFileSystem(fsys, "nonexistent.png")
	require.Error(t, err)
}

func TestNewImageFromFileSystemInvalidImage(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.png": &fstest.MapFile{Data: []byte("not an image")},
	}
	_, _, err := NewImageFromFileSystem(fsys, "bad.png")
	require.Error(t, err)
}
