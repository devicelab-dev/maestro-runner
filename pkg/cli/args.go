package cli

import (
	"strings"

	"github.com/urfave/cli/v2"
)

// normalizeArgs lets options follow the flow paths, as Maestro accepts
// (`maestro test flow.yaml --format junit`). The flag parser stops at the
// first flow path, so the options after it were read as more flow paths.
// It returns args with, after the command, every option ahead of the flow
// paths, and global options given after `test` moved ahead of `test`.
// Everything after a bare "--" is left as it is.
func normalizeArgs(args []string, global, test []cli.Flag, commands []*cli.Command) []string {
	if len(args) < 2 {
		return args
	}
	globalNames := flagNames(global)
	testNames := flagNames(test)

	// Find the command: the first argument that is not an option.
	i := 1
	for i < len(args) {
		a := args[i]
		if a == "--" || !isOption(a) {
			break
		}
		i += optionLen(args, i, globalNames, testNames)
	}
	if i >= len(args) || args[i] == "--" {
		return args
	}

	cmd := args[i]
	start := i + 1 // where the command's own arguments begin
	isTest := cmd == "test"
	if !isTest {
		for _, c := range commands {
			if c.Name == cmd || contains(c.Aliases, cmd) {
				return args // another command: leave its arguments alone
			}
		}
		// No command: the root runs flows, and cmd is the first flow path.
		start = i
	}

	var hoisted, options, paths, rest []string
	for j := start; j < len(args); {
		a := args[j]
		if a == "--" {
			rest = args[j:]
			break
		}
		if !isOption(a) {
			paths = append(paths, a)
			j++
			continue
		}
		n := optionLen(args, j, globalNames, testNames)
		name := optionName(a)
		if _, ok := testNames[name]; !ok && isTest {
			if _, ok := globalNames[name]; ok {
				hoisted = append(hoisted, args[j:j+n]...)
				j += n
				continue
			}
		}
		options = append(options, args[j:j+n]...)
		j += n
	}

	out := make([]string, 0, len(args))
	out = append(out, args[:i]...)
	out = append(out, hoisted...)
	if isTest {
		out = append(out, cmd)
	}
	out = append(out, options...)
	out = append(out, paths...)
	return append(out, rest...)
}

// flagNames maps every name and alias of flags to whether it takes a value.
func flagNames(flags []cli.Flag) map[string]bool {
	m := map[string]bool{}
	for _, f := range flags {
		_, isBool := f.(*cli.BoolFlag)
		for _, n := range f.Names() {
			m[n] = !isBool
		}
	}
	return m
}

func isOption(a string) bool {
	return len(a) > 1 && strings.HasPrefix(a, "-")
}

func optionName(a string) string {
	n := strings.TrimLeft(a, "-")
	if k := strings.IndexByte(n, '='); k >= 0 {
		n = n[:k]
	}
	return n
}

// optionLen is how many arguments the option at args[i] spans: 2 when its
// value is the next argument, else 1.
func optionLen(args []string, i int, maps ...map[string]bool) int {
	a := args[i]
	if strings.Contains(a, "=") {
		return 1
	}
	name := optionName(a)
	for _, m := range maps {
		if takesValue, ok := m[name]; ok {
			if takesValue && i+1 < len(args) {
				return 2
			}
			return 1
		}
	}
	return 1
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
