package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestRenderSkillStampsVersion(t *testing.T) {
	out := renderSkill("9.9.9")
	if strings.Contains(out, skillVersionPlaceholder) {
		t.Fatal("placeholder left in rendered skill")
	}
	m := skillVersionRe.FindStringSubmatch(out)
	if m == nil || m[1] != "9.9.9" {
		t.Fatalf("version stamp = %v", m)
	}
	if !strings.HasPrefix(out, "---\nname: maestro-runner\ndescription: ") {
		t.Errorf("skill must open with name/description front matter:\n%s", out[:80])
	}
}

// The skill names commands and flags; keep it honest as they change.
func TestSkillMentionsRealCommandsAndFlags(t *testing.T) {
	commands := map[string]bool{}
	for _, c := range []*cli.Command{testCommand, hierarchyCommand, lintCommand, devicesCommand, doctorCommand, screenshotCommand} {
		commands[c.Name] = true
	}
	for _, name := range []string{"test", "hierarchy", "lint", "devices", "doctor", "screenshot"} {
		if !commands[name] || !strings.Contains(skillTemplate, "maestro-runner "+name) &&
			!strings.Contains(skillTemplate, " "+name+" ") {
			t.Errorf("skill or CLI lacks command %q", name)
		}
	}
	flags := map[string]bool{}
	for _, f := range append(append([]cli.Flag{}, GlobalFlags...), testCommand.Flags...) {
		for _, n := range f.Names() {
			flags[n] = true
		}
	}
	for _, n := range []string{"platform", "device", "driver", "app-file", "team-id", "appium-url", "caps",
		"env", "include-tags", "parallel", "retries", "retry-failed", "output", "flatten", "headed", "wait-for-idle-timeout"} {
		if !flags[n] {
			t.Errorf("skill mentions --%s, which is not a flag", n)
		}
		if !strings.Contains(skillTemplate, "--"+n) {
			t.Errorf("skill no longer mentions --%s; drop it from this list if intended", n)
		}
	}
}

func TestInstallSkillFreshThenUnchangedThenUpdated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills", "maestro-runner")
	path := filepath.Join(dir, "SKILL.md")

	res, err := installSkill(dir, "1.0.0", false)
	if err != nil || res.Action != "installed" {
		t.Fatalf("fresh install: %+v %v", res, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "maestro-runner-skill-version: 1.0.0") {
		t.Fatal("installed file lacks the version stamp")
	}

	// Same version: left alone, even if edited.
	if err := os.WriteFile(path, append(data, []byte("\nlocal edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = installSkill(dir, "1.0.0", false)
	if err != nil || res.Action != "unchanged" {
		t.Fatalf("same version: %+v %v", res, err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "local edit") {
		t.Error("same-version install rewrote the file")
	}

	// --force rewrites it.
	res, err = installSkill(dir, "1.0.0", true)
	if err != nil || res.Action != "updated" {
		t.Fatalf("forced: %+v %v", res, err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "local edit") {
		t.Error("forced install kept the old file")
	}

	// New version: replaced.
	res, err = installSkill(dir, "1.1.0", false)
	if err != nil || res.Action != "updated" || res.OldVersion != "1.0.0" {
		t.Fatalf("new version: %+v %v", res, err)
	}
	if !strings.Contains(res.message(), "1.0.0 -> 1.1.0") {
		t.Errorf("message = %q", res.message())
	}
}

func TestInstallSkillKeepsForeignFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("my own skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installSkill(dir, "1.0.0", false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected a refusal mentioning --force, got %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "my own skill\n" {
		t.Error("foreign file was overwritten")
	}
	res, err := installSkill(dir, "1.0.0", true)
	if err != nil || !strings.Contains(res.message(), "Replaced") {
		t.Fatalf("forced over foreign file: %+v %v", res, err)
	}
}

func runSkillsCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	install := skillsCommand.Subcommands[0]
	set := flag.NewFlagSet("install", flag.ContinueOnError)
	for _, f := range install.Flags {
		if err := f.Apply(set); err != nil {
			t.Fatal(err)
		}
	}
	if err := set.Parse(args); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	app := &cli.App{Writer: &out}
	err := runSkillsInstall(cli.NewContext(app, set, nil))
	return out.String(), err
}

func TestSkillsInstallCommand(t *testing.T) {
	out, err := runSkillsCLI(t, "--print")
	if err != nil || !strings.Contains(out, "maestro-runner-skill-version: "+Version) {
		t.Fatalf("--print: %v\n%s", err, out)
	}

	dir := t.TempDir()
	out, err = runSkillsCLI(t, "--dir", dir)
	if err != nil || !strings.Contains(out, "Installed "+filepath.Join(dir, "SKILL.md")) {
		t.Fatalf("--dir: %v %q", err, out)
	}
	out, err = runSkillsCLI(t, "--dir", dir)
	if err != nil || !strings.Contains(out, "already up to date") {
		t.Fatalf("second install: %v %q", err, out)
	}
}

func TestSkillsInstallDefaultDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := runSkillsCLI(t); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "maestro-runner", "SKILL.md")); err != nil {
		t.Fatalf("default location not written: %v", err)
	}
}
