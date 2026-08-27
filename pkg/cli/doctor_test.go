package cli

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatDoctorReportCountsAndAnnotates(t *testing.T) {
	checks := []DoctorCheck{
		{Name: "adb", Status: StatusOK, Detail: "/usr/bin/adb"},
		{Name: "Xcode", Status: StatusWarn, Detail: "Command Line Tools only", Remedy: "sudo xcode-select --switch ..."},
		{Name: "Team", Status: StatusError, Detail: "no certificate", Remedy: "Add the account in Xcode."},
	}

	out := formatDoctorReport(checks)

	if !strings.Contains(out, "3 checked, 1 warning(s), 1 error(s)") {
		t.Errorf("summary line wrong:\n%s", out)
	}
	if !strings.Contains(out, "sudo xcode-select") || !strings.Contains(out, "Add the account in Xcode.") {
		t.Errorf("remedies should be printed for warn and error:\n%s", out)
	}
	if strings.Contains(out, "Everything a run needs") {
		t.Errorf("the all-clear must not appear alongside failures:\n%s", out)
	}
}

func TestFormatDoctorReportHidesRemedyForPassingChecks(t *testing.T) {
	out := formatDoctorReport([]DoctorCheck{
		{Name: "adb", Status: StatusOK, Detail: "/usr/bin/adb", Remedy: "should not be shown"},
	})
	if strings.Contains(out, "should not be shown") {
		t.Errorf("a passing check should not print a remedy:\n%s", out)
	}
	if !strings.Contains(out, "Everything a run needs") {
		t.Errorf("an all-ok report should say so:\n%s", out)
	}
}

func TestNormalizeSysdirStripsStrayAndroidPrefix(t *testing.T) {
	// avdmanager has been seen to write this prefix, which makes an otherwise
	// valid path resolve nowhere under the SDK root.
	tests := map[string]string{
		"android/system-images/android-34/google_apis/arm64-v8a": "system-images/android-34/google_apis/arm64-v8a",
		"system-images/android-34/google_apis/arm64-v8a/":        "system-images/android-34/google_apis/arm64-v8a",
		"system-images/android-33/default/x86_64":                "system-images/android-33/default/x86_64",
	}
	for in, want := range tests {
		if got := normalizeSysdir(in); got != want {
			t.Errorf("normalizeSysdir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSdkmanagerPackageUsesSemicolons(t *testing.T) {
	got := sdkmanagerPackage("system-images/android-34/google_apis/arm64-v8a")
	want := "system-images;android-34;google_apis;arm64-v8a"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadAVDSysdir(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.ini")
	contents := "avd.ini.encoding=UTF-8\nimage.sysdir.1=android/system-images/android-34/google_apis/arm64-v8a/\nhw.ramSize=4096\n"
	if err := os.WriteFile(config, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := readAVDSysdir(config); got != "system-images/android-34/google_apis/arm64-v8a" {
		t.Errorf("got %q, want the normalised sysdir", got)
	}
	if got := readAVDSysdir(filepath.Join(dir, "missing.ini")); got != "" {
		t.Errorf("a missing config should yield %q, got %q", "", got)
	}
}

// certPEM builds a self-signed certificate carrying teamID in its OU, which is
// where Apple puts the team on a signing certificate.
func certPEM(t *testing.T, teamID string, notAfter time.Time) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:         "Apple Development: Tester",
			OrganizationalUnit: []string{teamID},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestTeamIDsFromPEMReadsEveryCertificate(t *testing.T) {
	bundle := certPEM(t, "AAAA111111", time.Now().Add(24*time.Hour)) +
		certPEM(t, "BBBB222222", time.Now().Add(24*time.Hour))

	got := teamIDsFromPEM(bundle)

	if len(got) != 2 || got[0] != "AAAA111111" || got[1] != "BBBB222222" {
		t.Errorf("got %v, want both team IDs", got)
	}
}

func TestTeamIDsFromPEMSkipsExpiredCertificates(t *testing.T) {
	// An expired certificate is exactly as useless to a build as an absent
	// one; listing it would send someone chasing a team they cannot sign with.
	bundle := certPEM(t, "EXPIRED000", time.Now().Add(-time.Hour)) +
		certPEM(t, "VALID11111", time.Now().Add(24*time.Hour))

	got := teamIDsFromPEM(bundle)

	if len(got) != 1 || got[0] != "VALID11111" {
		t.Errorf("got %v, want only the unexpired team", got)
	}
}

func TestTeamIDsFromPEMIgnoresGarbage(t *testing.T) {
	if got := teamIDsFromPEM("not a certificate at all"); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestCheckVisibleDevicesNeverErrors(t *testing.T) {
	// doctor must stay usable on a machine with nothing attached — that is the
	// normal state for a first run, and the whole point of the command.
	if got := checkVisibleDevices(); got.Status == StatusError {
		t.Errorf("no devices should be a warning at worst, got %+v", got)
	}
}
