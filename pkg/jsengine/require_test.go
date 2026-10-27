package jsengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequire_RelativeModuleFromBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "helpers.js"),
		[]byte(`module.exports = { double: function(n){ return n * 2; }, name: "helpers" };`), 0o644); err != nil {
		t.Fatal(err)
	}

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(dir)

	v, err := e.Eval(`require("./helpers.js").double(21)`)
	if err != nil {
		t.Fatalf("require eval failed: %v", err)
	}
	if n, _ := v.(int64); n != 42 {
		t.Errorf("double(21) = %v, want 42", v)
	}

	v2, err := e.Eval(`require("./helpers.js").name`)
	if err != nil {
		t.Fatalf("require eval failed: %v", err)
	}
	if v2 != "helpers" {
		t.Errorf("name = %v, want helpers", v2)
	}
}

func TestRequire_NestedRequire(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.js"), []byte(`module.exports = { v: require("./b.js").v + 1 };`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.js"), []byte(`module.exports = { v: 10 };`), 0o644)

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(dir)

	v, err := e.Eval(`require("./a.js").v`)
	if err != nil {
		t.Fatalf("nested require failed: %v", err)
	}
	if n, _ := v.(int64); n != 11 {
		t.Errorf("a.v = %v, want 11", v)
	}
}

func TestRequire_MissingModuleIsAnError(t *testing.T) {
	e := New()
	defer e.Close()
	e.SetRequireBaseDir(t.TempDir())

	if _, err := e.Eval(`require("./does-not-exist.js")`); err == nil {
		t.Error("expected an error requiring a missing module")
	}
}

func writeModule(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// evalIn evaluates expr the way a script file in scriptDir runs ("" = inline).
func evalIn(e *Engine, scriptDir, expr string) (interface{}, error) {
	var v interface{}
	err := e.WithRequireScriptDir(scriptDir, func() error {
		var err error
		v, err = e.Eval(expr)
		return err
	})
	return v, err
}

func TestRequire_ScriptDirFirst(t *testing.T) {
	root := t.TempDir()
	flowDir := filepath.Join(root, "flow")
	scriptDir := filepath.Join(root, "scripts")
	writeModule(t, filepath.Join(flowDir, "lib.js"), `module.exports = "flow";`)
	writeModule(t, filepath.Join(scriptDir, "lib.js"), `module.exports = "script";`)

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(flowDir)

	v, err := evalIn(e, scriptDir, `require("./lib.js")`)
	if err != nil {
		t.Fatalf("require failed: %v", err)
	}
	if v != "script" {
		t.Errorf("got %v, want the module beside the script", v)
	}
}

func TestRequire_ScriptDirFallsBackToFlowDir(t *testing.T) {
	root := t.TempDir()
	flowDir := filepath.Join(root, "flow")
	scriptDir := filepath.Join(root, "scripts")
	writeModule(t, filepath.Join(flowDir, "lib.js"), `module.exports = "flow";`)
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatal(err)
	}

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(flowDir)

	// With and without the extension, both must find the flow's module.
	for _, p := range []string{"./lib.js", "./lib"} {
		v, err := evalIn(e, scriptDir, `require("`+p+`")`)
		if err != nil {
			t.Fatalf("require(%q) failed: %v", p, err)
		}
		if v != "flow" {
			t.Errorf("require(%q) = %v, want the flow's module", p, v)
		}
	}
}

func TestRequire_InlineIgnoresScriptDir(t *testing.T) {
	root := t.TempDir()
	flowDir := filepath.Join(root, "flow")
	writeModule(t, filepath.Join(flowDir, "lib.js"), `module.exports = "flow";`)
	writeModule(t, filepath.Join(root, "scripts", "lib.js"), `module.exports = "script";`)

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(flowDir)

	v, err := evalIn(e, "", `require("./lib.js")`)
	if err != nil {
		t.Fatalf("require failed: %v", err)
	}
	if v != "flow" {
		t.Errorf("got %v, want the flow's module", v)
	}
}

func TestRequire_ScriptDirRestored(t *testing.T) {
	root := t.TempDir()
	flowDir := filepath.Join(root, "flow")
	scriptDir := filepath.Join(root, "scripts")
	writeModule(t, filepath.Join(scriptDir, "only-here.js"), `module.exports = 1;`)
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		t.Fatal(err)
	}

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(flowDir)

	// A script that throws must restore the directory too.
	if err := e.WithRequireScriptDir(scriptDir, func() error {
		_, err := e.Eval(`throw new Error("boom")`)
		return err
	}); err == nil {
		t.Fatal("expected the script's error to be returned")
	}
	if e.requireScriptDir != "" {
		t.Errorf("requireScriptDir = %q after the script, want empty", e.requireScriptDir)
	}
	if _, err := e.Eval(`require("./only-here.js")`); err == nil {
		t.Error("a require after the script resolved against the script's directory")
	}
}

func TestRequire_NestedRequireDoesNotFallBack(t *testing.T) {
	root := t.TempDir()
	flowDir := filepath.Join(root, "flow")
	scriptDir := filepath.Join(root, "scripts")
	// helpers/a.js requires ./b.js, which exists only in the flow's directory:
	// a module's own require is relative to the module, with no fallback.
	writeModule(t, filepath.Join(scriptDir, "helpers", "a.js"), `module.exports = require("./b.js");`)
	writeModule(t, filepath.Join(flowDir, "b.js"), `module.exports = "flow";`)

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(flowDir)

	if _, err := evalIn(e, scriptDir, `require("./helpers/a.js")`); err == nil {
		t.Error("a nested require fell back to the flow's directory")
	}
}

// The same relative path from two script directories (or two flow
// directories) must load two modules, not the first one from the cache.
func TestRequire_SameRelativePathDifferentDirs(t *testing.T) {
	root := t.TempDir()
	writeModule(t, filepath.Join(root, "one", "lib.js"), `module.exports = "one";`)
	writeModule(t, filepath.Join(root, "two", "lib.js"), `module.exports = "two";`)

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(root)

	for _, want := range []string{"one", "two", "one"} {
		v, err := evalIn(e, filepath.Join(root, want), `require("./lib.js")`)
		if err != nil {
			t.Fatalf("require from script dir %s failed: %v", want, err)
		}
		if v != want {
			t.Errorf("script dir %s: got %v, want %v", want, v, want)
		}
	}

	for _, want := range []string{"one", "two"} {
		e.SetRequireBaseDir(filepath.Join(root, want))
		v, err := e.Eval(`require("./lib.js")`)
		if err != nil {
			t.Fatalf("require from flow dir %s failed: %v", want, err)
		}
		if v != want {
			t.Errorf("flow dir %s: got %v, want %v", want, v, want)
		}
	}
}

func TestRequire_AbsolutePathUnchanged(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "elsewhere", "lib.js")
	writeModule(t, abs, `module.exports = "abs";`)

	e := New()
	defer e.Close()
	e.SetRequireBaseDir(filepath.Join(root, "flow"))

	v, err := evalIn(e, filepath.Join(root, "scripts"), "require(`"+filepath.ToSlash(abs)+"`)")
	if err != nil {
		t.Fatalf("require failed: %v", err)
	}
	if v != "abs" {
		t.Errorf("got %v, want abs", v)
	}
}
