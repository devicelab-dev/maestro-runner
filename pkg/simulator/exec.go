package simulator

import "os/exec"

// execCommand is the package-level seam over exec.Command for testability.
// Production code MUST call this variable, not exec.Command directly.
// Tests override via t.Cleanup(func(){ execCommand = exec.Command }) and
// must not run with t.Parallel().
var execCommand = exec.Command

// execLookPath is the companion seam over exec.LookPath. Same rules.
var execLookPath = exec.LookPath
