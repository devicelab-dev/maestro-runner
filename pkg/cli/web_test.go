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
