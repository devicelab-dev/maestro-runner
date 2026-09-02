package cli

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestBuildWebDriverConfig_ExpandsFlowHeaderBeforeNavigate(t *testing.T) {
	const appURL = "http://127.0.0.1:3000"

	parsed, err := flow.Parse(
		[]byte("url: ${BASE_URL}\n---\n- launchApp\n"),
		"flow.yaml",
	)
	if err != nil {
		t.Fatalf("parse flow: %v", err)
	}

	cfg := &RunConfig{
		AppID: parsed.Config.EffectiveAppID(),
		Env:   map[string]string{"BASE_URL": appURL},
	}

	got := buildWebDriverConfig(cfg)
	if got.URL != appURL {
		t.Fatalf("browser navigate URL = %q, want %q", got.URL, appURL)
	}
}

func TestParseWindowSize(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		wantW int
		wantH int
	}{
		{"empty falls back", "", 0, 0},
		{"lowercase x", "390x844", 390, 844},
		{"uppercase X", "1024X768", 1024, 768},
		{"surrounding space", " 800x600 ", 800, 600},
		{"space around separator", "800 x 600", 800, 600},
		// A wrong separator is a typo, not a reason to fail the run — the
		// driver's 1280x800 default is a sane place to land.
		{"wrong separator", "390*844", 0, 0},
		{"missing height", "390x", 0, 0},
		{"non-numeric", "wide x tall", 0, 0},
		{"three parts", "1x2x3", 0, 0},
		// CDP reads a zero-width override as "no override", which would leave
		// the viewport wherever the browser opened — fall back instead.
		{"zero width", "0x500", 0, 0},
		{"negative", "-100x500", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := parseWindowSize(tt.in)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("parseWindowSize(%q) = %d, %d; want %d, %d", tt.in, w, h, tt.wantW, tt.wantH)
			}
		})
	}
}
