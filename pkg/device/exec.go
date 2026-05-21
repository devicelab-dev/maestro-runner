package device

import "os/exec"

// execCommand is a package-level seam over exec.Command so tests can
// override subprocess execution without spawning real children. Production
// code MUST always call this variable, not exec.Command directly. Tests
// override it via t.Cleanup(func(){ execCommand = exec.Command }) — they
// do not run with t.Parallel() to avoid clobbering.
var execCommand = exec.Command

// execLookPath is the companion seam over exec.LookPath. Same rules.
var execLookPath = exec.LookPath
