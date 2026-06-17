// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	content := `
flows:
  - "**"
includeTags:
  - smoke
excludeTags:
  - wip
env:
  USER: test
  PASS: secret
platform: ios
device: iPhone-15
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Flows) != 1 || cfg.Flows[0] != "**" {
		t.Errorf("expected flows [**], got %v", cfg.Flows)
	}
	if len(cfg.IncludeTags) != 1 || cfg.IncludeTags[0] != "smoke" {
		t.Errorf("expected includeTags [smoke], got %v", cfg.IncludeTags)
	}
	if len(cfg.ExcludeTags) != 1 || cfg.ExcludeTags[0] != "wip" {
		t.Errorf("expected excludeTags [wip], got %v", cfg.ExcludeTags)
	}
	if cfg.Env["USER"] != "test" || cfg.Env["PASS"] != "secret" {
		t.Errorf("expected env {USER:test, PASS:secret}, got %v", cfg.Env)
	}
	if cfg.Platform != "ios" {
		t.Errorf("expected platform ios, got %s", cfg.Platform)
	}
	if cfg.Device != "iPhone-15" {
		t.Errorf("expected device iPhone-15, got %s", cfg.Device)
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	content := `flows: [invalid yaml`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	content := ``
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Flows) != 0 {
		t.Errorf("expected empty flows, got %v", cfg.Flows)
	}
}

func TestLoadFromDir_ConfigYaml(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	content := `platform: android`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Platform != "android" {
		t.Errorf("expected platform android, got %s", cfg.Platform)
	}
}

func TestLoadFromDir_ConfigYml(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yml")

	content := `platform: ios`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Platform != "ios" {
		t.Errorf("expected platform ios, got %s", cfg.Platform)
	}
}

func TestLoadFromDir_NoConfig(t *testing.T) {
	dir := t.TempDir()

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return empty config
	if cfg.Platform != "" {
		t.Errorf("expected empty platform, got %s", cfg.Platform)
	}
	if len(cfg.Flows) != 0 {
		t.Errorf("expected empty flows, got %v", cfg.Flows)
	}
}

func TestLoadFromDir_PrefersYamlOverYml(t *testing.T) {
	dir := t.TempDir()

	// Create both config.yaml and config.yml
	yamlContent := `platform: ios`
	ymlContent := `platform: android`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(ymlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should prefer config.yaml
	if cfg.Platform != "ios" {
		t.Errorf("expected platform ios (from config.yaml), got %s", cfg.Platform)
	}
}
