package cli

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/executor"
)

// ParseEnvFile reads a `.env`-style file and returns the key/value pairs.
//
// Format (matches the de-facto dotenv convention):
//   - Blank lines and lines starting with `#` are skipped.
//   - Each non-comment line is `KEY=VALUE` (whitespace around `=` allowed; key
//     and value are trimmed).
//   - Values may be wrapped in single or double quotes (`KEY='v'` / `KEY="v"`);
//     content inside quotes is taken verbatim (including `#`, spaces).
//   - Unquoted values are terminated by ` #` (space + hash) — inline comment.
//     Bare `#` mid-value is preserved (so `URL=https://x#a` keeps the fragment).
//   - Keys must match [A-Za-z_][A-Za-z0-9_]* (matches POSIX env var names).
//
// Returns a non-nil map even on error so callers can show partial progress.
func ParseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path) //#nosec G304 -- user-provided env file path
	if err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	defer func() { _ = f.Close() }()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return out, fmt.Errorf("env file %s line %d: missing '=' (got %q)", path, lineNum, line)
		}

		key := strings.TrimSpace(line[:eq])
		if !isValidEnvKey(key) {
			return out, fmt.Errorf("env file %s line %d: invalid key %q", path, lineNum, key)
		}

		val := strings.TrimSpace(line[eq+1:])
		val = unquoteEnvValue(val)
		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("read env file %s: %w", path, err)
	}
	return out, nil
}

// unquoteEnvValue handles the three value forms:
//   - `"..."` / `'...'` — strip the matching quotes, take content verbatim.
//   - Unquoted with ` #` — split on the first ` #` and trim.
//   - Otherwise — return as-is (trimmed by caller).
func unquoteEnvValue(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return v[1 : len(v)-1]
		}
	}
	// Inline comment: ` #` (space + hash) anywhere terminates the value.
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// isValidEnvKey returns true for POSIX-shaped env var names: [A-Za-z_][A-Za-z0-9_]*.
func isValidEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, c := range k {
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			continue
		}
		if i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// expandRunnerVars resolves `${VAR}` in a value using the same engine and the
// same precedence the executor applies to steps: system environment first, then
// the merged runner env (workspace config, then --env-file, then -e).
//
// Values that reach a driver before any flow executes have to be expanded here,
// because nothing downstream will do it for them.
func expandRunnerVars(value string, env map[string]string) string {
	if value == "" || !strings.Contains(value, "${") {
		return value
	}
	script := executor.NewScriptEngine()
	defer script.Close()
	script.ImportSystemEnv()
	script.SetVariables(env)
	return script.ExpandVariables(value)
}

// varRefPattern matches a `${NAME}` reference.
var varRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// unresolvedVars returns the `${VAR}` names in value that have no value in env
// or the process environment, in first-appearance order.
//
// Expansion silently yields an empty string for an unset variable, which turns a
// misconfigured `url: ${BASE_URL}` into a confusing "no URL specified" much
// later in the run. Naming the variable up front is the whole point.
func unresolvedVars(value string, env map[string]string) []string {
	var missing []string
	seen := map[string]bool{}
	for _, m := range varRefPattern.FindAllStringSubmatch(value, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := env[name]; ok {
			continue
		}
		if _, ok := os.LookupEnv(name); ok {
			continue
		}
		missing = append(missing, name)
	}
	return missing
}
