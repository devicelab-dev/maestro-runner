package devicelab_ios

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestKeepPreviousLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xcodebuild.log")
	for i := 1; i <= 4; i++ {
		keepPreviousLog(path, 2)
		if err := os.WriteFile(path, []byte(strconv.Itoa(i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{"": "4", ".1": "3", ".2": "2"} {
		if got, _ := os.ReadFile(path + name); string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Error(".3 kept")
	}
}
