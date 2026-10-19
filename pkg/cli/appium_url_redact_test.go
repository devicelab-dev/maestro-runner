package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// The Appium URL of a cloud hub carries the access key; neither the console
// nor the run log may show it.
func TestCreateAppiumDriverRedactsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	const key = "s3cr3t-access-key"
	appiumURL := strings.Replace(server.URL, "http://", "http://alice:"+key+"@", 1) + "/wd/hub"

	logPath := filepath.Join(t.TempDir(), "run.log")
	if err := logger.Init(logPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logger.Close)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	_, _, createErr := createAppiumDriver(&RunConfig{AppiumURL: appiumURL})
	_ = w.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(r)
	if createErr == nil {
		t.Fatal("expected the session to fail against a server answering 500")
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"console": string(out), "log": string(logged)} {
		if strings.Contains(text, key) {
			t.Errorf("%s shows the access key:\n%s", name, text)
		}
		if !strings.Contains(text, strings.TrimPrefix(server.URL, "http://")+"/wd/hub") {
			t.Errorf("%s should still name the server:\n%s", name, text)
		}
	}
	if strings.Contains(createErr.Error(), key) {
		t.Errorf("error shows the access key: %v", createErr)
	}
}
