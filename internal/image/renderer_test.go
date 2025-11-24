package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"reflect"
	"strings"
	"testing"

	"plexmusic-tui/internal/domain"
)

func coloredImg(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return img
}

func TestRenderCacheKeyIncludesContentHash(t *testing.T) {
	r := NewRendererWithProtocol(domain.ProtocolUnicodeBlocks)
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
	key1 := fmt.Sprintf(
		"%s_%x_%d_%d_%dx%d_%dx%d",
		r.protocol.String(),
		h1[0:8],
		width,
		height,
		bounds1.Dx(),
		bounds1.Dy(),
		bounds1.Min.X,
		bounds1.Min.Y,
	)
	key2 := fmt.Sprintf(
		"%s_%x_%d_%d_%dx%d_%dx%d",
		r.protocol.String(),
		h2[0:8],
		width,
		height,
		bounds2.Dx(),
		bounds2.Dy(),
		bounds2.Min.X,
		bounds2.Min.Y,
	)

	if _, ok := r.cache[key1]; !ok {
		t.Fatalf("expected renderer cache to contain key for first image: %s", key1)
	}
	if _, ok := r.cache[key2]; !ok {
		t.Fatalf("expected renderer cache to contain key for second image: %s", key2)
	}
}

func TestRenderITerm2UsesCanvasPixelSize(t *testing.T) {
	// Force iTerm2 renderer via explicit protocol

	r := NewRendererWithProtocol(domain.ProtocolITerm2)
	img := coloredImg(32, 32, color.RGBA{255, 0, 0, 255})

	widthChar := 10
	heightChar := 5
	expectedWidthPx := widthChar * pixelPerCellITerm2
	expectedHeightPx := heightChar * pixelPerCellITerm2

	out := r.Render(img, widthChar, heightChar)
	// iTerm2 string includes width and height px in `width=%dpx;height=%dpx:`
	expected := fmt.Sprintf("width=%dpx;height=%dpx:", expectedWidthPx, expectedHeightPx)
	if !strings.Contains(out, expected) {
		t.Fatalf("expected iTerm2 render to include %q but got %s", expected, out)
	}
}

func TestRenderKittyCanvasHasExpectedPNGSize(t *testing.T) {
	// Force kitty renderer via explicit protocol

	r := NewRendererWithProtocol(domain.ProtocolKitty)
	img := coloredImg(64, 64, color.RGBA{0, 255, 0, 255})

	widthChar := 9
	heightChar := 7
	expectedPxW := widthChar * pixelPerCellKitty
	expectedPxH := heightChar * pixelPerCellKitty

	// Render will encode to PNG and add to r.pngCache keyed by pointer
	_ = r.Render(img, widthChar, heightChar)

	// Compute the pointer key the same way Render does
	v := reflect.ValueOf(img)
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Ptr {
		t.Fatalf("expected pointer image type")
	}
	_ = v.Pointer()

	// The PNG used by Kitty renderer is in the base64-encoded chunks inside
	// the returned output string; extract those chunks and decode them.
	out := r.Render(img, widthChar, heightChar)
	// Collect all base64 chunks in ":\x1b_G...;<chunk>\x1b\\" sequences
	var chunks []string
	parts := strings.Split(out, "\x1b_G")
	for _, p := range parts {
		if p == "" {
			continue
		}
		// find first ';'
		idx := strings.Index(p, ";")
		if idx == -1 {
			continue
		}
		// find termination sequence '\x1b\'
		term := strings.Index(p, "\x1b\\")
		if term == -1 {
			continue
		}
		chunk := p[idx+1 : term]
		// sanity: skip if chunk contains '=' or looks like flags rather than base64
		if len(chunk) == 0 {
			continue
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		t.Fatalf("failed to extract any encoded chunks from Kitty output")
	}
	encoded := strings.Join(chunks, "")
	decodedBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode Kitty base64: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(decodedBytes))
	if err != nil {
		t.Fatalf("failed to decode cached PNG: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != expectedPxW || bounds.Dy() != expectedPxH {
		t.Fatalf(
			"expected cached PNG size %dx%d, got %dx%d",
			expectedPxW,
			expectedPxH,
			bounds.Dx(),
			bounds.Dy(),
		)
	}
}
