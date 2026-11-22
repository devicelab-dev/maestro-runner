package simulator

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// promptStringTables are the string tables that word iOS's system permission
// prompts: notifications, privacy access (camera, photos, contacts, App
// Tracking Transparency, ...) and location. Paths are inside the runtime root.
var promptStringTables = []struct{ bundle, table string }{
	{"System/Library/PrivateFrameworks/UserNotificationsServer.framework", "Localizable"},
	{"System/Library/PrivateFrameworks/TCC.framework", "Localizable"},
	{"System/Library/Frameworks/CoreLocation.framework", "locationd"},
}

// PromptStringPairs returns, for a simulator, every permission-prompt string
// in English paired with the same key's value in each of the simulator's
// preferred languages (AppleLanguages) that is not English. The strings are
// read on the host from the simulator's runtime, so they are iOS's own
// translations. A simulator in English, or one whose language or runtime
// cannot be read, has no pairs.
func PromptStringPairs(udid string) ([][2]string, error) {
	dev, err := findSimForPrompts(udid)
	if err != nil {
		return nil, err
	}
	languages := preferredLanguages(dev.DataPath)
	if len(languages) == 0 {
		return nil, nil
	}
	root, err := runtimeRoot(dev.runtime)
	if err != nil {
		return nil, err
	}
	var pairs [][2]string
	for _, t := range promptStringTables {
		dir := filepath.Join(root, t.bundle)
		english, err := readStringsTable(filepath.Join(dir, "en.lproj", t.table+".strings"))
		if err != nil {
			return nil, err
		}
		done := map[string]bool{}
		for _, lang := range languages {
			loc := lprojFor(dir, lang)
			if loc == "" || done[loc] {
				continue
			}
			done[loc] = true
			localized, err := readStringsTable(filepath.Join(dir, loc+".lproj", t.table+".strings"))
			if err != nil {
				return nil, err
			}
			for key, en := range english {
				if v, ok := localized[key]; ok {
					pairs = append(pairs, [2]string{en, v})
				}
			}
		}
	}
	return pairs, nil
}

type simctlDev struct {
	UDID     string `json:"udid"`
	DataPath string `json:"dataPath"`
	runtime  string
}

// findSimForPrompts finds a simulator and its runtime identifier.
func findSimForPrompts(udid string) (*simctlDev, error) {
	out, err := exec.Command("xcrun", "simctl", "list", "devices", "-j").Output()
	if err != nil {
		return nil, fmt.Errorf("simctl list devices: %w", err)
	}
	var list struct {
		Devices map[string][]simctlDev `json:"devices"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("simctl list devices: %w", err)
	}
	for runtime, devs := range list.Devices {
		for _, d := range devs {
			if d.UDID == udid {
				d.runtime = runtime
				return &d, nil
			}
		}
	}
	return nil, fmt.Errorf("simulator %s not found", udid)
}

// runtimeRoot is the root file system of a simulator runtime on the host.
func runtimeRoot(identifier string) (string, error) {
	out, err := exec.Command("xcrun", "simctl", "list", "runtimes", "-j").Output()
	if err != nil {
		return "", fmt.Errorf("simctl list runtimes: %w", err)
	}
	var list struct {
		Runtimes []struct {
			Identifier  string `json:"identifier"`
			RuntimeRoot string `json:"runtimeRoot"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return "", fmt.Errorf("simctl list runtimes: %w", err)
	}
	for _, r := range list.Runtimes {
		if r.Identifier == identifier && r.RuntimeRoot != "" {
			return r.RuntimeRoot, nil
		}
	}
	return "", fmt.Errorf("runtime %s not found", identifier)
}

// preferredLanguages is the simulator's AppleLanguages without the English
// ones, which the English rule already covers. Nil when unset (a new
// simulator is in English).
func preferredLanguages(dataPath string) []string {
	if dataPath == "" {
		return nil
	}
	file := filepath.Join(dataPath, "Library", "Preferences", ".GlobalPreferences.plist")
	if _, err := os.Stat(file); err != nil {
		return nil
	}
	out, err := exec.Command("plutil", "-extract", "AppleLanguages", "json", "-o", "-", file).Output()
	if err != nil {
		return nil
	}
	var langs []string
	if json.Unmarshal(out, &langs) != nil {
		return nil
	}
	var keep []string
	for _, l := range langs {
		if l != "" && !isEnglish(l) {
			keep = append(keep, l)
		}
	}
	return keep
}

func isEnglish(lang string) bool {
	return lang == "en" || strings.HasPrefix(lang, "en-") || strings.HasPrefix(lang, "en_")
}

// lprojFor picks the localization of a bundle for a language tag such as
// "de", "de-DE", "pt-BR" or "zh-Hans-CN": the full tag, then language and
// region, then the Chinese script's usual folder, then the language alone.
// "" when the bundle has none of them.
func lprojFor(bundleDir, lang string) string {
	parts := strings.Split(strings.ReplaceAll(lang, "_", "-"), "-")
	var candidates []string
	candidates = append(candidates, strings.Join(parts, "_"))
	if len(parts) >= 2 {
		region := parts[len(parts)-1]
		candidates = append(candidates, parts[0]+"_"+region)
	}
	if parts[0] == "zh" && len(parts) >= 2 {
		switch parts[1] {
		case "Hans":
			candidates = append(candidates, "zh_CN")
		case "Hant":
			candidates = append(candidates, "zh_TW")
		}
	}
	if parts[0] == "es" && len(parts) >= 2 && parts[len(parts)-1] != "ES" {
		candidates = append(candidates, "es_419")
	}
	candidates = append(candidates, parts[0])
	for _, c := range candidates {
		if fi, err := os.Stat(filepath.Join(bundleDir, c+".lproj")); err == nil && fi.IsDir() {
			return c
		}
	}
	return ""
}

// readStringsTable reads a compiled .strings file (a property list) into its
// string values.
func readStringsTable(path string) (map[string]string, error) {
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", path).Output()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	table := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			table[k] = s
		}
	}
	return table, nil
}
