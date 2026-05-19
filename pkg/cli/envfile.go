package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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
