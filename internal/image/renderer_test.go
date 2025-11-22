package image

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
)

func coloredImg(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return img
}

func TestRenderCacheKeyIncludesContentHash(t *testing.T) {
	r := NewRendererWithProtocol(ProtocolUnicodeBlocks)
	img1 := coloredImg(32, 16, color.RGBA{255, 0, 0, 255})
	img2 := coloredImg(32, 16, color.RGBA{0, 255, 0, 255})

	// Render both images; since they differ in content, the output must not be identical
	_ = r.Render(img1, 16, 8)
	_ = r.Render(img2, 16, 8)

	// Sanity: confirm that images produce different content hashes
	var buf1 bytes.Buffer
	var buf2 bytes.Buffer
	if err := png.Encode(&buf1, img1); err != nil {
		t.Fatalf("failed to encode img1: %v", err)
	}
	if err := png.Encode(&buf2, img2); err != nil {
		t.Fatalf("failed to encode img2: %v", err)
	}
	h1 := sha256.Sum256(buf1.Bytes())
	h2 := sha256.Sum256(buf2.Bytes())
	if h1 == h2 {
		t.Fatalf("expected different content hash for images, got same hash")
	}

	// Compute the expected cache key so we can assert the renderer used
	// a per-content cache key instead of reusing same value.
	width := 16
	height := 8
	bounds1 := img1.Bounds()
	bounds2 := img2.Bounds()
	// Build keys using the short hashed prefix used by the renderer
	key1 := fmt.Sprintf("%s_%x_%d_%d_%dx%d_%dx%d",
		r.protocol.String(), h1[0:8], width, height, bounds1.Dx(), bounds1.Dy(), bounds1.Min.X, bounds1.Min.Y)
	key2 := fmt.Sprintf("%s_%x_%d_%d_%dx%d_%dx%d",
		r.protocol.String(), h2[0:8], width, height, bounds2.Dx(), bounds2.Dy(), bounds2.Min.X, bounds2.Min.Y)

	if _, ok := r.cache[key1]; !ok {
		t.Fatalf("expected renderer cache to contain key for first image: %s", key1)
	}
	if _, ok := r.cache[key2]; !ok {
		t.Fatalf("expected renderer cache to contain key for second image: %s", key2)
	}
}
