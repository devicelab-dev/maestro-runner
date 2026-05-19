package core

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders
	_ "image/png"
)

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
