package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeEnvFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func TestParseEnvFile_AllFeatures(t *testing.T) {
	contents := `# Top-of-file comment
APP_ID=com.example.app # inline comment
TEST_MESSAGE=Hello from environment file!
TEST_URL=https://api.example.com
TEST_QUOTE_1="Quote 1"
TEST_QUOTE_2='Quote 2'
COLOR="#FFFFFF"
HASH_IN_VALUE=https://example.com/page#section
EMPTY=
SPACES_AROUND  =  value with spaces
QUOTED_HASH_SPACE="value # with space"

# Trailing comment
`
	path := writeEnvFile(t, contents)

	got, err := ParseEnvFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		"APP_ID":            "com.example.app",
		"TEST_MESSAGE":      "Hello from environment file!",
		"TEST_URL":          "https://api.example.com",
		"TEST_QUOTE_1":      "Quote 1",
		"TEST_QUOTE_2":      "Quote 2",
		"COLOR":             "#FFFFFF",
		"HASH_IN_VALUE":     "https://example.com/page#section",
		"EMPTY":             "",
		"SPACES_AROUND":     "value with spaces",
		"QUOTED_HASH_SPACE": "value # with space",
	}

	if !reflect.DeepEqual(got, want) {
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
		for k := range got {
			if _, ok := want[k]; !ok {
				t.Errorf("unexpected key %q = %q", k, got[k])
			}
		}
	}
}

func TestParseEnvFile_MissingFile(t *testing.T) {
	_, err := ParseEnvFile("/nonexistent/.env")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseEnvFile_MissingEquals(t *testing.T) {
	path := writeEnvFile(t, "VALID=ok\nBROKEN no equals here\n")
	_, err := ParseEnvFile(path)
	if err == nil {
		t.Error("expected error for line missing '='")
	}
}

func TestParseEnvFile_InvalidKey(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{"starts with digit", "1KEY=value"},
		{"contains dash", "MY-KEY=value"},
		{"contains dot", "MY.KEY=value"},
		{"empty key", "=value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeEnvFile(t, tc.contents)
			_, err := ParseEnvFile(path)
			if err == nil {
				t.Errorf("expected error for %q", tc.contents)
			}
		})
	}
}

func TestIsValidEnvKey(t *testing.T) {
	valid := []string{"A", "_", "_FOO", "FOO", "foo_BAR", "K1", "K_1_2"}
	invalid := []string{"", "1KEY", "K-Y", "K.Y", "K Y", "K\tY", "KEY!", "FOO BAR"}
	for _, k := range valid {
		if !isValidEnvKey(k) {
			t.Errorf("isValidEnvKey(%q) = false, want true", k)
		}
	}
	for _, k := range invalid {
		if isValidEnvKey(k) {
			t.Errorf("isValidEnvKey(%q) = true, want false", k)
		}
	}
}

func TestUnquoteEnvValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"hello"`, `hello`},
		{`'hello'`, `hello`},
		{`"with spaces and # inside"`, `with spaces and # inside`},
		{`unquoted`, `unquoted`},
		{`unquoted # inline`, `unquoted`},
		{`"unbalanced`, `"unbalanced`},
		{`unbalanced"`, `unbalanced"`},
		{`https://x#section`, `https://x#section`}, // no space-hash → keep fragment
		{`https://x #frag`, `https://x`},           // space-hash → strip
	}
	for _, c := range cases {
		if got := unquoteEnvValue(c.in); got != c.want {
			t.Errorf("unquoteEnvValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExpandRunnerVars(t *testing.T) {
	t.Setenv("SYS_ONLY", "from-system")

	for name, tc := range map[string]struct {
		value string
		env   map[string]string
		want  string
	}{
		"runner env wins over system": {"${SYS_ONLY}", map[string]string{"SYS_ONLY": "from-runner"}, "from-runner"},
		"falls back to system":        {"${SYS_ONLY}", nil, "from-system"},
		"no template is untouched":    {"com.example.app", nil, "com.example.app"},
		"empty stays empty":           {"", nil, ""},
		"embedded in a url":           {"http://${HOST}:3000", map[string]string{"HOST": "127.0.0.1"}, "http://127.0.0.1:3000"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := expandRunnerVars(tc.value, tc.env); got != tc.want {
				t.Errorf("expandRunnerVars(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestUnresolvedVars(t *testing.T) {
	t.Setenv("PRESENT_IN_SYSTEM", "x")

	for name, tc := range map[string]struct {
		value string
		env   map[string]string
		want  []string
	}{
		"unset is reported":    {"${NOPE}", nil, []string{"NOPE"}},
		"runner env satisfies": {"${OK}", map[string]string{"OK": "v"}, nil},
		"system env satisfies": {"${PRESENT_IN_SYSTEM}", nil, nil},
		"reported once each":   {"${A}/${A}/${B}", nil, []string{"A", "B"}},
		"no template, nothing": {"com.example.app", nil, nil},
		"empty value is fine":  {"", nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			got := unresolvedVars(tc.value, tc.env)
			if len(got) != len(tc.want) {
				t.Fatalf("unresolvedVars(%q) = %v, want %v", tc.value, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("unresolvedVars(%q) = %v, want %v", tc.value, got, tc.want)
				}
			}
		})
	}
}
