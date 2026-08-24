package cli

import (
	"bufio"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
)

var doctorCommand = &cli.Command{
	Name:  "doctor",
	Usage: "Check this machine for everything a run needs",
	Description: `Check the toolchain, the SDKs and the devices, and say what to do about
whatever is missing.

Every check reports ok, warn or error. A warn is a platform you cannot drive
right now — no Xcode means no iOS, which is fine if you only test Android — so
only a real error (a broken install, a team ID Xcode does not have) exits
non-zero. That way doctor drops into CI as a pre-flight gate without failing a
Linux runner for having no Xcode.

Pass --team-id to verify it against the Apple signing identities on this Mac,
which is the check that explains 'No Account for Team' at WDA build time.

Examples:
  maestro-runner doctor
  maestro-runner doctor --json
  maestro-runner --team-id A3RCAA2YAX doctor`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "json",
			Usage: "Emit JSON instead of a report",
		},
	},
	Action: runDoctor,
}

// Doctor status values.
const (
	StatusOK    = "ok"
	StatusWarn  = "warn"
	StatusError = "error"
)

// DoctorCheck is one line of the report. The JSON tags are a public contract.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | error
	Detail string `json:"detail,omitempty"`
	Remedy string `json:"remedy,omitempty"` // what to actually do about it
}

func runDoctor(c *cli.Context) error {
	checks := RunDoctorChecks(globalString(c, "team-id"))

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(checks); err != nil {
			return err
		}
	} else {
		fmt.Print(formatDoctorReport(checks))
	}

	for _, check := range checks {
		if check.Status == StatusError {
			return cli.Exit("", 1)
		}
	}
	return nil
}

// RunDoctorChecks probes the host. teamID is optional; when empty the signing
// check is skipped rather than guessed at.
func RunDoctorChecks(teamID string) []DoctorCheck {
	var checks []DoctorCheck

	checks = append(checks, checkCommand("adb", "adb (Android SDK)", "Android runs need adb. Install Android platform-tools and put them on PATH."))
	checks = append(checks, checkAndroidHome())
	checks = append(checks, checkAVDImages()...)

	if runtime.GOOS == "darwin" {
		checks = append(checks, checkCommand("xcrun", "xcrun (Xcode command line tools)", "iOS runs need the Xcode command line tools. Install them with `xcode-select --install`."))
		checks = append(checks, checkFullXcode())
		if teamID != "" {
			checks = append(checks, checkTeamID(teamID))
		}
	}

	checks = append(checks, checkVisibleDevices())
	return checks
}

// checkCommand reports whether a tool is on PATH. Missing tools are a warn, not
// an error: a machine that only tests Android has no business failing because
// it has no xcrun.
func checkCommand(bin, name, remedy string) DoctorCheck {
	path, err := exec.LookPath(bin)
	if err != nil {
		return DoctorCheck{Name: name, Status: StatusWarn, Detail: "not found on PATH", Remedy: remedy}
	}
	return DoctorCheck{Name: name, Status: StatusOK, Detail: path}
}

func checkAndroidHome() DoctorCheck {
	const name = "ANDROID_HOME"
	home := androidSDKRoot()
	if home == "" {
		return DoctorCheck{
			Name:   name,
			Status: StatusWarn,
			Detail: "not set",
			Remedy: "Set ANDROID_HOME (or ANDROID_SDK_ROOT) to your Android SDK directory.",
		}
	}
	if _, err := os.Stat(home); err != nil {
		// A set-but-wrong path is worse than an unset one: everything
		// downstream fails with a confusing error instead of a clear one.
		return DoctorCheck{
			Name:   name,
			Status: StatusError,
			Detail: fmt.Sprintf("%s does not exist", home),
			Remedy: "Point ANDROID_HOME at a real Android SDK directory, or unset it.",
		}
	}
	return DoctorCheck{Name: name, Status: StatusOK, Detail: home}
}

func androidSDKRoot() string {
	for _, key := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// checkAVDImages catches the trap where an AVD names a system image that is not
// installed. Nothing reports this at run time: the emulator simply never
// appears, and the only symptom is a driver polling for a device that will
// never boot.
func checkAVDImages() []DoctorCheck {
	sdk := androidSDKRoot()
	avdHome := filepath.Join(os.Getenv("HOME"), ".android", "avd")
	entries, err := os.ReadDir(avdHome)
	if err != nil || sdk == "" {
		return nil // no AVDs, or no SDK to check them against — other checks cover that
	}

	var checks []DoctorCheck
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".avd") || !entry.IsDir() {
			continue
		}
		avd := strings.TrimSuffix(entry.Name(), ".avd")
		sysdir := readAVDSysdir(filepath.Join(avdHome, entry.Name(), "config.ini"))
		if sysdir == "" {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(sdk, sysdir)); statErr != nil {
			checks = append(checks, DoctorCheck{
				Name:   fmt.Sprintf("AVD %q system image", avd),
				Status: StatusError,
				Detail: fmt.Sprintf("config.ini points at %s, which is not installed under %s", sysdir, sdk),
				Remedy: fmt.Sprintf("Install it with `sdkmanager --sdk_root=%s \"%s\"`, or recreate the AVD.", sdk, sdkmanagerPackage(sysdir)),
			})
		}
	}
	return checks
}

// readAVDSysdir returns the image.sysdir.1 value from an AVD config.ini,
// normalised to be relative to the SDK root.
func readAVDSysdir(configPath string) string {
	f, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		value, ok := strings.CutPrefix(line, "image.sysdir.1=")
		if !ok {
			continue
		}
		return normalizeSysdir(strings.TrimSpace(value))
	}
	return ""
}

// normalizeSysdir strips the stray leading "android/" that avdmanager has been
// seen to write, which makes an otherwise-valid path resolve nowhere.
func normalizeSysdir(sysdir string) string {
	sysdir = strings.TrimSuffix(sysdir, "/")
	return strings.TrimPrefix(sysdir, "android/")
}

// sdkmanagerPackage turns "system-images/android-34/google_apis/arm64-v8a" into
// the "system-images;android-34;google_apis;arm64-v8a" spelling sdkmanager wants.
func sdkmanagerPackage(sysdir string) string {
	return strings.ReplaceAll(strings.Trim(sysdir, "/"), "/", ";")
}

// checkFullXcode distinguishes the Command Line Tools from a full Xcode.
// XCUITest needs the latter, and the failure when it is missing surfaces far
// from the cause.
func checkFullXcode() DoctorCheck {
	const name = "Xcode (full install, for iOS)"
	out, err := commandOutput("xcode-select", "-p")
	devDir := strings.TrimSpace(out)
	if err != nil || devDir == "" {
		return DoctorCheck{
			Name:   name,
			Status: StatusWarn,
			Detail: "xcode-select has no developer directory set",
			Remedy: "sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer",
		}
	}
	if strings.Contains(devDir, "CommandLineTools") {
		return DoctorCheck{
			Name:   name,
			Status: StatusWarn,
			Detail: fmt.Sprintf("Command Line Tools only (%s) — iOS needs full Xcode", devDir),
			Remedy: "sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer",
		}
	}

	verOut, verErr := commandOutput("xcodebuild", "-version")
	if verErr != nil && strings.Contains(strings.ToLower(verOut), "license") {
		return DoctorCheck{
			Name:   name,
			Status: StatusWarn,
			Detail: "the Xcode licence has not been accepted",
			Remedy: "sudo xcodebuild -license accept",
		}
	}
	if match := regexp.MustCompile(`Xcode\s+([\d.]+)`).FindStringSubmatch(verOut); match != nil {
		return DoctorCheck{Name: name, Status: StatusOK, Detail: fmt.Sprintf("Xcode %s (%s)", match[1], devDir)}
	}
	return DoctorCheck{
		Name:   name,
		Status: StatusWarn,
		Detail: fmt.Sprintf("xcodebuild is not usable from %s", devDir),
		Remedy: "Check the Xcode installation, then re-run doctor.",
	}
}

// checkTeamID answers the question behind `error: No Account for Team "..."` at
// WDA build time: Xcode has no signing identity belonging to that team. The
// team ID lives in the OU field of every Apple signing certificate, so the
// login keychain is the authority — no Xcode-internal state is parsed.
func checkTeamID(teamID string) DoctorCheck {
	name := fmt.Sprintf("Signing identity for team %s", teamID)
	teams := signingTeamIDs()
	if len(teams) == 0 {
		return DoctorCheck{
			Name:   name,
			Status: StatusError,
			Detail: "no Apple signing certificates found in the login keychain",
			Remedy: "Sign in to Xcode (Settings → Accounts) so it can create a development certificate. A free Apple ID works: it gets a Personal Team.",
		}
	}
	for _, t := range teams {
		if strings.EqualFold(t, teamID) {
			return DoctorCheck{Name: name, Status: StatusOK, Detail: "a matching signing certificate is installed"}
		}
	}
	return DoctorCheck{
		Name:   name,
		Status: StatusError,
		Detail: fmt.Sprintf("no certificate belongs to team %s; this Mac has: %s", teamID, strings.Join(teams, ", ")),
		Remedy: "Pass one of the team IDs listed above as --team-id, or add that team's account in Xcode (Settings → Accounts).",
	}
}

// signingTeamIDs returns the distinct team IDs of the Apple signing
// certificates in the login keychain, read from each certificate's OU.
func signingTeamIDs() []string {
	seen := map[string]bool{}
	var teams []string
	for _, certName := range []string{"Apple Development", "Apple Distribution", "iPhone Developer", "iPhone Distribution"} {
		out, err := commandOutput("security", "find-certificate", "-a", "-c", certName, "-p")
		if err != nil || out == "" {
			continue
		}
		for _, team := range teamIDsFromPEM(out) {
			if !seen[team] {
				seen[team] = true
				teams = append(teams, team)
			}
		}
	}
	sort.Strings(teams)
	return teams
}

// teamIDsFromPEM extracts the OU of every certificate in a PEM bundle. Expired
// certificates are skipped — they are exactly as useless to a build as an
// absent one, and listing them would send someone chasing a team they cannot
// sign with.
func teamIDsFromPEM(pemData string) []string {
	var teams []string
	rest := []byte(pemData)
	now := time.Now()
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return teams
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || now.After(cert.NotAfter) {
			continue
		}
		teams = append(teams, cert.Subject.OrganizationalUnit...)
	}
}

// checkVisibleDevices reuses the same discovery as `devices`, so doctor and the
// listing can never disagree about what is attached.
func checkVisibleDevices() DoctorCheck {
	entries := collectDevices("", false)
	var ready int
	for _, e := range entries {
		if e.Ready {
			ready++
		}
	}
	switch {
	case ready > 0:
		return DoctorCheck{Name: "Devices", Status: StatusOK, Detail: fmt.Sprintf("%d ready to run", ready)}
	case len(entries) > 0:
		return DoctorCheck{
			Name:   "Devices",
			Status: StatusWarn,
			Detail: fmt.Sprintf("%d found, none ready", len(entries)),
			Remedy: "Run `maestro-runner devices` to see why.",
		}
	default:
		return DoctorCheck{
			Name:   "Devices",
			Status: StatusWarn,
			Detail: "none attached",
			Remedy: "Connect a device, boot a simulator or start an emulator; `maestro-runner devices` lists what is visible.",
		}
	}
}

// commandOutput runs a command and returns its combined output. Combined,
// because the diagnostics worth reading — the Xcode licence prompt in
// particular — arrive on stderr.
func commandOutput(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// formatDoctorReport renders the checks. Kept free of I/O so it can be tested.
func formatDoctorReport(checks []DoctorCheck) string {
	var b strings.Builder
	var warns, errs int

	for _, check := range checks {
		marker := "✓"
		switch check.Status {
		case StatusWarn:
			marker, warns = "!", warns+1
		case StatusError:
			marker, errs = "✗", errs+1
		}
		fmt.Fprintf(&b, "%s %s", marker, check.Name)
		if check.Detail != "" {
			fmt.Fprintf(&b, " — %s", check.Detail)
		}
		b.WriteString("\n")
		if check.Remedy != "" && check.Status != StatusOK {
			fmt.Fprintf(&b, "    %s\n", check.Remedy)
		}
	}

	fmt.Fprintf(&b, "\n%d checked, %d warning(s), %d error(s)\n", len(checks), warns, errs)
	if errs == 0 && warns == 0 {
		b.WriteString("Everything a run needs is in place.\n")
	}
	return b.String()
}
