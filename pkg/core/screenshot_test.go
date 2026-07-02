package core

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodeGradientPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func decodePNGSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode cropped png: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestCropScreenshot_HappyPath(t *testing.T) {
	src := encodeGradientPNG(t, 1000, 2000)
	bounds := Bounds{X: 100, Y: 200, Width: 300, Height: 400}
	out, err := CropScreenshot(src, bounds, 1000, 2000)
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	w, h := decodePNGSize(t, out)
	if w != 300 || h != 400 {
		t.Errorf("expected cropped 300x400, got %dx%d", w, h)
	}
}

// Devicelab Android agent downscales screenshots to 50% before transmit.
// Bounds (in device-pixel space) must scale proportionally to image space.
func TestCropScreenshot_HalfResScreenshot(t *testing.T) {
	src := encodeGradientPNG(t, 500, 1000)                                // screenshot at 50% of device pixels
	bounds := Bounds{X: 200, Y: 400, Width: 400, Height: 600}     // device-pixel bounds
	out, err := CropScreenshot(src, bounds, 1000, 2000)           // screen is 1000x2000 device pixels
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	// Bounds scaled by image/screen ratio (0.5) → expect 200x300 in image pixels.
	w, h := decodePNGSize(t, out)
	if w != 200 || h != 300 {
		t.Errorf("expected cropped 200x300 (scaled), got %dx%d", w, h)
	}
}

func TestCropScreenshot_ClampsToImage(t *testing.T) {
	src := encodeGradientPNG(t, 800, 600)
	bounds := Bounds{X: 700, Y: 500, Width: 200, Height: 200} // extends past image edge
	out, err := CropScreenshot(src, bounds, 800, 600)
	if err != nil {
		t.Fatalf("crop: %v", err)
	}
	w, h := decodePNGSize(t, out)
	if w != 100 || h != 100 {
		t.Errorf("expected clamped 100x100, got %dx%d", w, h)
	}
}

func TestCropScreenshot_RejectsZeroBounds(t *testing.T) {
	src := encodeGradientPNG(t, 100, 100)
	if _, err := CropScreenshot(src, Bounds{X: 0, Y: 0, Width: 0, Height: 50}, 100, 100); err == nil {
		t.Error("expected error for zero-width bounds")
	}
	if _, err := CropScreenshot(src, Bounds{X: 0, Y: 0, Width: 50, Height: 0}, 100, 100); err == nil {
		t.Error("expected error for zero-height bounds")
	}
}

func TestCropScreenshot_RejectsEmptyData(t *testing.T) {
	if _, err := CropScreenshot(nil, Bounds{X: 0, Y: 0, Width: 10, Height: 10}, 100, 100); err == nil {
		t.Error("expected error for nil data")
	}
}

func TestCropScreenshot_RejectsZeroScreenDims(t *testing.T) {
	src := encodeGradientPNG(t, 100, 100)
	if _, err := CropScreenshot(src, Bounds{X: 0, Y: 0, Width: 10, Height: 10}, 0, 100); err == nil {
		t.Error("expected error for zero screen width")
	}
}

func TestCropScreenshot_FullyOffScreenBoundsErrors(t *testing.T) {
	src := encodeGradientPNG(t, 100, 100)
	// Bounds entirely past the right edge.
	if _, err := CropScreenshot(src, Bounds{X: 200, Y: 50, Width: 50, Height: 20}, 100, 100); err == nil {
		t.Error("expected error for fully off-screen bounds")
	}
}
