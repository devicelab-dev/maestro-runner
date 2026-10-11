package device

import (
	"strings"
	"testing"
)

// The bundled agent must say which commit it was built from, and that commit
// must have been clean: an agent built from uncommitted or old source once
// shipped in a release with nothing to tell.
func TestBundledAgentBuildIsClean(t *testing.T) {
	build, err := AgentBuildOf("../../drivers/android/devicelab-android-driver.apk")
	if err != nil {
		t.Fatalf("bundled agent: %v", err)
	}
	if build == "unknown" || strings.HasSuffix(build, "-dirty") {
		t.Errorf("bundled agent build is %q; bundle an agent built from a clean, committed checkout", build)
	}
}

func TestAgentBuildOfRejectsNonAPK(t *testing.T) {
	if _, err := AgentBuildOf("agent_build_test.go"); err == nil {
		t.Error("expected an error for a file that is not an APK")
	}
}
