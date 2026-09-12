package devicelab_ios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestLaunchAppPermissions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		permissions map[string]string
		want        []string
		fail        bool
	}{
		{"default", nil, []string{"grant all"}, false},
		{"allow photos", map[string]string{"photos": "allow"}, []string{"grant photos"}, false},
		{"deny photos", map[string]string{"photos": "deny"}, []string{"revoke photos"}, false},
		{"reset photos", map[string]string{"photos": "unset"}, []string{"reset photos"}, false},
		{"override all", map[string]string{"all": "allow", "photos": "deny"}, []string{"grant all", "revoke photos"}, false},
		{"permission failure", map[string]string{"photos": "allow"}, []string{"grant photos"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			logPath := filepath.Join(dir, "simctl.log")
			appPath := filepath.Join(dir, "Example.app")
			if err := os.Mkdir(appPath, 0o755); err != nil {
				t.Fatal(err)
			}
			script := `#!/bin/sh
printf '%s\n' "$*" >> "$TEST_SIMCTL_LOG"
case "$2" in
  get_app_container) printf '%s\n' "$TEST_APP_PATH" ;;
  privacy)
    if [ "$TEST_PERMISSION_FAILURE" = 1 ]; then
      printf 'permission update failed\n' >&2
      exit 1
    fi
    ;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TEST_SIMCTL_LOG", logPath)
			t.Setenv("TEST_APP_PATH", appPath)
			if tc.fail {
				t.Setenv("TEST_PERMISSION_FAILURE", "1")
			}
			driver := NewDriver(nil, &core.PlatformInfo{IsSimulator: true}, "test-device", nil)
			driver.SetAppID("com.example.app")
			result := driver.handleLaunchApp(&flow.LaunchAppStep{
				ClearState: true, Permissions: tc.permissions,
			})
			if result.Success == tc.fail {
				t.Errorf("success = %v: %s", result.Success, result.Message)
			}
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			calls := string(data)
			previous := strings.Index(calls, "simctl install test-device ")
			if previous < 0 {
				t.Fatal("expected clearState to reinstall the app")
			}
			for _, action := range tc.want {
				call := "simctl privacy test-device " + action + " com.example.app\n"
				position := strings.Index(calls, call)
				if position <= previous {
					t.Errorf("expected %q after app installation and earlier permission updates; calls:\n%s", call, calls)
				}
				previous = position
			}
			launch := strings.Index(calls, "simctl launch ")
			if tc.fail && launch >= 0 {
				t.Error("app launched after a permission update failed")
			}
			if !tc.fail && launch <= previous {
				t.Error("permissions must be applied before launch")
			}
		})
	}
}
