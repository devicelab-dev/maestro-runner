package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/urfave/cli/v2"
)

// skillTemplate is the agent skill shipped in the binary. {{VERSION}} is
// replaced with the binary's version, so an installed copy says which release
// it describes and `skills install` can tell when it is stale.
//
//go:embed skills/SKILL.md
var skillTemplate string

const skillVersionPlaceholder = "{{VERSION}}"

// skillVersionRe reads the version stamp out of an installed SKILL.md.
var skillVersionRe = regexp.MustCompile(`<!-- maestro-runner-skill-version: (\S+) -->`)

var skillsCommand = &cli.Command{
	Name:  "skills",
	Usage: "Install the maestro-runner skill for AI coding agents",
	Subcommands: []*cli.Command{
		{
			Name:  "install",
			Usage: "Write SKILL.md, telling an agent how to write and run maestro-runner flows",
			Description: `Install the maestro-runner agent skill (SKILL.md) for Claude Code and other
agents that read skills: how to write Maestro flows, run them, inspect the
screen and read the reports.

By default it goes to ~/.claude/skills/maestro-runner/SKILL.md, for every
project. --dir puts it elsewhere, such as .claude/skills/maestro-runner in a
repository. The file carries this binary's version; an installed copy is
replaced only when its version differs, or with --force. A SKILL.md there
that maestro-runner did not write is left alone unless --force is given.

Examples:
  maestro-runner skills install
  maestro-runner skills install --dir .claude/skills/maestro-runner
  maestro-runner skills install --print > SKILL.md`,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "dir",
					Usage: "Directory to write SKILL.md into (default: ~/.claude/skills/maestro-runner)",
				},
				&cli.BoolFlag{
					Name:  "print",
					Usage: "Print the skill to stdout instead of installing it",
				},
				&cli.BoolFlag{
					Name:  "force",
					Usage: "Overwrite an installed SKILL.md even when its version matches or it was not written by maestro-runner",
				},
			},
			Action: runSkillsInstall,
		},
	},
}

func runSkillsInstall(c *cli.Context) error {
	if c.Bool("print") {
		_, err := fmt.Fprint(c.App.Writer, renderSkill(Version))
		return err
	}
	dir := c.String("dir")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("find home directory: %w (pass --dir)", err)
		}
		dir = filepath.Join(home, ".claude", "skills", "maestro-runner")
	}
	res, err := installSkill(dir, Version, c.Bool("force"))
	if err != nil {
		return err
	}
	fmt.Fprintln(c.App.Writer, res.message())
	return nil
}

// renderSkill returns the skill stamped with version.
func renderSkill(version string) string {
	return strings.ReplaceAll(skillTemplate, skillVersionPlaceholder, version)
}

// skillInstallResult says what installSkill did.
type skillInstallResult struct {
	Path       string
	Action     string // "installed", "updated", "unchanged"
	OldVersion string // version stamp of the file that was there, if any
	NewVersion string
}

func (r skillInstallResult) message() string {
	switch r.Action {
	case "unchanged":
		return fmt.Sprintf("%s is already up to date (%s); --force rewrites it", r.Path, r.OldVersion)
	case "updated":
		if r.OldVersion == "" {
			return fmt.Sprintf("Replaced %s (now %s)", r.Path, r.NewVersion)
		}
		return fmt.Sprintf("Updated %s (%s -> %s)", r.Path, r.OldVersion, r.NewVersion)
	default:
		return fmt.Sprintf("Installed %s (%s)", r.Path, r.NewVersion)
	}
}

// installSkill writes SKILL.md into dir. An existing file is replaced when its
// version stamp differs from version, or when force is set; a file without a
// stamp was not written by maestro-runner and is kept unless force is set.
func installSkill(dir, version string, force bool) (skillInstallResult, error) {
	path := filepath.Join(dir, "SKILL.md")
	res := skillInstallResult{Path: path, Action: "installed", NewVersion: version}

	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		m := skillVersionRe.FindSubmatch(existing)
		if m == nil && !force {
			return res, fmt.Errorf("%s exists and was not written by maestro-runner; pass --force to replace it", path)
		}
		res.Action = "updated"
		if m != nil {
			res.OldVersion = string(m[1])
			if res.OldVersion == version && !force {
				res.Action = "unchanged"
				return res, nil
			}
		}
	case !errors.Is(err, fs.ErrNotExist):
		return res, fmt.Errorf("read %s: %w", path, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(renderSkill(version)), 0o644); err != nil {
		return res, fmt.Errorf("write %s: %w", path, err)
	}
	return res, nil
}
