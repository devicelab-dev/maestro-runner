package core

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // register decoders
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maestroPixelTolerance = 0.1

// ImageDifference returns the fraction of pixels that differ between two
// encoded images (PNG or JPEG). Returns 1.0 when sizes differ or decoding
// fails — that's "maximally different", which lets callers stop comparing and
// retry rather than crashing on transient screenshot encoding hiccups.
//
// Used by waitForAnimationToEnd to detect a static screen: two consecutive
// screenshots with diff ≤ threshold (0.5% upstream) → animation complete.
//
// Cost is O(width × height). For a 1080×2340 Android screenshot that's ~2.5M
// pixels per comparison; in practice screenshots arrive every ~200ms so the
// polling rate is bounded by ADB round-trip, not pixel work.
func ImageDifference(a, b []byte) float64 {
	imgA, _, err := image.Decode(bytes.NewReader(a))
	if err != nil {
		return 1.0
	}
	imgB, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return 1.0
	}

	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()
	if boundsA != boundsB {
		return 1.0
	}

	total := boundsA.Dx() * boundsA.Dy()
	if total == 0 {
		return 1.0
	}

	differing := 0
	for y := boundsA.Min.Y; y < boundsA.Max.Y; y++ {
		for x := boundsA.Min.X; x < boundsA.Max.X; x++ {
			r1, g1, b1, _ := imgA.At(x, y).RGBA()
			r2, g2, b2, _ := imgB.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				differing++
			}
		}
	}
	return float64(differing) / float64(total)
}

// CheckImageDifference is a convenience wrapper that returns an explicit error
// for the same input as ImageDifference. Useful for callers that want to
// distinguish decoding failures from genuine pixel differences.
func CheckImageDifference(a, b []byte) (float64, error) {
	imgA, _, err := image.Decode(bytes.NewReader(a))
	if err != nil {
		return 1.0, fmt.Errorf("decode first image: %w", err)
	}
	imgB, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return 1.0, fmt.Errorf("decode second image: %w", err)
	}

	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()
	if boundsA != boundsB {
		return 1.0, nil
	}

	total := boundsA.Dx() * boundsA.Dy()
	if total == 0 {
		return 1.0, nil
	}

	differing := 0
	for y := boundsA.Min.Y; y < boundsA.Max.Y; y++ {
		for x := boundsA.Min.X; x < boundsA.Max.X; x++ {
			r1, g1, b1, _ := imgA.At(x, y).RGBA()
			r2, g2, b2, _ := imgB.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				differing++
			}
		}
	}
	return float64(differing) / float64(total), nil
}

// ImageMatchStats describes an assertScreenshot comparison: the match
// percentage plus the raw pixel counts behind it. The counts matter because a
// handful of differing pixels in a multi-megapixel screenshot rounds to
// "100.00%" at two decimals, which reads like a runner bug rather than a real
// (if tiny) visual difference (#138).
type ImageMatchStats struct {
	MatchPercentage float64
	DifferingPixels int
	TotalPixels     int
}

// CompareImages returns the percentage of pixels whose RGB color distance is
// within Maestro's per-pixel tolerance, along with the pixel counts. This
// mirrors Maestro's assertScreenshot comparison rather than requiring exact
// RGB equality.
func CompareImages(expectedData, actualData []byte) (ImageMatchStats, error) {
	expected, _, err := image.Decode(bytes.NewReader(expectedData))
	if err != nil {
		return ImageMatchStats{}, fmt.Errorf("decode expected image: %w", err)
	}
	actual, _, err := image.Decode(bytes.NewReader(actualData))
	if err != nil {
		return ImageMatchStats{}, fmt.Errorf("decode actual image: %w", err)
	}

	expectedBounds := expected.Bounds()
	actualBounds := actual.Bounds()
	if expectedBounds.Dx() != actualBounds.Dx() || expectedBounds.Dy() != actualBounds.Dy() {
		return ImageMatchStats{}, fmt.Errorf(
			"screenshot size mismatch: expected %dx%d, actual %dx%d",
			expectedBounds.Dx(),
			expectedBounds.Dy(),
			actualBounds.Dx(),
			actualBounds.Dy(),
		)
	}

	total := expectedBounds.Dx() * expectedBounds.Dy()
	if total == 0 {
		return ImageMatchStats{MatchPercentage: 100}, nil
	}

	differing := 0
	for _, differs := range differingPixelMask(
		expected, actual, expectedBounds, actualBounds, expectedBounds.Dx(), expectedBounds.Dy(),
	) {
		if differs {
			differing++
		}
	}
	return ImageMatchStats{
		MatchPercentage: 100 - float64(differing)/float64(total)*100,
		DifferingPixels: differing,
		TotalPixels:     total,
	}, nil
}

// CheckImageMatchPercentage returns just the match percentage of CompareImages.
func CheckImageMatchPercentage(expectedData, actualData []byte) (float64, error) {
	stats, err := CompareImages(expectedData, actualData)
	if err != nil {
		return 0, err
	}
	return stats.MatchPercentage, nil
}

// MatchDecimals returns how many decimal places a match percentage needs so it
// does not print identically to the threshold it failed against. Two decimals
// turn 99.9998% into "100.00%", producing the nonsensical "100.00% match is
// below threshold 100.00%" (#138); widen precision until the two differ.
func MatchDecimals(match, threshold float64) int {
	const (
		minDecimals = 2
		maxDecimals = 6
	)
	for d := minDecimals; d < maxDecimals; d++ {
		if strconv.FormatFloat(match, 'f', d, 64) != strconv.FormatFloat(threshold, 'f', d, 64) {
			return d
		}
	}
	return maxDecimals
}

// DiffScreenshotPath returns the Maestro-style sidecar path for a screenshot
// comparison diff: "screen.png" → "screen_diff.png".
func DiffScreenshotPath(referencePath string) string {
	ext := filepath.Ext(referencePath)
	base := strings.TrimSuffix(referencePath, ext)
	if ext == "" {
		ext = ".png"
	}
	return base + "_diff" + ext
}

// Diff overlay knobs aligned with Maestro's ScreenshotMatch / ImageComparison defaults.
const (
	diffRectangleLineWidth = 4
	diffMinimalRectSize    = 40
	diffMergePad           = 16
)

type diffRect struct {
	minX, minY, maxX, maxY int
}

// WriteScreenshotDiff writes a PNG of the actual screenshot with transparent
// red rectangles around regions that differ from the expected image (beyond
// Maestro's per-pixel color tolerance). Mirrors Maestro's assertScreenshot
// _diff.png artifact style (rectangle overlay rather than per-pixel paint).
func WriteScreenshotDiff(expectedData, actualData []byte, diffPath string) error {
	expected, _, err := image.Decode(bytes.NewReader(expectedData))
	if err != nil {
		return fmt.Errorf("decode expected image: %w", err)
	}
	actual, _, err := image.Decode(bytes.NewReader(actualData))
	if err != nil {
		return fmt.Errorf("decode actual image: %w", err)
	}

	expectedBounds := expected.Bounds()
	actualBounds := actual.Bounds()
	width, height := expectedBounds.Dx(), expectedBounds.Dy()
	if width != actualBounds.Dx() || height != actualBounds.Dy() {
		return fmt.Errorf(
			"screenshot size mismatch: expected %dx%d, actual %dx%d",
			width,
			height,
			actualBounds.Dx(),
			actualBounds.Dy(),
		)
	}

	mask := differingPixelMask(expected, actual, expectedBounds, actualBounds, width, height)
	rects := mergeDiffRects(diffBoundingRects(mask, width, height), diffMergePad)
	for i := range rects {
		rects[i] = expandDiffRect(rects[i], width, height, diffMinimalRectSize)
	}

	diff := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(diff, diff.Bounds(), actual, actualBounds.Min, draw.Src)
	for _, r := range rects {
		drawDiffRectangle(diff, r, diffRectangleLineWidth)
	}

	if err := os.MkdirAll(filepath.Dir(diffPath), 0o755); err != nil {
		return fmt.Errorf("create diff directory: %w", err)
	}
	f, err := os.Create(diffPath)
	if err != nil {
		return fmt.Errorf("create diff file: %w", err)
	}
	defer func() { _ = f.Close() }() // safety net for early return; Close error surfaced below
	if err := png.Encode(f, diff); err != nil {
		return fmt.Errorf("encode diff PNG: %w", err)
	}
	// Surface a close error on the write path — a deferred, unchecked Close
	// can hide a final flush failure.
	if err := f.Close(); err != nil {
		return fmt.Errorf("close diff file: %w", err)
	}
	return nil
}

func differingPixelMask(
	expected, actual image.Image,
	expectedBounds, actualBounds image.Rectangle,
	width, height int,
) []bool {
	maxColorDistance := math.Sqrt(255.0 * 255.0 * 3)
	differenceLimit := math.Pow(maestroPixelTolerance*maxColorDistance, 2)
	mask := make([]bool, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			er, eg, eb, _ := expected.At(expectedBounds.Min.X+x, expectedBounds.Min.Y+y).RGBA()
			ar, ag, ab, _ := actual.At(actualBounds.Min.X+x, actualBounds.Min.Y+y).RGBA()
			dr := float64(int(ar>>8) - int(er>>8))
			dg := float64(int(ag>>8) - int(eg>>8))
			db := float64(int(ab>>8) - int(eb>>8))
			if dr*dr+dg*dg+db*db > differenceLimit {
				mask[y*width+x] = true
			}
		}
	}
	return mask
}

func diffBoundingRects(mask []bool, width, height int) []diffRect {
	visited := make([]bool, len(mask))
	var rects []diffRect
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if !mask[idx] || visited[idx] {
				continue
			}
			r := floodDiffRect(mask, visited, width, height, x, y)
			rects = append(rects, r)
		}
	}
	return rects
}

func floodDiffRect(mask, visited []bool, width, height, startX, startY int) diffRect {
	type point struct{ x, y int }
	queue := []point{{startX, startY}}
	visited[startY*width+startX] = true
	r := diffRect{minX: startX, minY: startY, maxX: startX, maxY: startY}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if p.x < r.minX {
			r.minX = p.x
		}
		if p.y < r.minY {
			r.minY = p.y
		}
		if p.x > r.maxX {
			r.maxX = p.x
		}
		if p.y > r.maxY {
			r.maxY = p.y
		}
		for _, n := range []point{
			{p.x - 1, p.y}, {p.x + 1, p.y}, {p.x, p.y - 1}, {p.x, p.y + 1},
		} {
			if n.x < 0 || n.y < 0 || n.x >= width || n.y >= height {
				continue
			}
			idx := n.y*width + n.x
			if !mask[idx] || visited[idx] {
				continue
			}
			visited[idx] = true
			queue = append(queue, n)
		}
	}
	return r
}

func mergeDiffRects(rects []diffRect, pad int) []diffRect {
	if len(rects) == 0 {
		return nil
	}
	merged := append([]diffRect(nil), rects...)
	changed := true
	for changed {
		changed = false
		next := make([]diffRect, 0, len(merged))
		used := make([]bool, len(merged))
		for i := range merged {
			if used[i] {
				continue
			}
			cur := merged[i]
			for j := i + 1; j < len(merged); j++ {
				if used[j] {
					continue
				}
				if diffRectsOverlapOrNear(cur, merged[j], pad) {
					cur = unionDiffRect(cur, merged[j])
					used[j] = true
					changed = true
				}
			}
			used[i] = true
			next = append(next, cur)
		}
		merged = next
	}
	return merged
}

func diffRectsOverlapOrNear(a, b diffRect, pad int) bool {
	return !(a.maxX+pad < b.minX || b.maxX+pad < a.minX || a.maxY+pad < b.minY || b.maxY+pad < a.minY)
}

func unionDiffRect(a, b diffRect) diffRect {
	return diffRect{
		minX: min(a.minX, b.minX),
		minY: min(a.minY, b.minY),
		maxX: max(a.maxX, b.maxX),
		maxY: max(a.maxY, b.maxY),
	}
}

func expandDiffRect(r diffRect, width, height, minSize int) diffRect {
	w := r.maxX - r.minX + 1
	h := r.maxY - r.minY + 1
	if w < minSize {
		extra := minSize - w
		r.minX -= extra / 2
		r.maxX += extra - extra/2
	}
	if h < minSize {
		extra := minSize - h
		r.minY -= extra / 2
		r.maxY += extra - extra/2
	}
	if r.minX < 0 {
		r.minX = 0
	}
	if r.minY < 0 {
		r.minY = 0
	}
	if r.maxX >= width {
		r.maxX = width - 1
	}
	if r.maxY >= height {
		r.maxY = height - 1
	}
	return r
}

func drawDiffRectangle(img *image.RGBA, r diffRect, lineWidth int) {
	if lineWidth < 1 {
		lineWidth = 1
	}
	fill := color.RGBA{R: 255, A: 48} // translucent red wash
	border := color.RGBA{R: 255, A: 255}

	for y := r.minY; y <= r.maxY; y++ {
		for x := r.minX; x <= r.maxX; x++ {
			onBorder := x < r.minX+lineWidth || x > r.maxX-lineWidth ||
				y < r.minY+lineWidth || y > r.maxY-lineWidth
			if onBorder {
				img.SetRGBA(x, y, border)
				continue
			}
			under := img.RGBAAt(x, y)
			img.SetRGBA(x, y, blendRGBA(under, fill))
		}
	}
}

func blendRGBA(dst, src color.RGBA) color.RGBA {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 {
		return src
	}
	inv := 255 - uint16(src.A)
	return color.RGBA{
		R: uint8((uint16(src.R)*uint16(src.A) + uint16(dst.R)*inv) / 255),
		G: uint8((uint16(src.G)*uint16(src.A) + uint16(dst.G)*inv) / 255),
		B: uint8((uint16(src.B)*uint16(src.A) + uint16(dst.B)*inv) / 255),
		A: 255,
	}
}
