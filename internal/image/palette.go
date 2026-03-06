package image

import (
	"image"
	"image/color"
	"math"
	"sync"
)

// Palette represents a set of dominant colors extracted from an image.
// Uses perceptually-accurate colors for UI composition.
type Palette struct {
	Primary   color.Color // Most dominant color
	Secondary color.Color // High-contrast color
	Accent    color.Color // Vibrant variant
	Muted     color.Color // Desaturated variant for text contrast
}

// paletteCache caches extracted palettes by image pointer to avoid recomputation
var (
	paletteCache = make(map[uintptr]*Palette)
	paletteMu    sync.RWMutex
)

// LAB represents a color in LAB color space (more perceptually accurate than RGB)
type LAB struct {
	L, A, B float64
}

// RGB represents a color in sRGB color space
type sRGB struct {
	R, G, B float64 // [0, 1] range
}

// ExtractPalette extracts a Palette of dominant colors from an image using k-means clustering.
// Results are cached by image pointer to avoid recomputation.
func ExtractPalette(img image.Image) *Palette {
	ptr := uintptr(0)
	switch img := img.(type) {
	case *image.RGBA:
		if len(img.Pix) > 0 {
			ptr = uintptr(img.Pix[0])
		}
	case *image.NRGBA:
		if len(img.Pix) > 0 {
			ptr = uintptr(img.Pix[0])
		}
	case *image.YCbCr:
		if len(img.Y) > 0 {
			ptr = uintptr(img.Y[0])
		}
	default:
		// For unknown types, compute without caching
		return extractPaletteImpl(img)
	}

	// Check cache
	paletteMu.RLock()
	if pal, exists := paletteCache[ptr]; exists {
		paletteMu.RUnlock()
		return pal
	}
	paletteMu.RUnlock()

	// Extract and cache
	pal := extractPaletteImpl(img)
	paletteMu.Lock()
	paletteCache[ptr] = pal
	paletteMu.Unlock()

	return pal
}

// ClearPaletteCache clears the palette cache. Call this when album art changes.
func ClearPaletteCache() {
	paletteMu.Lock()
	paletteCache = make(map[uintptr]*Palette)
	paletteMu.Unlock()
}

// extractPaletteImpl performs the actual palette extraction algorithm.
func extractPaletteImpl(img image.Image) *Palette {
	// Sample image pixels, skipping every 2-4 pixels for performance
	pixels := samplePixels(img)
	if len(pixels) == 0 {
		return defaultPalette()
	}

	// Convert to LAB color space for perceptually-accurate clustering
	labPixels := make([]LAB, len(pixels))
	for i, p := range pixels {
		labPixels[i] = srgbToLab(p)
	}

	// K-means clustering to find 4 dominant colors
	k := 4
	clusters := kMeansClustering(labPixels, k)

	// Convert back to RGB and sort by luminance (bright first)
	clusterColors := make([]color.Color, len(clusters))
	for i, lab := range clusters {
		rgb := labToSrgb(lab)
		clusterColors[i] = rgb.toColor()
	}

	// Sort by luminance (bright colors first)
	sortByLuminance(clusterColors)

	// Generate palette variants from dominant colors
	return generatePalette(clusterColors)
}

// samplePixels extracts a sample of pixels from the image for faster processing
func samplePixels(img image.Image) []sRGB {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Skip pixels based on image size to keep sampling consistent
	step := 2
	if width > 400 || height > 400 {
		step = 4
	}

	samples := []sRGB{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, _ := img.At(x, y).RGBA()
			samples = append(samples, sRGB{
				R: float64(r>>8) / 255.0,
				G: float64(g>>8) / 255.0,
				B: float64(b>>8) / 255.0,
			})
		}
	}
	return samples
}

// srgbToLab converts sRGB to LAB color space using D65 illuminant
func srgbToLab(rgb sRGB) LAB {
	// Linearize sRGB
	r := srgbGammaToLinear(rgb.R)
	g := srgbGammaToLinear(rgb.G)
	b := srgbGammaToLinear(rgb.B)

	// Convert to XYZ (sRGB matrix)
	x := r*0.4124564 + g*0.3575761 + b*0.1804375
	y := r*0.2126729 + g*0.7151522 + b*0.0721750
	z := r*0.0193339 + g*0.1191920 + b*0.9503041

	// Normalize by D65 illuminant
	x /= 0.95047
	y /= 1.00000
	z /= 1.08883

	// Convert to LAB
	fX := xyzToLabComponent(x)
	fY := xyzToLabComponent(y)
	fZ := xyzToLabComponent(z)

	return LAB{
		L: 116*fY - 16,
		A: 500 * (fX - fY),
		B: 200 * (fY - fZ),
	}
}

// labToSrgb converts LAB to sRGB color space
func labToSrgb(lab LAB) sRGB {
	// Convert LAB to XYZ
	fY := (lab.L + 16) / 116
	fX := lab.A/500 + fY
	fZ := fY - lab.B/200

	// Inverse function
	xyzInverseComponent := func(t float64) float64 {
		delta := 6.0 / 29.0
		if t > delta {
			return t * t * t
		}
		return (t - 4.0/29.0) * 3 * delta * delta
	}

	x := xyzInverseComponent(fX) * 0.95047
	y := xyzInverseComponent(fY) * 1.00000
	z := xyzInverseComponent(fZ) * 1.08883

	// XYZ to sRGB
	r := x*3.2404542 + y*-1.5371385 + z*-0.4985314
	g := x*-0.9692660 + y*1.8760108 + z*0.0415560
	b := x*0.0556434 + y*-0.2040259 + z*1.0572252

	// De-linearize sRGB
	return sRGB{
		R: linearToSrgbGamma(math.Max(0, math.Min(1, r))),
		G: linearToSrgbGamma(math.Max(0, math.Min(1, g))),
		B: linearToSrgbGamma(math.Max(0, math.Min(1, b))),
	}
}

// srgbGammaToLinear converts gamma-encoded sRGB component to linear RGB
func srgbGammaToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// linearToSrgbGamma converts linear RGB component to gamma-encoded sRGB
func linearToSrgbGamma(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// xyzToLabComponent is the f(t) function used in XYZ to LAB conversion
func xyzToLabComponent(t float64) float64 {
	delta := 6.0 / 29.0
	if t > delta*delta*delta {
		return math.Cbrt(t)
	}
	return t/(3*delta*delta) + (4.0 / 29.0)
}

// colorDistance calculates the perceptual distance between two colors in LAB space
func colorDistance(lab1, lab2 LAB) float64 {
	dL := lab1.L - lab2.L
	dA := lab1.A - lab2.A
	dB := lab1.B - lab2.B
	// CIE76 distance formula (simple Euclidean distance in LAB space)
	return math.Sqrt(dL*dL + dA*dA + dB*dB)
}

// kMeansClustering performs k-means clustering on LAB pixels to find dominant colors
func kMeansClustering(pixels []LAB, k int) []LAB {
	if len(pixels) == 0 {
		return []LAB{}
	}

	if len(pixels) < k {
		k = len(pixels)
	}

	// Initialize centroids with random pixels
	centroids := make([]LAB, k)
	step := len(pixels) / k
	for i := 0; i < k; i++ {
		centroids[i] = pixels[i*step]
	}

	// K-means iterations
	maxIterations := 10
	for iter := 0; iter < maxIterations; iter++ {
		// Assign pixels to nearest centroid
		assignments := make([][]LAB, k)
		for _, pixel := range pixels {
			nearest := 0
			minDist := colorDistance(pixel, centroids[0])
			for i := 1; i < k; i++ {
				dist := colorDistance(pixel, centroids[i])
				if dist < minDist {
					minDist = dist
					nearest = i
				}
			}
			assignments[nearest] = append(assignments[nearest], pixel)
		}

		// Recompute centroids
		oldCentroids := make([]LAB, k)
		copy(oldCentroids, centroids)

		for i := 0; i < k; i++ {
			if len(assignments[i]) == 0 {
				// Keep old centroid if cluster is empty
				continue
			}
			// Calculate mean of cluster
			sumL, sumA, sumB := 0.0, 0.0, 0.0
			for _, pixel := range assignments[i] {
				sumL += pixel.L
				sumA += pixel.A
				sumB += pixel.B
			}
			n := float64(len(assignments[i]))
			centroids[i] = LAB{
				L: sumL / n,
				A: sumA / n,
				B: sumB / n,
			}
		}

		// Check convergence
		converged := true
		for i := 0; i < k; i++ {
			if colorDistance(centroids[i], oldCentroids[i]) > 0.1 {
				converged = false
				break
			}
		}
		if converged {
			break
		}
	}

	return centroids
}

// sortByLuminance sorts colors by LAB luminance (L value) in descending order
func sortByLuminance(colors []color.Color) {
	for i := 0; i < len(colors)-1; i++ {
		for j := i + 1; j < len(colors); j++ {
			r1, g1, b1, _ := colors[i].RGBA()
			r2, g2, b2, _ := colors[j].RGBA()

			lab1 := srgbToLab(sRGB{
				R: float64(r1>>8) / 255.0,
				G: float64(g1>>8) / 255.0,
				B: float64(b1>>8) / 255.0,
			})
			lab2 := srgbToLab(sRGB{
				R: float64(r2>>8) / 255.0,
				G: float64(g2>>8) / 255.0,
				B: float64(b2>>8) / 255.0,
			})

			if lab2.L > lab1.L {
				colors[i], colors[j] = colors[j], colors[i]
			}
		}
	}
}

// generatePalette generates a Palette from a set of dominant colors
func generatePalette(colors []color.Color) *Palette {
	if len(colors) == 0 {
		return defaultPalette()
	}

	// Primary: brightest color
	primary := colors[0]

	// Secondary: find most contrasting color
	secondary := colors[0]
	maxContrast := 0.0
	for i := 1; i < len(colors); i++ {
		r1, g1, b1, _ := primary.RGBA()
		r2, g2, b2, _ := colors[i].RGBA()

		lab1 := srgbToLab(sRGB{R: float64(r1>>8) / 255, G: float64(g1>>8) / 255, B: float64(b1>>8) / 255})
		lab2 := srgbToLab(sRGB{R: float64(r2>>8) / 255, G: float64(g2>>8) / 255, B: float64(b2>>8) / 255})

		contrast := colorDistance(lab1, lab2)
		if contrast > maxContrast {
			maxContrast = contrast
			secondary = colors[i]
		}
	}

	// Accent: vibrant variant of primary
	accent := createAccent(primary)

	// Muted: desaturated variant for text
	muted := createMuted(primary)

	return &Palette{
		Primary:   primary,
		Secondary: secondary,
		Accent:    accent,
		Muted:     muted,
	}
}

// createAccent creates a vibrant accent color from a base color
func createAccent(base color.Color) color.Color {
	r, g, b, a := base.RGBA()
	rgb := sRGB{
		R: float64(r>>8) / 255.0,
		G: float64(g>>8) / 255.0,
		B: float64(b>>8) / 255.0,
	}
	lab := srgbToLab(rgb)

	// Increase lightness and chroma
	lab.L = math.Min(100, lab.L+10)
	lab.A = lab.A * 1.2
	lab.B = lab.B * 1.2

	accentRGB := labToSrgb(lab)
	r8 := uint8(math.Min(255, accentRGB.R*255))
	g8 := uint8(math.Min(255, accentRGB.G*255))
	b8 := uint8(math.Min(255, accentRGB.B*255))

	return color.RGBA{R: r8, G: g8, B: b8, A: uint8(a >> 8)}
}

// createMuted creates a desaturated variant of a color
func createMuted(base color.Color) color.Color {
	r, g, b, a := base.RGBA()
	rgb := sRGB{
		R: float64(r>>8) / 255.0,
		G: float64(g>>8) / 255.0,
		B: float64(b>>8) / 255.0,
	}
	lab := srgbToLab(rgb)

	// Reduce saturation by reducing A and B components
	lab.A *= 0.5
	lab.B *= 0.5

	mutedRGB := labToSrgb(lab)
	r8 := uint8(math.Min(255, mutedRGB.R*255))
	g8 := uint8(math.Min(255, mutedRGB.G*255))
	b8 := uint8(math.Min(255, mutedRGB.B*255))

	return color.RGBA{R: r8, G: g8, B: b8, A: uint8(a >> 8)}
}

// defaultPalette returns a fallback palette when no image is available
func defaultPalette() *Palette {
	return &Palette{
		Primary:   color.RGBA{R: 100, G: 150, B: 200, A: 255},
		Secondary: color.RGBA{R: 200, G: 100, B: 150, A: 255},
		Accent:    color.RGBA{R: 255, G: 200, B: 100, A: 255},
		Muted:     color.RGBA{R: 150, G: 150, B: 150, A: 255},
	}
}

// DefaultPalette returns a fallback palette when no image is available (exported version)
func DefaultPalette() *Palette {
	return defaultPalette()
}

// sRGB.toColor converts an sRGB value to color.Color
func (rgb sRGB) toColor() color.Color {
	r := uint8(math.Max(0, math.Min(255, rgb.R*255)))
	g := uint8(math.Max(0, math.Min(255, rgb.G*255)))
	b := uint8(math.Max(0, math.Min(255, rgb.B*255)))
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
