#!/usr/bin/env node
// Entry point for `npx maestro-runner`.
//
// The real program is a Go binary, shipped in one of four platform packages
// that npm installs selectively via optionalDependencies and their os/cpu
// fields. This shim finds the one that matched and hands over to it.
//
// There is deliberately no postinstall step and nothing is downloaded at
// install time: npm resolves the right package from the lockfile, so installs
// work offline, behind a proxy, and in the locked-down CI environments where a
// postinstall download is exactly what gets blocked.

const { spawnSync } = require('node:child_process');
const { binaryPath } = require('../lib/resolve.js');

const result = spawnSync(binaryPath(), process.argv.slice(2), { stdio: 'inherit' });

if (result.error) {
  console.error(`maestro-runner: ${result.error.message}`);
  process.exit(1);
}

// A binary killed by a signal has no exit code. Report it the way a shell
// would, so a CI step sees a failure rather than a silent success.
if (result.signal) {
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
