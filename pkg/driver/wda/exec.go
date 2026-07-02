package wda

import "os/exec"

// execCommand is the package-level seam over exec.Command so tests can
// fake out external commands like `xcrun simctl`. Production code paths
// added with a test seam MUST call this variable, not exec.Command.
// Tests override via t.Cleanup(func(){ execCommand = exec.Command }) and
// must not run with t.Parallel().
var execCommand = exec.Command
