package wda

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"16.17.5", [3]int{16, 17, 5}, true},
		{"v16.15.3", [3]int{16, 15, 3}, true},
		{"0.0.0", [3]int{0, 0, 0}, true},
		{"16.17", [3]int{}, false},
		{"16.17.5.1", [3]int{}, false},
		{"16.17.5-beta.1", [3]int{}, false},
		{"16..5", [3]int{}, false},
		{"", [3]int{}, false},
		{"latest", [3]int{}, false},
	}
	for _, tt := range tests {
		got, ok := parseVersion(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("parseVersion(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestHighestWDATag(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
		ok   bool
	}{
		{
			name: "numeric not lexical order",
			tags: []string{"appium-webdriveragent@16.9.0", "appium-webdriveragent@16.17.5", "appium-webdriveragent@16.16.10"},
			want: "16.17.5", ok: true,
		},
		{
			name: "other packages ignored",
			tags: []string{"@appium/coresim@99.0.0", "appium-ios-simulator@10.10.0", "appium-webdriveragent@16.16.0"},
			want: "16.16.0", ok: true,
		},
		{
			name: "pre-release ignored",
			tags: []string{"appium-webdriveragent@17.0.0-rc.1", "appium-webdriveragent@16.17.5"},
			want: "16.17.5", ok: true,
		},
		{
			name: "no wda tag",
			tags: []string{"appium-xcode@7.2.4"},
			ok:   false,
		},
		{name: "empty", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := highestWDATag(tt.tags)
			if got != tt.want || ok != tt.ok {
				t.Errorf("highestWDATag = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestWDASource(t *testing.T) {
	tests := []struct {
		version    string
		wantURL    string
		wantSubdir string
	}{
		{"16.17.5", "https://github.com/appium/appium-ios/archive/refs/tags/appium-webdriveragent@16.17.5.zip", "packages/webdriveragent"},
		{"16.16.0", "https://github.com/appium/appium-ios/archive/refs/tags/appium-webdriveragent@16.16.0.zip", "packages/webdriveragent"},
		{"v16.16.1", "https://github.com/appium/appium-ios/archive/refs/tags/appium-webdriveragent@16.16.1.zip", "packages/webdriveragent"},
		{"17.0.0", "https://github.com/appium/appium-ios/archive/refs/tags/appium-webdriveragent@17.0.0.zip", "packages/webdriveragent"},
		{"16.15.3", "https://github.com/appium/WebDriverAgent/archive/refs/tags/v16.15.3.zip", ""},
		{"11.1.3", "https://github.com/appium/WebDriverAgent/archive/refs/tags/v11.1.3.zip", ""},
		{"9.99.99", "https://github.com/appium/WebDriverAgent/archive/refs/tags/v9.99.99.zip", ""},
	}
	for _, tt := range tests {
		url, subdir := wdaSource(tt.version)
		if url != tt.wantURL || subdir != tt.wantSubdir {
			t.Errorf("wdaSource(%q) = %q, %q; want %q, %q", tt.version, url, subdir, tt.wantURL, tt.wantSubdir)
		}
	}
}

// fakeGitHub serves the two version-lookup endpoints and the archives.
type fakeGitHub struct {
	tagsStatus   int
	tags         []string
	legacyStatus int
	legacyTag    string
	archives     map[string][]byte // path -> zip
	requested    []string
}

func (f *fakeGitHub) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requested = append(f.requested, r.URL.Path)
		switch {
		case r.URL.Path == "/tags":
			if f.tagsStatus != 0 && f.tagsStatus != http.StatusOK {
				w.WriteHeader(f.tagsStatus)
				return
			}
			refs := make([]map[string]string, 0, len(f.tags))
			for _, tag := range f.tags {
				refs = append(refs, map[string]string{"ref": "refs/tags/" + tag})
			}
			_ = json.NewEncoder(w).Encode(refs)
		case r.URL.Path == "/legacy/latest":
			if f.legacyStatus != 0 && f.legacyStatus != http.StatusOK {
				w.WriteHeader(f.legacyStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": f.legacyTag})
		default:
			data, ok := f.archives[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(data)
		}
	}))
	t.Cleanup(srv.Close)

	oldTags, oldArchive, oldLegacy, oldLegacyArchive := monorepoTagsAPI, monorepoArchiveURL, legacyLatestAPI, legacyArchiveURL
	monorepoTagsAPI = srv.URL + "/tags"
	monorepoArchiveURL = srv.URL + "/mono/%s.zip"
	legacyLatestAPI = srv.URL + "/legacy/latest"
	legacyArchiveURL = srv.URL + "/legacy/v%s.zip"
	t.Cleanup(func() {
		monorepoTagsAPI, monorepoArchiveURL, legacyLatestAPI, legacyArchiveURL = oldTags, oldArchive, oldLegacy, oldLegacyArchive
	})
	return srv
}

func TestGetLatestWDAVersion(t *testing.T) {
	tests := []struct {
		name    string
		fake    fakeGitHub
		want    string
		wantErr bool
	}{
		{
			name: "monorepo tags win",
			fake: fakeGitHub{
				tags:      []string{"appium-webdriveragent@16.16.0", "appium-webdriveragent@16.17.5", "appium-webdriveragent@16.17.4"},
				legacyTag: "v16.15.3",
			},
			want: "16.17.5",
		},
		{
			name: "monorepo lookup fails, legacy used",
			fake: fakeGitHub{tagsStatus: http.StatusForbidden, legacyTag: "v16.15.3"},
			want: "16.15.3",
		},
		{
			name: "monorepo has no wda tag, legacy used",
			fake: fakeGitHub{tags: []string{}, legacyTag: "v16.15.3"},
			want: "16.15.3",
		},
		{
			name:    "both fail",
			fake:    fakeGitHub{tagsStatus: http.StatusInternalServerError, legacyStatus: http.StatusInternalServerError},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fake
			f.start(t)
			got, err := GetLatestWDAVersion()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("version = %q, want %q", got, tt.want)
			}
		})
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func useTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MAESTRO_RUNNER_HOME", home)
	config.ResetHome()
	t.Cleanup(config.ResetHome)
	return home
}

func TestDownloadWDA(t *testing.T) {
	monoZip := makeZip(t, map[string]string{
		"appium-ios-appium-webdriveragent-16.17.5/package.json":                                                     `{"version":"0.0.0-monorepo-root"}`,
		"appium-ios-appium-webdriveragent-16.17.5/packages/coresim/package.json":                                    `{"version":"1.14.0"}`,
		"appium-ios-appium-webdriveragent-16.17.5/packages/webdriveragent/package.json":                             `{"version":"16.17.5"}`,
		"appium-ios-appium-webdriveragent-16.17.5/packages/webdriveragent/WebDriverAgent.xcodeproj/project.pbxproj": "pbx",
	})
	legacyZip := makeZip(t, map[string]string{
		"WebDriverAgent-16.15.3/package.json":                             `{"version":"16.15.3"}`,
		"WebDriverAgent-16.15.3/WebDriverAgent.xcodeproj/project.pbxproj": "pbx",
	})
	noProjectZip := makeZip(t, map[string]string{
		"appium-ios-appium-webdriveragent-16.17.4/packages/webdriveragent/package.json": `{"version":"16.17.4"}`,
	})

	tests := []struct {
		name        string
		version     string
		wantPath    string
		wantVersion string
		wantErr     string
	}{
		{name: "monorepo subtree", version: "16.17.5", wantPath: "/mono/16.17.5.zip", wantVersion: "16.17.5"},
		{name: "legacy repo", version: "16.15.3", wantPath: "/legacy/v16.15.3.zip", wantVersion: "16.15.3"},
		{name: "missing tag", version: "16.99.0", wantPath: "/mono/16.99.0.zip", wantErr: "HTTP 404"},
		{name: "archive without project", version: "16.17.4", wantPath: "/mono/16.17.4.zip", wantErr: "WebDriverAgent.xcodeproj not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := useTempHome(t)
			f := &fakeGitHub{archives: map[string][]byte{
				"/mono/16.17.5.zip":    monoZip,
				"/mono/16.17.4.zip":    noProjectZip,
				"/legacy/v16.15.3.zip": legacyZip,
			}}
			f.start(t)

			// An installed WDA that must survive a failed update.
			wdaPath := filepath.Join(home, "drivers", "ios", "WebDriverAgent")
			if err := os.MkdirAll(filepath.Join(wdaPath, "WebDriverAgent.xcodeproj"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(wdaPath, "package.json"), []byte(`{"version":"16.12.8"}`), 0o644); err != nil {
				t.Fatal(err)
			}

			err := DownloadWDA(tt.version)
			if len(f.requested) != 1 || f.requested[0] != tt.wantPath {
				t.Errorf("requested %v, want [%s]", f.requested, tt.wantPath)
			}

			gotVersion, verr := GetLocalWDAVersion()
			if verr != nil {
				t.Fatalf("GetLocalWDAVersion: %v", verr)
			}

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				if gotVersion != "16.12.8" {
					t.Errorf("installed WDA changed to %q on a failed update", gotVersion)
				}
			} else {
				if err != nil {
					t.Fatalf("DownloadWDA: %v", err)
				}
				if gotVersion != tt.wantVersion {
					t.Errorf("installed version = %q, want %q", gotVersion, tt.wantVersion)
				}
				if !IsWDAInstalled() {
					t.Error("IsWDAInstalled = false after download")
				}
			}

			// Nothing but the WDA directory is left in drivers/ios.
			entries, _ := os.ReadDir(filepath.Join(home, "drivers", "ios"))
			if len(entries) != 1 || entries[0].Name() != "WebDriverAgent" {
				names := []string{}
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Errorf("drivers/ios contains %v, want [WebDriverAgent]", names)
			}
		})
	}
}
