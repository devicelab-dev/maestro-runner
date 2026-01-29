package wda

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// WDAVersion is the pinned WebDriverAgent version for stability.
	WDAVersion = "v8.0.0"
	// WDARepoURL is the GitHub repository URL template for downloading WDA.
	WDARepoURL = "https://github.com/appium/WebDriverAgent/archive/refs/tags/%s.zip"
	// WDADir is the local directory where WDA is stored.
	WDADir = ".maestro/wda"
)

// Setup ensures WDA is available (bundled in project).
func Setup() (string, error) {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return "", err
	}

	// Check if bundled WDA exists
	projectPath := filepath.Join(wdaPath, "WebDriverAgent.xcodeproj")
	if _, err := os.Stat(projectPath); err != nil {
		return "", fmt.Errorf("WebDriverAgent not found at %s\nPlease ensure drivers/ios/WebDriverAgent-%s exists",
			wdaPath, strings.TrimPrefix(WDAVersion, "v"))
	}

	return wdaPath, nil
}

// GetWDAPath returns the path where WDA is bundled in the project.
func GetWDAPath() (string, error) {
	// Use bundled WDA in drivers/ios directory
	return filepath.Join(".", "drivers", "ios", fmt.Sprintf("WebDriverAgent-%s", strings.TrimPrefix(WDAVersion, "v"))), nil
}

// GetWDABasePath returns the base WDA directory.
func GetWDABasePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, WDADir), nil
}

func downloadWDA(destPath string) error {
	url := fmt.Sprintf(WDARepoURL, WDAVersion)

	// Create temp file for download
	tmpFile, err := os.CreateTemp("", "wda-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Download
	resp, err := http.Get(url)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	tmpFile.Close()

	// Create destination directory
	baseDir := filepath.Dir(destPath)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Extract zip
	if err := unzip(tmpPath, baseDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Security: prevent zip slip
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
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
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

// IsWDAInstalled checks if WDA is already downloaded.
func IsWDAInstalled() bool {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return false
	}
	projectPath := filepath.Join(wdaPath, "WebDriverAgent.xcodeproj")
	_, err = os.Stat(projectPath)
	return err == nil
}
