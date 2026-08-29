// Resolving which platform package supplies the binary.
//
// Kept separate from the CLI entry point so it can be required by consumers
// that want the path rather than a subprocess — a Node test harness shelling
// out to the runner, for instance.

const path = require('node:path');

// npm's `cpu` field uses Node's process.arch names, so the package names below
// use them too: x64 rather than the amd64 of the release tarballs.
const PACKAGE_BY_TARGET = {
  'darwin-arm64': '@devicelab/maestro-runner-darwin-arm64',
  'darwin-x64': '@devicelab/maestro-runner-darwin-x64',
  'linux-arm64': '@devicelab/maestro-runner-linux-arm64',
  'linux-x64': '@devicelab/maestro-runner-linux-x64',
};

function target() {
  return `${process.platform}-${process.arch}`;
}

/**
 * Absolute path to the maestro-runner binary for this platform.
 * Throws with an actionable message rather than a bare MODULE_NOT_FOUND.
 */
function binaryPath() {
  const key = target();
  const pkg = PACKAGE_BY_TARGET[key];

  if (!pkg) {
    throw new Error(
      `maestro-runner has no build for ${key}. Supported: ${Object.keys(PACKAGE_BY_TARGET).join(', ')}. ` +
        `Windows users can run it under WSL.`
    );
  }

  try {
    // Resolve the package's own manifest rather than the binary: a package's
    // main entry may be absent, but its package.json always resolves, and it
    // anchors us to the install directory wherever npm placed it.
    // Search the consumer's tree as well as our own. A registry install puts
    // both packages side by side and the default lookup finds it; `npm link`
    // symlinks this package back to a checkout, where the platform package is
    // not a sibling and only the consumer's node_modules has it.
    const from = require.resolve(`${pkg}/package.json`, { paths: [__dirname, process.cwd()] });
    return path.join(path.dirname(from), 'bin', 'maestro-runner');
  } catch {
    throw new Error(
      `maestro-runner: the ${pkg} package is missing. This usually means the install ran with ` +
        `--no-optional, or the lockfile was built on a different platform. Reinstall without ` +
        `--no-optional, or install ${pkg} directly.`
    );
  }
}

module.exports = { binaryPath, target, PACKAGE_BY_TARGET };
