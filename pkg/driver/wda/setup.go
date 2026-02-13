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
)

const (
	// WDAGitHubAPI is the GitHub API URL to get the latest WDA release.
	WDAGitHubAPI = "https://api.github.com/repos/appium/WebDriverAgent/releases/latest"
	// WDARepoURL is the GitHub repository URL template for downloading WDA.
	WDARepoURL = "https://github.com/appium/WebDriverAgent/archive/refs/tags/v%s.zip"
)

// Setup ensures WDA is available. Downloads if missing.
func Setup() (string, error) {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return "", err
	}

	// Check if bundled WDA exists
	projectPath := filepath.Join(wdaPath, "WebDriverAgent.xcodeproj")
	if _, err := os.Stat(projectPath); err != nil {
		// WDA not found, download the latest version
		fmt.Println("WebDriverAgent not found. Downloading...")
		if err := UpdateWDA(); err != nil {
			return "", fmt.Errorf("failed to download WebDriverAgent: %w", err)
		}
	}

	return wdaPath, nil
}

// GetWDAPath returns the path where WDA is bundled in the project.
func GetWDAPath() (string, error) {
	// Use bundled WDA in drivers/ios directory (version-agnostic path)
	return filepath.Join(".", "drivers", "ios", "WebDriverAgent"), nil
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

// GetLatestWDAVersion fetches the latest WDA version from GitHub releases.
func GetLatestWDAVersion() (string, error) {
	resp, err := http.Get(WDAGitHubAPI)
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
	version := strings.TrimPrefix(release.TagName, "v")
	return version, nil
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
func DownloadWDA(version string) error {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return err
	}

	url := fmt.Sprintf(WDARepoURL, version)

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
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	_ = tmpFile.Close()

	// Remove old WDA directory if exists
	if _, err := os.Stat(wdaPath); err == nil {
		if err := os.RemoveAll(wdaPath); err != nil {
			return fmt.Errorf("failed to remove old WDA: %w", err)
		}
	}

	// Create destination directory
	baseDir := filepath.Dir(wdaPath)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Extract zip
	fmt.Println("Extracting...")
	if err := unzip(tmpPath, baseDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	// Rename extracted folder (WebDriverAgent-11.1.3 -> WebDriverAgent)
	extractedPath := filepath.Join(baseDir, fmt.Sprintf("WebDriverAgent-%s", version))
	if err := os.Rename(extractedPath, wdaPath); err != nil {
		return fmt.Errorf("failed to rename WDA directory: %w", err)
	}

	fmt.Printf("WebDriverAgent v%s installed successfully\n", version)
	return nil
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
