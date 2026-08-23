---
name: typescript-test-runner
description: >
  Runs tests, lint, and build for the maestro-runner TypeScript client at
  client/typescript/. Use this skill whenever the user mentions TypeScript
  tests, Jest, npm run test:unit, npm run test:device, e2e tests, linting, or
  building the TS client — even if they don't say "TypeScript" explicitly,
  apply this skill whenever the context is clearly client/typescript/. Use
  this skill when the user says a TypeScript test is failing, "why is the Jest
  test red", "module not found", "can't connect to server", or when running
  iOS or Android device tests from the TypeScript client. Handles server
  startup automatically via setup.ts. Automatically handles test sequencing
  and device lock conflicts for "run all tests" requests. DO NOT use for
  Python tests, Go tests, or server-side code — use the python-test-runner
  skill or run Go tests directly.
allowed-tools: "Bash(npm:*) Bash(npx:*) Bash(node:*) Bash(adb:*) Bash(curl:*) Bash(make:*) Bash(pkill:*) Bash(sleep:*)"
metadata:
  author: maestro-runner
  version: 1.1.0
  category: testing
  tags: [typescript, jest, e2e, android, lint, build, test-sequencing, device-lock]
---

# TypeScript Test Runner

Runs tests, lint, and build for the TypeScript client at `client/typescript/`.

## Do NOT use this skill for

- Python tests → use the `python-test-runner` skill
- Go / server tests → run `go test ./...` directly
- General TypeScript questions unrelated to running tests

## Prerequisites

- **Node.js** ≥ 18 (`node --version`)
- Dependencies installed: `npm install` inside `client/typescript/`
- For e2e tests: Android emulator running + `maestro-runner` binary built

## Step 0: Setup (first time only)

```sh
cd client/typescript && npm install
```
## Run All Tests (Complete Suite)

**Use this when asked to "run all tests"** — handles proper sequencing and device lock mitigation:

```sh
# 1) Clean up any stale maestro-runner server processes
pkill -f "maestro-runner.*server" || true
sleep 2

# 2) Run unit tests in parallel (no real device required)
cd client/typescript && npm run test:unit

# 3) Run Android real-device tests in serial mode
cd client/typescript && npm run test:device:android

# 4) Run iOS real-device tests in serial mode
cd client/typescript && \
  MAESTRO_PLATFORM=ios MAESTRO_DEVICE_ID=<UDID> npm run test:device:ios
```

**Why this works:**
- Stale server processes are cleaned up before tests to prevent device lock conflicts
- `setup.ts` auto-starts a fresh maestro-runner server for the test suite
- Single-worker execution avoids session/device races on single-emulator setups

To run only one group directly:
```sh
cd client/typescript && npm run test:unit
cd client/typescript && npm run test:device:android
cd client/typescript && MAESTRO_PLATFORM=ios MAESTRO_DEVICE_ID=<UDID> npm run test:device:ios
```


## Step 1: Unit Tests (parallel-safe)

The test setup (`tests/setup.ts`) auto-starts the maestro-runner server if it isn't already running — no manual server startup needed.

```sh
# Unit tests only (parallel-safe)
cd client/typescript && npm run test:unit

# Android real-device tests (serial)
npm run test:device:android

# iOS real-device tests (serial, requires MAESTRO_DEVICE_ID)
MAESTRO_PLATFORM=ios MAESTRO_DEVICE_ID=<UDID> npm run test:device:ios

# Specific test file
npx jest tests/test_add_contact.test.ts

# Specific test by name pattern
npx jest -t "should add a contact"

# Watch mode (re-runs on file change)
npx jest --watch
```

## Step 2: Real-Device Android Tests (serial)

### 1. Check emulator is attached
```sh
adb devices
```

### 2. Run (server is auto-started by setup.ts)
```sh
cd client/typescript && npm run test:device:android
```

To target a different server:
```sh
MAESTRO_SERVER_URL=http://localhost:8888 MAESTRO_PLATFORM=android npm run test:device:android
```

## Step 2b: Real-Device iOS Tests (serial)

### 1. Check simulator is running
```sh
xcrun simctl list devices booted
```

### 2. Run (server is auto-started by setup.ts with the iOS platform)
```sh
cd client/typescript && \
  MAESTRO_PLATFORM=ios MAESTRO_DEVICE_ID=<UDID> npm run test:device:ios
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAESTRO_SERVER_URL` | `http://localhost:9999` | Server URL |
| `MAESTRO_PLATFORM` | `android` | Target platform (`android` / `ios`) |
| `MAESTRO_DEVICE_ID` | _(unset)_ | Device/simulator UDID (required for iOS; optional for Android) |
| `MAESTRO_RUNNER_BIN` | `../../maestro-runner` | Path to maestro-runner binary |

## Step 3 (optional): Manual Server Startup

If `setup.ts` can't locate the binary or you want to manage the server yourself:

```sh
# From repo root — Android
./maestro-runner --platform android server --port 9999 &>/tmp/maestro-server.log &

# From repo root — iOS (provide simulator UDID)
./maestro-runner --platform ios --device <UDID> server --port 9999 &>/tmp/maestro-server.log &

# Verify
curl -s http://localhost:9999/status
```

## Step 4: Build

```sh
cd client/typescript && npm run build
# Output: dist/ (JS + .d.ts + source maps)
```

## Step 5: Lint

```sh
cd client/typescript && npm run lint        # Check for issues
cd client/typescript && npm run lint:fix    # Auto-fix what's possible
```

Key ESLint rules: `consistent-type-imports`, `no-explicit-any` (warn in `src/`), `no-unused-vars`, `eqeqeq`, `no-console` (warn in `src/`).

## Reports

HTML and JUnit XML reports are written to `client/typescript/reports/` after every Jest run:
- `reports/report.html`
- `reports/junit-report.xml`

## Common Issues

See **`device-integration-testing`** for the shared harness pitfalls
(device lock, stale `/tmp/uia2-*` PID guard, `pkill -f maestro-runner`
self-match, teardown stream races — including the `setup.ts` `write after end`
fix). TypeScript-specific issues:

| Problem | Fix |
|---------|-----|
| `Connection refused` | Server failed to auto-start; check `MAESTRO_RUNNER_BIN` path or start manually |
| `Cannot find module` | Dependencies not installed: `npm install` |
| `adb: command not found` | Android SDK not on PATH; set `ANDROID_HOME` |
| TypeScript compile errors | Run `npm run build` to see full tsc diagnostics |
| Lint `no-explicit-any` error | Avoid `any` in `src/`; use proper types or `unknown` |
