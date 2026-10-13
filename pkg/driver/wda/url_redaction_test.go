package wda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

func TestOpenURLRedaction(t *testing.T) {
	const link = "worldidstg://verify?key=fake-secret-key#fake-secret-fragment"
	for _, tc := range []struct {
		name                       string
		status                     int
		malformed, browser, failed bool
	}{
		{name: "success", status: 200},
		{name: "device error", status: 500, failed: true},
		{name: "device error with HTTP 200", status: 200, failed: true},
		{name: "malformed response", status: 500, malformed: true, failed: true},
		{name: "browser error", status: 500, browser: true, failed: true},
		{name: "browser success", status: 200, browser: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			logPath := filepath.Join(dir, "runner.log")
			if err := logger.Init(logPath); err != nil {
				t.Fatal(err)
			}
			defer logger.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["url"] != link {
					t.Errorf("device did not receive original URL: %q", body["url"])
				}
				w.WriteHeader(tc.status)
				if tc.malformed {
					fmt.Fprint(w, "invalid response echoes "+link)
					return
				}
				if tc.failed {
					jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "unknown error", "message": "cannot open " + link}})
					return
				}
				jsonResponse(w, map[string]interface{}{"value": nil})
			}))
			defer server.Close()
			driver := createTestDriver(server)
			step := &flow.OpenLinkStep{BaseStep: flow.BaseStep{StepType: flow.StepOpenLink}, Link: link}
			var result *core.CommandResult
			if tc.browser {
				result = driver.openBrowser(&flow.OpenBrowserStep{URL: link})
			} else {
				result = driver.openLink(step)
			}
			if result.Success != !tc.failed {
				t.Fatalf("unexpected result: %+v", result)
			}
			if !strings.Contains(result.Message, "worldidstg://verify") {
				t.Error("missing destination")
			}
			details := ""
			if result.Error != nil {
				details = result.Error.Error()
				if !strings.Contains(details, fmt.Sprintf("HTTP %d", tc.status)) {
					t.Error("missing HTTP status")
				}
			}
			index, flows, err := report.BuildSkeleton([]flow.Flow{{SourcePath: "connector.yaml", Steps: []flow.Step{step}}}, report.BuilderConfig{})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Success {
				flows[0].Commands[0].Error = &report.Error{Message: result.Message + ": " + details}
			}
			if err := report.WriteSkeleton(dir, index, flows); err != nil {
				t.Fatal(err)
			}
			// Check the actual runner log and JSON artifacts, including the failure message.
			if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for _, secret := range []string{"fake-secret-key", "fake-secret-fragment"} {
					if strings.Contains(string(data), secret) {
						t.Errorf("secret in artifact %s", path)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			for _, s := range []string{result.Message, details} {
				if strings.Contains(s, "fake-secret") {
					t.Error("secret in command result")
				}
			}
		})
	}
}

func TestOpenURLSimulatorFailureRedaction(t *testing.T) {
	dir := t.TempDir()
	// A failed simctl invocation can include the full URL in stderr.
	script := "#!/bin/sh\nprintf '%s\\n' \"$4\" >&2\nexit 7\n"
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	driver := &Driver{info: &core.PlatformInfo{IsSimulator: true}, udid: "fake-device"}
	result := driver.openLink(&flow.OpenLinkStep{Link: "worldidstg://verify?key=fake-secret-key"})
	if result.Success || result.Error == nil {
		t.Fatal("expected simulator failure")
	}
	if !strings.Contains(result.Error.Error(), "exit status 7") {
		t.Errorf("missing exit status: %v", result.Error)
	}
	if strings.Contains(result.Message+result.Error.Error(), "fake-secret-key") {
		t.Fatal("simctl output exposed connector key")
	}
}

func TestOpenURLSimulatorCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	driver := &Driver{info: &core.PlatformInfo{IsSimulator: true}, udid: "fake-device"}
	driver.SetContext(ctx)
	err := driver.openURL("worldidstg://verify?key=fake-secret-key")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
