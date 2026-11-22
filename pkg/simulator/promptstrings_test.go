package simulator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLprojFor(t *testing.T) {
	dir := t.TempDir()
	for _, l := range []string{"de", "pt_BR", "pt_PT", "zh_CN", "zh_TW", "zh_HK", "es", "es_419", "ja"} {
		if err := os.Mkdir(filepath.Join(dir, l+".lproj"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for lang, want := range map[string]string{
		"de": "de", "de-DE": "de", "de-CH": "de", "pt-BR": "pt_BR", "pt-PT": "pt_PT",
		"zh-Hans-CN": "zh_CN", "zh-Hans": "zh_CN", "zh-Hant-HK": "zh_HK", "zh-Hant": "zh_TW",
		"es-ES": "es", "es-MX": "es_419", "ja-JP": "ja", "ko-KR": "",
	} {
		if got := lprojFor(dir, lang); got != want {
			t.Errorf("lprojFor(%q) = %q, want %q", lang, got, want)
		}
	}
}

func TestIsEnglish(t *testing.T) {
	for lang, want := range map[string]bool{"en": true, "en-GB": true, "en_AU": true, "de": false, "es-419": false} {
		if got := isEnglish(lang); got != want {
			t.Errorf("isEnglish(%q) = %v", lang, got)
		}
	}
}
