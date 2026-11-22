package wda

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
)

const (
	// WDAGitHubAPI is the GitHub API URL to get the latest release of the
	// legacy appium/WebDriverAgent repository (frozen at 16.15.3). It is only
	// used when the appium/appium-ios lookup fails.
	WDAGitHubAPI = "https://api.github.com/repos/appium/WebDriverAgent/releases/latest"
	// WDARepoURL is the archive URL template for WDA versions before
	// 16.16.0, which were released from appium/WebDriverAgent.
	WDARepoURL = "https://github.com/appium/WebDriverAgent/archive/refs/tags/v%s.zip"
)

// From 16.16.0 WebDriverAgent is developed in the appium/appium-ios monorepo
// under packages/webdriveragent and tagged appium-webdriveragent@X.Y.Z. That
// repository releases other packages too, so its "latest release" is not WDA.
const (
	monorepoTagPrefix = "appium-webdriveragent@"
	monorepoSubdir    = "packages/webdriveragent"
)

// monorepoFirstVersion is the first WDA version released from appium/appium-ios.
var monorepoFirstVersion = [3]int{16, 16, 0}

// Endpoints, variables so tests can point them at a fake server.
var (
	monorepoTagsAPI    = "https://api.github.com/repos/appium/appium-ios/git/matching-refs/tags/" + monorepoTagPrefix
	monorepoArchiveURL = "https://github.com/appium/appium-ios/archive/refs/tags/" + monorepoTagPrefix + "%s.zip"
	legacyLatestAPI    = WDAGitHubAPI
	legacyArchiveURL   = WDARepoURL
)

// Setup verifies the bundled WebDriverAgent is present and returns its path.
// Releases ship WDA inside drivers/ios/; a missing bundle indicates a broken
// install. We deliberately do NOT auto-download as a fallback — fetching
// "latest" from GitHub at runtime would deliver an untested WDA version,
// introduce a hidden network dependency, and make test runs non-reproducible.
// Use `maestro-runner wda update` to refresh WDA explicitly.
func Setup() (string, error) {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return "", err
	}

	projectPath := filepath.Join(wdaPath, "WebDriverAgent.xcodeproj")
	if _, err := os.Stat(projectPath); err != nil {
		return "", fmt.Errorf(
			"WebDriverAgent not found at %s — bundled WDA missing from install.\n"+
				"Reinstall maestro-runner, or run `maestro-runner wda update` to fetch it",
			wdaPath,
		)
	}

	return wdaPath, nil
}

// GetWDAPath returns the path where WDA is bundled in the project.
// Resolves via config.GetDriversDir so it works regardless of CWD: prefers
// $MAESTRO_RUNNER_HOME, then binary-relative (<binary-dir>/..), then CWD.
func GetWDAPath() (string, error) {
	return filepath.Join(config.GetDriversDir("ios"), "WebDriverAgent"), nil
}

// GetLocalWDAVersion reads the version from the bundled WDA's package.json.
func GetLocalWDAVersion() (string, error) {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return "", err
	}

	packagePath := filepath.Join(wdaPath, "package.json")
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read package.json: %w", err)
	}

	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", fmt.Errorf("failed to parse package.json: %w", err)
	}

	return pkg.Version, nil
}

// GetLatestWDAVersion returns the latest WDA version: the highest
// appium-webdriveragent@X.Y.Z tag in appium/appium-ios, or the legacy
// appium/WebDriverAgent latest release when that lookup fails.
func GetLatestWDAVersion() (string, error) {
	version, err := latestMonorepoVersion()
	if err == nil {
		return version, nil
	}
	legacy, legacyErr := latestLegacyVersion()
	if legacyErr != nil {
		return "", fmt.Errorf("%v; legacy repository: %w", err, legacyErr)
	}
	return legacy, nil
}

// latestMonorepoVersion lists the appium-webdriveragent@ tags of
// appium/appium-ios and returns the highest plain X.Y.Z version.
func latestMonorepoVersion() (string, error) {
	resp, err := http.Get(monorepoTagsAPI)
	if err != nil {
		return "", fmt.Errorf("failed to list appium/appium-ios tags: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d listing appium/appium-ios tags", resp.StatusCode)
	}

	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&refs); err != nil {
		return "", fmt.Errorf("failed to parse appium/appium-ios tags: %w", err)
	}

	tags := make([]string, 0, len(refs))
	for _, r := range refs {
		tags = append(tags, strings.TrimPrefix(r.Ref, "refs/tags/"))
	}
	version, ok := highestWDATag(tags)
	if !ok {
		return "", fmt.Errorf("no %sX.Y.Z tag found in appium/appium-ios", monorepoTagPrefix)
	}
	return version, nil
}

// latestLegacyVersion returns the latest release of appium/WebDriverAgent.
func latestLegacyVersion() (string, error) {
	resp, err := http.Get(legacyLatestAPI)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	// Remove 'v' prefix if present (e.g., "v11.1.3" -> "11.1.3")
	return strings.TrimPrefix(release.TagName, "v"), nil
}

// highestWDATag returns the highest version among tags of the form
// appium-webdriveragent@X.Y.Z. Other packages' tags and pre-release
// versions are ignored.
func highestWDATag(tags []string) (string, bool) {
	var best [3]int
	bestStr := ""
	for _, tag := range tags {
		v, ok := strings.CutPrefix(tag, monorepoTagPrefix)
		if !ok {
			continue
		}
		parsed, ok := parseVersion(v)
		if !ok {
			continue
		}
		if bestStr == "" || compareVersions(parsed, best) > 0 {
			best, bestStr = parsed, v
		}
	}
	return bestStr, bestStr != ""
}

// parseVersion parses a plain X.Y.Z version (an optional leading "v" is
// accepted). Anything else, pre-releases included, is rejected.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		if p == "" {
			return v, false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return v, false
			}
			n = n*10 + int(c-'0')
		}
		v[i] = n
	}
	return v, true
}

func compareVersions(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// wdaSource says where a WDA version is downloaded from: the archive URL
// and the directory inside the archive's top-level folder that holds
// WebDriverAgent.xcodeproj ("" for the archive root).
func wdaSource(version string) (url, subdir string) {
	if v, ok := parseVersion(version); ok && compareVersions(v, monorepoFirstVersion) >= 0 {
		return fmt.Sprintf(monorepoArchiveURL, strings.TrimPrefix(version, "v")), monorepoSubdir
	}
	return fmt.Sprintf(legacyArchiveURL, strings.TrimPrefix(version, "v")), ""
}

// CheckWDAUpdate compares local and latest versions, returns (localVersion, latestVersion, updateAvailable, error).
func CheckWDAUpdate() (string, string, bool, error) {
	localVersion, err := GetLocalWDAVersion()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to get local version: %w", err)
	}

	latestVersion, err := GetLatestWDAVersion()
	if err != nil {
		return localVersion, "", false, fmt.Errorf("failed to get latest version: %w", err)
	}

	updateAvailable := localVersion != latestVersion
	return localVersion, latestVersion, updateAvailable, nil
}

// DownloadWDA downloads and extracts a specific WDA version to the drivers/ios directory.
// Versions from 16.16.0 come from appium/appium-ios (packages/webdriveragent),
// earlier ones from appium/WebDriverAgent.
func DownloadWDA(version string) error {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return err
	}
	version = strings.TrimPrefix(version, "v")
	url, subdir := wdaSource(version)

	// Create temp file for download
	tmpFile, err := os.CreateTemp("", "wda-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	// Download
	fmt.Printf("Downloading WebDriverAgent v%s...\n", version)
	resp, err := http.Get(url)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: HTTP %d (%s)", resp.StatusCode, url)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	_ = tmpFile.Close()

	// Create destination directory
	baseDir := filepath.Dir(wdaPath)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Extract into a staging directory next to the destination, so the
	// installed WDA is only replaced once the new one is known to be good.
	fmt.Println("Extracting...")
	stagingDir, err := os.MkdirTemp(baseDir, ".wda-extract-")
	if err != nil {
		return fmt.Errorf("failed to create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	if err := unzip(tmpPath, stagingDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	srcDir, err := extractedWDADir(stagingDir, subdir)
	if err != nil {
		return err
	}

	// Remove old WDA directory if exists
	if _, err := os.Stat(wdaPath); err == nil {
		if err := os.RemoveAll(wdaPath); err != nil {
			return fmt.Errorf("failed to remove old WDA: %w", err)
		}
	}

	// Move the WDA project (WebDriverAgent-11.1.3 or
	// appium-ios-appium-webdriveragent-16.17.5/packages/webdriveragent) into place.
	if err := os.Rename(srcDir, wdaPath); err != nil {
		return fmt.Errorf("failed to move WDA into place: %w", err)
	}

	fmt.Printf("WebDriverAgent v%s installed successfully\n", version)
	return nil
}

// extractedWDADir returns the directory holding WebDriverAgent.xcodeproj in
// an extracted GitHub archive: subdir inside the archive's single top-level
// folder.
func extractedWDADir(stagingDir, subdir string) (string, error) {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return "", fmt.Errorf("failed to read extracted archive: %w", err)
	}
	var roots []string
	for _, e := range entries {
		if e.IsDir() {
			roots = append(roots, e.Name())
		}
	}
	if len(roots) != 1 {
		return "", fmt.Errorf("unexpected archive layout: %d top-level directories", len(roots))
	}
	dir := filepath.Join(stagingDir, roots[0], filepath.FromSlash(subdir))
	if _, err := os.Stat(filepath.Join(dir, "WebDriverAgent.xcodeproj")); err != nil {
		return "", fmt.Errorf("WebDriverAgent.xcodeproj not found in the downloaded archive (%s)",
			filepath.ToSlash(filepath.Join(roots[0], subdir)))
	}
	return dir, nil
}

// UpdateWDA downloads and installs the latest WDA version.
func UpdateWDA() error {
	latestVersion, err := GetLatestWDAVersion()
	if err != nil {
		return err
	}
	return DownloadWDA(latestVersion)
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		// Security: prevent zip slip
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		if err := extractFile(f, target); err != nil {
			return err
		}
	}

	return nil
}

func extractFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, rc)
	return err
}

// IsWDAInstalled checks if WDA is bundled in the project.
func IsWDAInstalled() bool {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return false
	}
	projectPath := filepath.Join(wdaPath, "WebDriverAgent.xcodeproj")
	_, err = os.Stat(projectPath)
	return err == nil
}
