package core

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func encodePNG(t *testing.T, w, h int, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestImageDifference_Identical(t *testing.T) {
	a := encodePNG(t, 100, 100, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	b := encodePNG(t, 100, 100, color.RGBA{R: 200, G: 100, B: 50, A: 255})

	if diff := ImageDifference(a, b); diff != 0 {
		t.Errorf("identical images: diff = %f, want 0", diff)
	}
}

func TestImageDifference_Disjoint(t *testing.T) {
	a := encodePNG(t, 100, 100, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	b := encodePNG(t, 100, 100, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	if diff := ImageDifference(a, b); diff != 1.0 {
		t.Errorf("fully-different images: diff = %f, want 1.0", diff)
	}
}

func TestImageDifference_MixedPixels(t *testing.T) {
	// 100×100 image. Half white, half black. Compare against fully-white.
	imgA := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if y < 50 {
				imgA.Set(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				imgA.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	var bufA bytes.Buffer
	if err := png.Encode(&bufA, imgA); err != nil {
		t.Fatalf("encode A: %v", err)
	}
	b := encodePNG(t, 100, 100, color.RGBA{255, 255, 255, 255})

	diff := ImageDifference(bufA.Bytes(), b)
	if diff < 0.49 || diff > 0.51 {
		t.Errorf("half-different images: diff = %f, want ~0.5", diff)
	}
}

func TestImageDifference_SizeMismatch(t *testing.T) {
	a := encodePNG(t, 100, 100, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	b := encodePNG(t, 200, 200, color.RGBA{R: 0, G: 0, B: 0, A: 255})

	if diff := ImageDifference(a, b); diff != 1.0 {
		t.Errorf("size mismatch: diff = %f, want 1.0", diff)
	}
}

func TestImageDifference_DecodeFailure(t *testing.T) {
	// Non-image bytes — should return 1.0 (not crash).
	if diff := ImageDifference([]byte("not an image"), []byte("not an image either")); diff != 1.0 {
		t.Errorf("decode failure: diff = %f, want 1.0", diff)
	}
}

func TestCheckImageDifference_ReturnsErrorOnDecodeFail(t *testing.T) {
	_, err := CheckImageDifference([]byte("garbage"), []byte("garbage"))
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestCheckImageMatchPercentage_UsesMaestroColorTolerance(t *testing.T) {
	expected := encodePNG(t, 10, 10, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	withinTolerance := encodePNG(t, 10, 10, color.RGBA{R: 120, G: 100, B: 100, A: 255})

	match, err := CheckImageMatchPercentage(expected, withinTolerance)
	if err != nil {
		t.Fatalf("CheckImageMatchPercentage() error = %v", err)
	}
	if match != 100 {
		t.Errorf("match = %f, want 100", match)
	}
}

func TestCheckImageMatchPercentage_CountsPixelsOutsideTolerance(t *testing.T) {
	expected := encodePNG(t, 10, 10, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	outsideTolerance := encodePNG(t, 10, 10, color.RGBA{R: 200, G: 100, B: 100, A: 255})

	match, err := CheckImageMatchPercentage(expected, outsideTolerance)
	if err != nil {
		t.Fatalf("CheckImageMatchPercentage() error = %v", err)
	}
	if match != 0 {
		t.Errorf("match = %f, want 0", match)
	}
}

func TestCheckImageMatchPercentage_ReportsSizeMismatch(t *testing.T) {
	expected := encodePNG(t, 10, 10, color.Black)
	actual := encodePNG(t, 20, 10, color.Black)

	_, err := CheckImageMatchPercentage(expected, actual)
	if err == nil {
		t.Fatal("expected size mismatch error")
	}
}

func TestDiffScreenshotPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"screen.png", "screen_diff.png"},
		{"/tmp/a/b.png", "/tmp/a/b_diff.png"},
		{"screen", "screen_diff.png"},
		{"screen.JPEG", "screen_diff.JPEG"},
	}
	for _, tt := range tests {
		if got := DiffScreenshotPath(tt.in); got != tt.want {
			t.Errorf("DiffScreenshotPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWriteScreenshotDiff_DrawsRectangleOverlay(t *testing.T) {
	// 80×80 gray expected. Actual has a solid block of differing pixels in the center.
	expected := encodePNG(t, 80, 80, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	actualImg := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			if x >= 30 && x < 50 && y >= 30 && y < 50 {
				actualImg.Set(x, y, color.RGBA{R: 200, G: 100, B: 100, A: 255})
			} else {
				actualImg.Set(x, y, color.RGBA{R: 100, G: 100, B: 100, A: 255})
			}
		}
	}
	var actualBuf bytes.Buffer
	if err := png.Encode(&actualBuf, actualImg); err != nil {
		t.Fatalf("encode actual: %v", err)
	}

	diffPath := filepath.Join(t.TempDir(), "screen_diff.png")
	if err := WriteScreenshotDiff(expected, actualBuf.Bytes(), diffPath); err != nil {
		t.Fatalf("WriteScreenshotDiff() error = %v", err)
	}

	data, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatalf("read diff: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode diff: %v", err)
	}

	// Corner of the image should remain unchanged (outside any overlay).
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 100 || g>>8 != 100 || b>>8 != 100 {
		t.Errorf("unaffected corner = rgb(%d,%d,%d), want gray", r>>8, g>>8, b>>8)
	}

	// Border of the expanded/merged rectangle should be solid red.
	foundBorder := false
	for y := 20; y < 60 && !foundBorder; y++ {
		for x := 20; x < 60; x++ {
			rr, gg, bb, _ := img.At(x, y).RGBA()
			if rr>>8 == 255 && gg>>8 == 0 && bb>>8 == 0 {
				foundBorder = true
				break
			}
		}
	}
	if !foundBorder {
		t.Error("expected a red rectangle border around the differing region")
	}
}
