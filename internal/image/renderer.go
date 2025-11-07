package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"

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

// Renderer handles image rendering to terminal with various protocols
type Renderer struct {
	protocol Protocol
}

// NewRenderer creates a new image renderer, auto-detecting the best protocol
func NewRenderer() *Renderer {
	protocol := DetectImageProtocol()
	return &Renderer{protocol: protocol}
}

// NewRendererWithProtocol creates a new image renderer with a specific protocol
func NewRendererWithProtocol(p Protocol) *Renderer {
	return &Renderer{protocol: p}
}

// SetProtocol changes the protocol used by this renderer for runtime toggles
func (r *Renderer) SetProtocol(p Protocol) {
	r.protocol = p
}

// DetectImageProtocol detects the best image protocol supported by the terminal
func DetectImageProtocol() Protocol {
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

// Render renders an image to terminal output using the detected protocol
// width and height are in character dimensions
func (r *Renderer) Render(img image.Image, width, height int) string {
	if img == nil {
		return ""
	}

	switch r.protocol {
	case ProtocolKitty:
		return r.renderImageKitty(img, width, height)
	case ProtocolITerm2:
		return r.renderImageITerm2(img, width, height)
	case ProtocolSixel:
		return r.renderImageSixel(img, width, height)
	default:
		return r.renderImageUnicodeBlocks(img, width, height)
	}
}

// RenderPlaceholder renders a text-based placeholder when no image is available
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
		if y == centerY {
			// Center the message
			output.WriteString(strings.Repeat(" ", messagePadding))
			output.WriteString(message)
			remaining := width - 2 - messagePadding - len(message)
			if remaining > 0 {
				output.WriteString(strings.Repeat(" ", remaining))
			}
		} else if y == centerY-2 {
			// Music note symbol
			symbol := "♫"
			symbolPadding := (width - 2 - len(symbol)) / 2
			output.WriteString(strings.Repeat(" ", symbolPadding))
			output.WriteString(symbol)
			remaining := width - 2 - symbolPadding - len(symbol)
			if remaining > 0 {
				output.WriteString(strings.Repeat(" ", remaining))
			}
		} else {
			output.WriteString(strings.Repeat(" ", width-2))
		}
		output.WriteString("║\n")
	}

	// Draw bottom border
	output.WriteString("╚" + strings.Repeat("═", width-2) + "╝")

	return output.String()
}

// renderImageKitty renders an image using the Kitty graphics protocol
// Uses virtual placement with content-hash based stable IDs
func (r *Renderer) renderImageKitty(img image.Image, width, height int) string {
	// Resize image to desired pixel dimensions
	pixelWidth := width * 10   // Reduced from 20 to 10 for better performance
	pixelHeight := height * 10
	resized := imaging.Fit(img, pixelWidth, pixelHeight, imaging.Lanczos)

	// Encode image to PNG in memory
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return "" // Fall back to empty on error
	}
	imageData := buf.Bytes()

	// Generate a stable ID based on content hash
	// This ensures the same image gets the same ID every time
	hash := sha256.Sum256(imageData)
	imageID := fmt.Sprintf("%x", hash[:4]) // Use first 4 bytes (8 hex chars) as ID

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(imageData)

	var output strings.Builder

	// Split base64 data into chunks (Kitty has a 4096 byte limit per chunk)
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
			// First chunk: transmit and display
			// a=T: transmit and display immediately
			// f=100: PNG format
			// i=<id>: stable content-based ID
			// c=<width>: columns (character width)
			// r=<height>: rows (character height)
			if isLastChunk {
				output.WriteString(fmt.Sprintf("\x1b_Ga=T,f=100,i=%s,c=%d,r=%d;", imageID, width, height))
			} else {
				output.WriteString(fmt.Sprintf("\x1b_Ga=T,f=100,i=%s,c=%d,r=%d,m=1;", imageID, width, height))
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
	
	// Add newlines to reserve vertical space (align with UI behavior)
	for row := 0; row < height; row++ {
		output.WriteString("\n")
	}
	
	return output.String()
}

// renderImageITerm2 renders an image using iTerm2's inline image protocol
func (r *Renderer) renderImageITerm2(img image.Image, width, height int) string {
	// Resize image - using 20 pixels per character for both dimensions
	pixelWidth := width * 20
	pixelHeight := height * 20
	resized := imaging.Fit(img, pixelWidth, pixelHeight, imaging.Lanczos)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return ""
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// iTerm2 inline image format
	// \x1b]1337;File=inline=1;width=Npx;height=Npx:<base64 data>\a
	bounds := resized.Bounds()
	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%dpx;height=%dpx:%s\a\n",
		bounds.Dx(), bounds.Dy(), encoded)
}

// renderImageSixel renders an image using the Sixel protocol
func (r *Renderer) renderImageSixel(img image.Image, width, height int) string {
	// Note: Full Sixel encoding is complex. For a complete implementation,
	// you would need a proper Sixel encoder library.
	// This is a placeholder that falls back to Unicode blocks
	// A proper implementation would use a library like github.com/mattn/go-sixel
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
