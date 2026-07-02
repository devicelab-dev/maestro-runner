package core

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
)

// CropScreenshot crops a screenshot to the given element bounds.
//
// Element bounds are in device-pixel screen coordinates. The screenshot may
// be at a different resolution (some drivers downscale — e.g. the devicelab
// Android agent ships screenshots at 50% to keep WebSocket frames small),
// so bounds are scaled proportionally to the actual decoded image
// dimensions before cropping.
//
// Supports PNG and JPEG input; output format matches input. If the bounds
// fall partly off-screen they are clamped to the image rectangle.
//
// Used by takeScreenshot's cropOn selector (Maestro compatibility).
func CropScreenshot(data []byte, bounds Bounds, screenW, screenH int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty screenshot data")
	}
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return nil, fmt.Errorf("cropOn: element has zero or negative bounds")
	}
	if screenW <= 0 || screenH <= 0 {
		return nil, fmt.Errorf("cropOn: screen dimensions required for bounds scaling")
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode screenshot: %w", err)
	}

	imgRect := img.Bounds()
	imgW, imgH := imgRect.Dx(), imgRect.Dy()

	// Scale element bounds (device pixels) → image pixels.
	sx := float64(imgW) / float64(screenW)
	sy := float64(imgH) / float64(screenH)
	x := int(float64(bounds.X) * sx)
	y := int(float64(bounds.Y) * sy)
	w := int(float64(bounds.Width) * sx)
	h := int(float64(bounds.Height) * sy)

	// Clamp to image rectangle so we don't ask for pixels outside the screenshot.
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > imgW {
		w = imgW - x
	}
	if y+h > imgH {
		h = imgH - y
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("cropOn: element entirely outside screenshot bounds")
	}

	cropRect := image.Rect(imgRect.Min.X+x, imgRect.Min.Y+y, imgRect.Min.X+x+w, imgRect.Min.Y+y+h)
	subImager, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("decoded image type %T does not support SubImage", img)
	}
	cropped := subImager.SubImage(cropRect)

	var out bytes.Buffer
	switch format {
	case "jpeg":
		if err := jpeg.Encode(&out, cropped, &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("encode cropped jpeg: %w", err)
		}
	default:
		// PNG by default — preserves quality, no recompression artefacts.
		if err := png.Encode(&out, cropped); err != nil {
			return nil, fmt.Errorf("encode cropped png: %w", err)
		}
	}
	return out.Bytes(), nil
}
