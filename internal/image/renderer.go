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
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/disintegration/imaging"

	"plexmusic-tui/internal/cache"
	"plexmusic-tui/internal/domain"
)

// Renderer handles image rendering to terminal with various protocols
type Renderer struct {
	protocol domain.Protocol
	cache    cache.Cache // Hybrid cache (memory + disk)
	// Cache derived from resized image content and render protocol
	// Cache keyed by contentHash + protocol + bucketed resize dimensions
	pngCache map[string][]byte
	// Pointer map: cache content hash by image pointer to avoid recomputing
	// hash for the same image instance across multiple render calls.
	hashCache map[uintptr]string
	mu        sync.RWMutex
	debug     bool
}

// bucketSize rounds a dimension to the nearest bucket size to improve cache hit rates.
// Small changes in terminal size (e.g., 80->82 columns) will hit the same cache entry.
func bucketSize(size, bucketWidth int) int {
	if bucketWidth <= 0 {
		bucketWidth = 4
	}
	return (size / bucketWidth) * bucketWidth
}

// NewRenderer creates a new image renderer, auto-detecting the best protocol
func NewRenderer() *Renderer {
	protocol := DetectImageProtocol()
	cacheDir, _ := cache.GetCacheDir()
	return &Renderer{
		protocol:  protocol,
		cache:     cache.NewHybridCache(100, cacheDir, 7*24*time.Hour), // 7 day TTL
		pngCache:  make(map[string][]byte),
		hashCache: make(map[uintptr]string),
	}
}

// NewRendererWithProtocol creates a new image renderer with a specific protocol
func NewRendererWithProtocol(p domain.Protocol) *Renderer {
	cacheDir, _ := cache.GetCacheDir()
	return &Renderer{
		protocol:  p,
		cache:     cache.NewHybridCache(100, cacheDir, 7*24*time.Hour), // 7 day TTL
		pngCache:  make(map[string][]byte),
		hashCache: make(map[uintptr]string),
	}
}

// SetDebug toggles debug logging output for the renderer. This is primarily
// for development and should be enabled via CLI flags rather than env vars.
func (r *Renderer) SetDebug(enabled bool) {
	r.debug = enabled
}

// SetProtocol changes the protocol used by this renderer for runtime toggles
func (r *Renderer) SetProtocol(p domain.Protocol) {
	r.protocol = p
}

// ClearHashCache clears the in-memory image hash cache.
// This should be called when album art changes to prevent stale cache hits
// due to Go's memory reuse (a new image may get the same pointer address
// as a previous image, causing incorrect hash lookups).
func (r *Renderer) ClearHashCache() {
	r.mu.Lock()
	r.pngCache = make(map[string][]byte)
	r.hashCache = make(map[uintptr]string)
	r.mu.Unlock()
}

// Precompute resizes and encoded PNGs for common sizes to avoid blocking on
// the first render call. This runs work in a background goroutine and returns
// immediately. The caller should not rely on the precompute having completed.
func (r *Renderer) Precompute(img image.Image, width, height int) {
	// Perform background precompute for the current renderer protocol and
	// supplied width/height.
	go func() {
		var pixelPerCell int
		switch r.protocol {
		case domain.ProtocolKitty:
			pixelPerCell = pixelPerCellKitty
		case domain.ProtocolITerm2:
			pixelPerCell = pixelPerCellITerm2
		default:
			// Unicode blocks don't need PNG encodes; just return.
			return
		}

		// compute content hash
		pixelBytes := func(img image.Image) []byte {
			switch v := img.(type) {
			case *image.RGBA:
				return v.Pix
			case *image.NRGBA:
				return v.Pix
			default:
				b := img.Bounds()
				rgba := image.NewRGBA(b)
				draw.Draw(rgba, b, img, b.Min, draw.Src)
				return rgba.Pix
			}
		}(img)
		var contentHash string
		if len(pixelBytes) > 0 {
			sum := sha256.Sum256(pixelBytes)
			contentHash = fmt.Sprintf("%x", sum[:8])
		}
		if contentHash == "" {
			contentHash = "empty"
		}

		// Precompute the PNG encode/resize in the cache
		_, _, _, _ = r.getOrEncodePng(contentHash, img, r.protocol, width, height, pixelPerCell)
	}()
}

// getPngCacheKey builds a cache key for a pre-encoded resized PNG. Includes
// the content hash (image content), the protocol, the bucketed widths/heights
// and the pixelPerCell which affects the resize.
func (r *Renderer) getPngCacheKey(
	contentHash string,
	protocol domain.Protocol,
	width, height, pixelPerCell int,
) string {
	return fmt.Sprintf("%s_%s_%d_%d_%d", contentHash, protocol.String(), width, height, pixelPerCell)
}

// getOrEncodePng returns encoded PNG bytes for a resized image sized to
// width/height in characters using the specified pixelPerCell. It will fetch
// from the in-memory pngCache if present, otherwise it resizes and encodes
// and stores it. This makes renderers avoid repeated PNG encodes for the
// same image content and resize dimensions.
func (r *Renderer) getOrEncodePng(
	contentHash string,
	img image.Image,
	protocol domain.Protocol,
	width, height, pixelPerCell int,
) ([]byte, int, int, error) {
	key := r.getPngCacheKey(contentHash, protocol, width, height, pixelPerCell)
	pixelW := width * pixelPerCell
	pixelH := height * pixelPerCell
	r.mu.RLock()
	if v, ok := r.pngCache[key]; ok {
		r.mu.RUnlock()
		return v, pixelW, pixelH, nil
	}
	r.mu.RUnlock()

	// Resize and encode
	resized := imaging.Fit(img, pixelW, pixelH, imaging.Lanczos)
	// Center into exact canvas to avoid fractional scaling artifacts
	canvas := imaging.New(pixelW, pixelH, color.Transparent)
	resized = imaging.PasteCenter(canvas, resized)

	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return nil, 0, 0, err
	}
	b := buf.Bytes()

	r.mu.Lock()
	r.pngCache[key] = b
	r.mu.Unlock()
	return b, pixelW, pixelH, nil
}

// DetectImageProtocol detects the best image protocol supported by the terminal
func DetectImageProtocol() domain.Protocol {
	// Detect the protocol by reading terminal environment variables.
	// Check for Kitty terminal
	if os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return domain.ProtocolKitty
	}

	// Check for iTerm2
	if strings.Contains(os.Getenv("TERM_PROGRAM"), "iTerm") {
		return domain.ProtocolITerm2
	}

	// Check for Sixel support via TERM environment variable
	term := os.Getenv("TERM")
	if strings.Contains(term, "sixel") || term == "mlterm" || term == "yaft-256color" {
		return domain.ProtocolSixel
	}

	// Check for xterm with sixel support (some modern xterms)
	if strings.Contains(term, "xterm") {
		// Could query terminal capabilities here, but for simplicity
		// we'll default to Unicode blocks for xterm
		return domain.ProtocolUnicodeBlocks
	}

	// Default to Unicode blocks for maximum compatibility
	return domain.ProtocolUnicodeBlocks
}

// Pixel per-cell constants for Kitty and iTerm2 renderers.
// Choose the target pixel canvas size to reduce seam artifacts produced by
// fractional-scaling when images are resized into terminal cells.
const (
	pixelPerCellKitty  = 10
	pixelPerCellITerm2 = 20
)

// Render renders an image to terminal output using the detected protocol
// width and height are in character dimensions
func (r *Renderer) Render(img image.Image, width, height int) string {
	if img == nil {
		return ""
	}

	// Build a stable content hash from the raw pixel bytes so cache keys
	// depend only on decoded image content. Hashing the RGBA bytes avoids
	// expensive PNG encodes on every render while still preventing
	// pointer/address dependent caching.
	var contentHash string
	var pngBytes []byte
	// Compute the hash from the RGBA pixel slice (first 8 bytes of a SHA256).
	pixelBytes := func(img image.Image) []byte {
		switch v := img.(type) {
		case *image.RGBA:
			return v.Pix
		case *image.NRGBA:
			return v.Pix
		default:
			// Convert to RGBA and return bytes
			b := img.Bounds()
			rgba := image.NewRGBA(b)
			draw.Draw(rgba, b, img, b.Min, draw.Src)
			return rgba.Pix
		}
	}(img)
	if len(pixelBytes) > 0 {
		sum := sha256.Sum256(pixelBytes)
		contentHash = fmt.Sprintf("%x", sum[:8])
	}
	// Fall back to a sentinel hash for empty images.
	if contentHash == "" {
		contentHash = "empty"
	}
	// Store cached png bytes keyed by content hash to avoid re-encoding
	// If we haven't cached PNG for this content yet, lazily encode and cache
	r.mu.RLock()
	_, pngOk := r.pngCache[contentHash]
	r.mu.RUnlock()
	if !pngOk {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil {
			pngBytes = buf.Bytes()
			r.mu.Lock()
			r.pngCache[contentHash] = pngBytes
			r.mu.Unlock()
		}
	}
	// Pointer-based hash cache helps us avoid re-hashing image instances
	v := reflect.ValueOf(img)
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Ptr {
		addr := v.Pointer()
		r.mu.RLock()
		existing, addrOk := r.hashCache[addr]
		r.mu.RUnlock()
		if !addrOk || existing != contentHash {
			r.mu.Lock()
			r.hashCache[addr] = contentHash
			r.mu.Unlock()
		}
	}

	// Generate cache key from protocol, content hash and bucketed sizes.
	// Bucketing dimensions (rounding to nearest 4) improves cache hit rates when
	// terminal size changes slightly (e.g., window resize by a few columns).
	bounds := img.Bounds()
	bucketedW := bucketSize(width, 4)
	bucketedH := bucketSize(height, 4)
	cacheKey := fmt.Sprintf("%s_%s_%d_%d_%dx%d_%dx%d",
		r.protocol.String(),
		contentHash,
		bucketedW, bucketedH,
		bounds.Dx(), bounds.Dy(),
		bounds.Min.X, bounds.Min.Y)

	// Check cache first
	if cached, found := r.cache.Get(cacheKey); found {
		return cached
	}

	// Use bucketed dimensions for actual rendering to ensure cache key matches output
	width = bucketedW
	height = bucketedH

	var result string
	switch r.protocol {
	case domain.ProtocolKitty:
		result = r.renderImageKitty(img, width, height, contentHash)
	case domain.ProtocolITerm2:
		result = r.renderImageITerm2(img, width, height, contentHash)
	case domain.ProtocolSixel:
		result = r.renderImageSixel(img, width, height)
	default:
		result = r.renderImageUnicodeBlocks(img, width, height)
	}

	// Trim trailing whitespace to avoid extra blank lines from renderers.
	result = strings.TrimRight(result, "\r\n ")
	// Cache the result
	_ = r.cache.Set(cacheKey, result)
	return result
}

// RenderPlaceholder draws a boxed text placeholder when no image is available.
func (r *Renderer) RenderPlaceholder(width, height int, message string) string {
	// Create a simple text-based placeholder using box drawing characters
	var output strings.Builder

	// Draw top border
	output.WriteString("╔" + strings.Repeat("═", width-2) + "╗\n")

	// Calculate center position for message
	centerY := height / 2
	messageLen := len(message)
	messagePadding := (width - 2 - messageLen) / 2
	if messagePadding < 0 {
		messagePadding = 0
		if messageLen > width-4 {
			message = message[:width-4] // Truncate if too long
		}
	}

	// Draw middle rows
	for y := 1; y < height-1; y++ {
		output.WriteString("║")
		switch y {
		case centerY:
			// Center the message
			output.WriteString(strings.Repeat(" ", messagePadding))
			output.WriteString(message)
			remaining := width - 2 - messagePadding - len(message)
			if remaining > 0 {
				output.WriteString(strings.Repeat(" ", remaining))
			}
		case centerY - 2:
			// Music note symbol
			symbol := "♫"
			symbolPadding := (width - 2 - len(symbol)) / 2
			output.WriteString(strings.Repeat(" ", symbolPadding))
			output.WriteString(symbol)
			remaining := width - 2 - symbolPadding - len(symbol)
			if remaining > 0 {
				output.WriteString(strings.Repeat(" ", remaining))
			}
		default:
			output.WriteString(strings.Repeat(" ", width-2))
		}
		output.WriteString("║\n")
	}

	// Draw bottom border
	output.WriteString("╚" + strings.Repeat("═", width-2) + "╝")

	return output.String()
}

// renderImageKitty renders an image using the Kitty graphics protocol
// Uses a two-step process: transmit to memory, then place using virtual placements
func (r *Renderer) renderImageKitty(img image.Image, width, height int, contentHash string) string {
	// Pixel size per character for Kitty rendering. Using constants makes it
	// explicit and easier to tune for seam/anti-alias artifacts.
	pixelPerCell := pixelPerCellKitty
	// Use cached resized PNG if available, otherwise encode and cache.
	imageData, _, _, err := r.getOrEncodePng(
		contentHash,
		img,
		domain.ProtocolKitty,
		width,
		height,
		pixelPerCell,
	)
	if err != nil {
		return ""
	}

	// Generate numeric ID from PNG content hash
	hash := sha256.Sum256(imageData)
	// Convert first 4 bytes to a uint32 for numeric ID
	imageID := uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])

	// Base64 encode PNG
	encoded := base64.StdEncoding.EncodeToString(imageData)

	var output strings.Builder

	// Step 1: Transmit image data to Kitty's memory (a=t for transmit only, not display)
	chunkSize := 4096
	numChunks := (len(encoded) + chunkSize - 1) / chunkSize

	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]
		chunkIndex := i / chunkSize
		isLastChunk := chunkIndex == numChunks-1

		if i == 0 {
			// First chunk: transmit to memory (lowercase 'a=t', not 'a=T')
			// a=t: transmit only (store in memory)
			// f=100: PNG format
			// i=<id>: image ID for later reference
			if isLastChunk {
				output.WriteString(fmt.Sprintf("\x1b_Ga=t,f=100,i=%d;", imageID))
			} else {
				output.WriteString(fmt.Sprintf("\x1b_Ga=t,f=100,i=%d,m=1;", imageID))
			}
			output.WriteString(chunk)
			output.WriteString("\x1b\\")
		} else {
			// Continuation chunks
			if isLastChunk {
				output.WriteString("\x1b_Gm=0;")
			} else {
				output.WriteString("\x1b_Gm=1;")
			}
			output.WriteString(chunk)
			output.WriteString("\x1b\\")
		}
	}

	// Step 2: Place the transmitted image using virtual placement
	// Creates a grid where each cell displays part of the image

	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			// a=p: place (display) previously transmitted image
			// i=<id>: which image to place
			// c=<cols>: total columns the image should occupy
			// r=<rows>: total rows the image should occupy
			// X=<col>: which column of the image this cell should show (0-based)
			// Y=<row>: which row of the image this cell should show (0-based)
			// C=1: do not move cursor
			output.WriteString(fmt.Sprintf("\x1b_Ga=p,i=%d,c=%d,r=%d,X=%d,Y=%d,C=1;\x1b\\",
				imageID, width, height, col, row))

			// Output a space character as the placeholder
			output.WriteString(" ")
		}
		// Newline after each row
		if row < height-1 {
			output.WriteString("\n")
		}
	}

	return output.String()
}

// renderImageITerm2 renders an image using iTerm2's inline image protocol
func (r *Renderer) renderImageITerm2(img image.Image, width, height int, contentHash string) string {
	// Pixel size per character for iTerm2 rendering.
	pixelPerCell := pixelPerCellITerm2

	// Use cached resized PNG when available. getOrEncodePng returns encoded
	// PNG as well as the resized pixel dimensions for logging.
	enc, pixelW, pixelH, err := r.getOrEncodePng(contentHash, img, domain.ProtocolITerm2, width, height, pixelPerCell)

	if err != nil {
		return ""
	}
	// Base64 encode PNG bytes (already encoded)
	encoded := base64.StdEncoding.EncodeToString(enc)

	// iTerm2 inline image format
	// \x1b]1337;File=inline=1;width=Npx;height=Npx:<base64 data>\a
	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%dpx;height=%dpx:%s\a\n",
		pixelW, pixelH, encoded)
}

// renderImageSixel renders an image using the Sixel protocol
func (r *Renderer) renderImageSixel(img image.Image, width, height int) string {
	// Sixel not implemented; fallback to Unicode blocks.
	return r.renderImageUnicodeBlocks(img, width, height)
}

// renderImageUnicodeBlocks renders an image using Unicode half-block characters (fallback)
func (r *Renderer) renderImageUnicodeBlocks(img image.Image, width, height int) string {
	if img == nil {
		return ""
	}

	// Resize image to fit terminal dimensions
	// Use half-block characters (▀) which are 2 vertical pixels per character
	// width and height are already in a 2:1 ratio (e.g., 80x40) for square display
	resized := imaging.Fit(img, width, height*2, imaging.Lanczos)

	bounds := resized.Bounds()
	// Pre-allocate output buffer with estimated capacity to reduce allocations
	// Each cell produces ~30-40 bytes of ANSI escape codes + character
	estimatedSize := bounds.Dx() * (bounds.Dy() / 2) * 45
	var output strings.Builder
	output.Grow(estimatedSize)

	// Pre-allocate a hex lookup table for faster color formatting
	hexChars := []byte("0123456789abcdef")
	// Reusable buffer for color string building (avoids fmt.Sprintf per pixel)
	var colorBuf [7]byte // "#RRGGBB"
	colorBuf[0] = '#'

	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// Get colors for top and bottom pixels
			topColor := resized.At(x, y)
			r1, g1, b1, _ := topColor.RGBA()

			// Get bottom pixel color
			bottomColor := topColor // Default to top if no bottom pixel
			if y+1 < bounds.Max.Y {
				bottomColor = resized.At(x, y+1)
			}
			r2, g2, b2, _ := bottomColor.RGBA()

			// Convert from 16-bit color to 8-bit
			r1, g1, b1 = r1>>8, g1>>8, b1>>8
			r2, g2, b2 = r2>>8, g2>>8, b2>>8

			// Build foreground color string directly
			colorBuf[1] = hexChars[r1>>4]
			colorBuf[2] = hexChars[r1&0xf]
			colorBuf[3] = hexChars[g1>>4]
			colorBuf[4] = hexChars[g1&0xf]
			colorBuf[5] = hexChars[b1>>4]
			colorBuf[6] = hexChars[b1&0xf]
			fgColor := string(colorBuf[:])

			// Build background color string directly
			colorBuf[1] = hexChars[r2>>4]
			colorBuf[2] = hexChars[r2&0xf]
			colorBuf[3] = hexChars[g2>>4]
			colorBuf[4] = hexChars[g2&0xf]
			colorBuf[5] = hexChars[b2>>4]
			colorBuf[6] = hexChars[b2&0xf]
			bgColor := string(colorBuf[:])

			// Write ANSI escape codes directly instead of using lipgloss.Style per pixel
			// Format: \x1b[38;2;R;G;B;48;2;R;G;Bm▀\x1b[0m
			// Using hex colors via lipgloss for terminal compatibility
			output.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color(fgColor)).
				Background(lipgloss.Color(bgColor)).
				Render("▀"))
		}
		output.WriteString("\n")
	}

	return output.String()
}
