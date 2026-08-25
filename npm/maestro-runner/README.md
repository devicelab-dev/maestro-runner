# maestro-runner

Fast mobile UI test automation for Android, iOS, React Native, Flutter and Expo. Runs Maestro YAML flows from a single Go binary — no JVM, no Appium server required.

```bash
npx maestro-runner test flows/
```

Or add it to a project:

```bash
npm install --save-dev maestro-runner
```

## Why npm

The runner is a single binary and installs the same way from a shell script. This package exists because if your app is React Native or Expo, your toolchain is already npm — pinning the test runner in `package.json` means every machine and every CI job runs the same version, no separate bootstrap step.

There is no postinstall script and nothing is downloaded when you install. The binary for your platform arrives as an ordinary optional dependency that npm selects by `os` and `cpu`, so installs work offline, behind a proxy, and in CI environments that block postinstall network access.

## First run

```bash
npx maestro-runner doctor     # check the toolchain and what is missing
npx maestro-runner devices    # what this machine can drive
npx maestro-runner test flows/
```

## Platforms

macOS and Linux, on arm64 and x64. Windows is not supported; WSL works.

## Documentation

- Getting started — https://open.devicelab.dev/maestro-runner/docs/getting-started
- CLI reference — https://open.devicelab.dev/maestro-runner/docs/cli-reference
- Flow commands — https://open.devicelab.dev/maestro-runner/docs/flow-commands

## License

Apache-2.0
