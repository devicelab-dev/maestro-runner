package executor

import (
	"bytes"
	"image"
	imgcolor "image/color"
	"image/png"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// solidPNG renders a 20x20 image of one colour, so two frames either match
// exactly or differ in every pixel.
func solidPNG(t *testing.T, gray uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for x := 0; x < 20; x++ {
		for y := 0; y < 20; y++ {
			img.Set(x, y, imgcolor.RGBA{gray, gray, gray, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

// settleFake replays a scripted sequence of frames, repeating the last one.
type settleFake struct {
	core.Driver
	frames [][]byte
	calls  int
}

func (f *settleFake) Execute(flow.Step) *core.CommandResult {
	i := f.calls
	if i >= len(f.frames) {
		i = len(f.frames) - 1
	}
	f.calls++
	return &core.CommandResult{Success: true, Data: f.frames[i]}
}

func TestCaptureSettledScreenshot_WaitsForTwoMatchingFrames(t *testing.T) {
	moving, settled := solidPNG(t, 0), solidPNG(t, 255)
	fake := &settleFake{frames: [][]byte{moving, solidPNG(t, 128), settled, settled}}
	fr := &FlowRunner{driver: fake}

	result := fr.captureSettledScreenshot(&flow.AssertScreenshotStep{})

	if !result.Success {
		t.Fatalf("expected success, got %v", result.Error)
	}
	if got := result.Data.([]byte); !bytes.Equal(got, settled) {
		t.Error("should return the settled frame, not one captured mid-animation")
	}
	if fake.calls != 4 {
		t.Errorf("captured %d frames, want 4 (initial + until two agree)", fake.calls)
	}
}

func TestCaptureSettledScreenshot_ReturnsImmediatelyWhenAlreadyStill(t *testing.T) {
	still := solidPNG(t, 200)
	fake := &settleFake{frames: [][]byte{still, still}}
	fr := &FlowRunner{driver: fake}

	if !fr.captureSettledScreenshot(&flow.AssertScreenshotStep{}).Success {
		t.Fatal("expected success")
	}
	if fake.calls != 2 {
		t.Errorf("captured %d frames, want 2 — a still screen costs one extra capture", fake.calls)
	}
}

// flickerFake alternates between two frames forever, so it can never settle.
type flickerFake struct {
	core.Driver
	a, b  []byte
	calls int
}

func (f *flickerFake) Execute(flow.Step) *core.CommandResult {
	f.calls++
	if f.calls%2 == 1 {
		return &core.CommandResult{Success: true, Data: f.a}
	}
	return &core.CommandResult{Success: true, Data: f.b}
}

func TestCaptureSettledScreenshot_GivesUpRatherThanHanging(t *testing.T) {
	// A screen that never settles (spinner, video, blinking caret) must still
	// yield a frame; the assertion's own threshold then decides pass or fail.
	prev := screenshotSettleTimeout
	screenshotSettleTimeout = 80 * time.Millisecond
	defer func() { screenshotSettleTimeout = prev }()

	fake := &flickerFake{a: solidPNG(t, 0), b: solidPNG(t, 255)}
	start := time.Now()
	result := (&FlowRunner{driver: fake}).captureSettledScreenshot(&flow.AssertScreenshotStep{})
	elapsed := time.Since(start)

	if !result.Success {
		t.Fatal("giving up must still return a usable frame")
	}
	if result.Data == nil {
		t.Error("gave up without returning any frame")
	}
	if elapsed > time.Second {
		t.Errorf("took %v — should bail at the timeout, not hang", elapsed)
	}
	if fake.calls < 2 {
		t.Errorf("only %d captures — the loop never ran", fake.calls)
	}
}

func TestCaptureSettledScreenshot_HandsBackAnEmptyCapture(t *testing.T) {
	// An empty first frame is the caller's error to report, so the loop must
	// pass it through untouched rather than spinning on it.
	empty := &settleFake{frames: [][]byte{nil}}
	result := (&FlowRunner{driver: empty}).captureSettledScreenshot(&flow.AssertScreenshotStep{})

	if got, _ := result.Data.([]byte); len(got) != 0 {
		t.Errorf("expected the empty capture handed back, got %d bytes", len(got))
	}
	if empty.calls != 1 {
		t.Errorf("made %d captures, want 1 — an empty frame must not be retried", empty.calls)
	}
}
