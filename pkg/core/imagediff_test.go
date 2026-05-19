package core

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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
