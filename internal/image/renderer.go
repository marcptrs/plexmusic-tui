package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"reflect"
	"strings"
	"sync"

	log "github.com/charmbracelet/log/v2"

	"github.com/charmbracelet/lipgloss"
	"github.com/disintegration/imaging"
)

// Protocol represents the supported terminal image protocols
type Protocol int

const (
	ProtocolUnicodeBlocks Protocol = iota // Fallback using Unicode half-blocks
	ProtocolKitty                         // Kitty graphics protocol
	ProtocolITerm2                        // iTerm2 inline images
	ProtocolSixel                         // Sixel graphics
)

// String returns the string representation of the protocol
func (p Protocol) String() string {
	switch p {
	case ProtocolUnicodeBlocks:
		return "UnicodeBlocks"
	case ProtocolKitty:
		return "Kitty"
	case ProtocolITerm2:
		return "iTerm2"
	case ProtocolSixel:
		return "Sixel"
	default:
		return "Unknown"
	}
}

// Renderer handles image rendering to terminal with various protocols
type Renderer struct {
	protocol Protocol
	cache    map[string]string // Cache rendered output by key
	// Cache derived from image content
	pngCache  map[uintptr][]byte
	hashCache map[uintptr]string
	mu        sync.RWMutex
	debug     bool
}

// NewRenderer creates a new image renderer, auto-detecting the best protocol
func NewRenderer() *Renderer {
	protocol := DetectImageProtocol()
	return &Renderer{
		protocol:  protocol,
		cache:     make(map[string]string),
		pngCache:  make(map[uintptr][]byte),
		hashCache: make(map[uintptr]string),
	}
}

// NewRendererWithProtocol creates a new image renderer with a specific protocol
func NewRendererWithProtocol(p Protocol) *Renderer {
	return &Renderer{
		protocol:  p,
		cache:     make(map[string]string),
		pngCache:  make(map[uintptr][]byte),
		hashCache: make(map[uintptr]string),
	}
}

// SetDebug toggles debug logging output for the renderer. This is primarily
// for development and should be enabled via CLI flags rather than env vars.
func (r *Renderer) SetDebug(enabled bool) {
	r.debug = enabled
}

// SetProtocol changes the protocol used by this renderer for runtime toggles
func (r *Renderer) SetProtocol(p Protocol) {
	r.protocol = p
}

// DetectImageProtocol detects the best image protocol supported by the terminal
func DetectImageProtocol() Protocol {
	// Detect the protocol by reading terminal environment variables.
	// Check for Kitty terminal
	if os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return ProtocolKitty
	}

	// Check for iTerm2
	if strings.Contains(os.Getenv("TERM_PROGRAM"), "iTerm") {
		return ProtocolITerm2
	}

	// Check for Sixel support via TERM environment variable
	term := os.Getenv("TERM")
	if strings.Contains(term, "sixel") || term == "mlterm" || term == "yaft-256color" {
		return ProtocolSixel
	}

	// Check for xterm with sixel support (some modern xterms)
	if strings.Contains(term, "xterm") {
		// Could query terminal capabilities here, but for simplicity
		// we'll default to Unicode blocks for xterm
		return ProtocolUnicodeBlocks
	}

	// Default to Unicode blocks for maximum compatibility
	return ProtocolUnicodeBlocks
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

	// Build a stable content hash (PNG encoding) and reuse cached encodings
	// via the image pointer when possible to avoid re-encoding on every call.
	var contentHash string
	var addr uintptr
	v := reflect.ValueOf(img)
	// If img is an interface, get its underlying value
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Ptr {
		addr = v.Pointer()
	}
	if addr != 0 {
		r.mu.RLock()
		ch, ok := r.hashCache[addr]
		r.mu.RUnlock()
		if ok {
			contentHash = ch
		} else {
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err == nil {
				sum := sha256.Sum256(buf.Bytes())
				contentHash = fmt.Sprintf("%x", sum[:8])
				r.mu.Lock()
				r.hashCache[addr] = contentHash
				r.pngCache[addr] = buf.Bytes()
				r.mu.Unlock()
			}
		}
	} else {
		// Fallback for non-pointer image types - encode directly
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil {
			sum := sha256.Sum256(buf.Bytes())
			contentHash = fmt.Sprintf("%x", sum[:8])
		}
	}

	// Generate cache key from protocol, content hash and sizes
	bounds := img.Bounds()
	cacheKey := fmt.Sprintf("%s_%s_%d_%d_%dx%d_%dx%d",
		r.protocol.String(),
		contentHash,
		width, height,
		bounds.Dx(), bounds.Dy(),
		bounds.Min.X, bounds.Min.Y)

	// Check cache first
	if cached, found := r.cache[cacheKey]; found {
		return cached
	}

	var result string
	switch r.protocol {
	case ProtocolKitty:
		result = r.renderImageKitty(img, width, height)
	case ProtocolITerm2:
		result = r.renderImageITerm2(img, width, height)
	case ProtocolSixel:
		result = r.renderImageSixel(img, width, height)
	default:
		result = r.renderImageUnicodeBlocks(img, width, height)
	}

	// Trim trailing whitespace to avoid extra blank lines from renderers.
	result = strings.TrimRight(result, "\r\n ")
	// Cache the result
	r.cache[cacheKey] = result
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
func (r *Renderer) renderImageKitty(img image.Image, width, height int) string {
	// Pixel size per character for Kitty rendering. Using constants makes it
	// explicit and easier to tune for seam/anti-alias artifacts.
	pixelPerCell := pixelPerCellKitty
	pixelWidth := width * pixelPerCell
	pixelHeight := height * pixelPerCell

	// Fit image to pixel bounds and paste into an exact-size canvas to avoid
	// fractional-scaling seams (maps PNG pixels -> terminal cell grid).
	resized := imaging.Fit(img, pixelWidth, pixelHeight, imaging.Lanczos)
	canvas := imaging.New(pixelWidth, pixelHeight, color.Transparent)
	resized = imaging.PasteCenter(canvas, resized)

	// Encode image to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return "" // Fall back to empty on error
	}
	imageData := buf.Bytes()

	// Generate numeric ID from PNG content hash
	hash := sha256.Sum256(imageData)
	// Convert first 4 bytes to a uint32 for numeric ID
	imageID := uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])

	// Base64 encode PNG
	encoded := base64.StdEncoding.EncodeToString(imageData)

	var output strings.Builder
	if r.debug {
		log.Debug("KittyRender", "charW", width, "charH", height, "pixelW", pixelWidth, "pixelH", pixelHeight, "resizedW", resized.Bounds().Dx(), "resizedH", resized.Bounds().Dy())
	}

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
func (r *Renderer) renderImageITerm2(img image.Image, width, height int) string {
	// Pixel size per character for iTerm2 rendering.
	pixelPerCell := pixelPerCellITerm2
	pixelWidth := width * pixelPerCell
	pixelHeight := height * pixelPerCell

	// Use Fit + exact canvas to avoid fractional scaling seams just like
	// Kitty rendering.
	resized := imaging.Fit(img, pixelWidth, pixelHeight, imaging.Lanczos)
	if r.debug {
		log.Debug("ITerm2Render", "charW", width, "charH", height, "pixelW", pixelWidth, "pixelH", pixelHeight, "resizedW", resized.Bounds().Dx(), "resizedH", resized.Bounds().Dy())
	}
	canvas := imaging.New(pixelWidth, pixelHeight, color.Transparent)
	resized = imaging.PasteCenter(canvas, resized)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return ""
	}

	// Base64 encode PNG
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// iTerm2 inline image format
	// \x1b]1337;File=inline=1;width=Npx;height=Npx:<base64 data>\a
	bounds := resized.Bounds()
	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%dpx;height=%dpx:%s\a\n",
		bounds.Dx(), bounds.Dy(), encoded)
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

	var output strings.Builder
	bounds := resized.Bounds()

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

			// Use upper half block with top color as foreground and bottom color as background
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r1, g1, b1))).
				Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r2, g2, b2)))

			output.WriteString(style.Render("▀"))
		}
		output.WriteString("\n")
	}

	return output.String()
}
