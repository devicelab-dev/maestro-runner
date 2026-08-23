---
name: python-test-runner
description: >
  Runs tests, lint, and type checks for the maestro-runner Python client at
  client/python/. Use this skill whenever the user mentions pytest, ruff,
  mypy, the Python venv, or running/debugging tests for client/python/ — even
  if they just say "run the tests", "why is the Python test failing", "test
  won't pass", "module not found", "venv isn't working", or the context is
  clearly Python. Use for any pytest or make lint-py command. Automatically
  handles test sequencing and device lock conflicts for "run all tests"
  requests. DO NOT use for TypeScript tests, Go tests, or server-side code —
  use the typescript-test-runner skill or run Go tests directly.
allowed-tools: "Bash(python:*) Bash(python3:*) Bash(pip:*) Bash(pip3:*) Bash(pytest:*) Bash(ruff:*) Bash(mypy:*) Bash(make:*) Bash(adb:*) Bash(curl:*) Bash(source:*)"
metadata:
  author: maestro-runner
  version: 1.1.0
  category: testing
  tags: [python, pytest, e2e, android, lint, mypy, ruff, test-sequencing, device-lock]
---

# Python Test Runner

Runs tests, lint, and type checks for the Python client at `client/python/`.

## Do NOT use this skill for

- TypeScript tests → use the `typescript-test-runner` skill
- Go / server tests → run `go test ./...` directly
- General Python questions unrelated to running tests

## Prerequisites

- Python venv at `client/python/.venv/` — activate it before every command or
  pytest will use the system Python and not find the project's dependencies
- Prefer the project-local venv (`client/python/.venv`) over repo-root venvs.
  If `python -m pytest` says `No module named pytest`, you're likely using the
  wrong interpreter.
- For e2e/Android tests: Android emulator running + `maestro-runner` binary built

```sh
# Activate the venv first — all commands below assume it is active
cd client/python && source .venv/bin/activate

# Or run directly without activating
cd client/python && ./.venv/bin/python -m pytest -v
```

## Step 1: Unit Tests (no device needed)

```sh
# All unit tests
cd client/python && source .venv/bin/activate && python -m pytest tests/test_client.py tests/test_models.py -v

# Single file
python -m pytest tests/test_client.py -v

# Specific test
python -m pytest tests/test_client.py::TestSessionManagement::test_create_session -v

# Parallel execution
python -m pytest tests/test_client.py tests/test_models.py -n auto -v
```

## Step 2: E2E Android Tests

Requires the server to be running first.

### 1. Check emulator is attached
```sh
adb devices
```

### 2. Start the server (if not already running)
```sh
# From repo root — runs in background
./maestro-runner --platform android server --port 9999 &>/tmp/maestro-server.log &

# Verify it's up
curl -s http://localhost:9999/status

# Optional: ensure no stale server process is holding the device
pgrep -af "maestro-runner.*server" || true
```

### 3. Run the tests
```sh
cd client/python && source .venv/bin/activate && python -m pytest tests/test_e2e_android.py -v
```

To target a different server URL or device:
```sh
MAESTRO_SERVER_URL=http://localhost:8888 python -m pytest tests/test_e2e_android.py -v
MAESTRO_DEVICE_ID=emulator-5554 python -m pytest tests/test_e2e_android.py -v
```

## Step 3: Page-Object / Integration Tests (need device + server)

```sh
cd client/python && source .venv/bin/activate && \
  python -m pytest tests/test_add_contact.py tests/test_contact_persists.py -n auto -v
```

## Step 4: iOS Tests

Requires iOS simulator running with the server started against that simulator.

```sh
# Start the server targeting the iOS simulator
./maestro-runner --platform ios --device <UDID> server --port 9999 &>/tmp/maestro-server.log &
curl -s http://localhost:9999/status

# Run the iOS contact test
cd client/python && source .venv/bin/activate && \
  MAESTRO_PLATFORM=ios MAESTRO_DEVICE_ID=<UDID> \
  python -m pytest tests/test_add_contact_ios.py -v
```

Environment variables for iOS:

| Variable | Example | Description |
|----------|---------|-------------|
| `MAESTRO_PLATFORM` | `ios` | Must be set to `ios` |
| `MAESTRO_DEVICE_ID` | `E0E08E8A-29CC-4A5C-91D7-9799C245B140` | iOS simulator UDID |
| `MAESTRO_SERVER_URL` | `http://localhost:9999` | Server URL (default) |
| `MAESTRO_RUNNER_BIN` | `../../maestro-runner` | Path to binary (auto-detected) |

## Step 5: Lint

```sh
# From repo root via Makefile
make lint-py

# Or directly
cd client/python && source .venv/bin/activate
ruff check maestro_runner tests
mypy maestro_runner

# Auto-fix
make lint-py-fix
```

## Run All Tests (Complete Suite)

**Use this when asked to "run all tests"** — handles proper sequencing and device lock mitigation:

```sh
# 1) Unit tests (no device needed — always run first, fastest)
cd client/python && ./.venv/bin/python -m pytest tests/test_client.py tests/test_models.py -v

# 2) Page-object / integration tests (reuse existing server session)
./.venv/bin/python -m pytest tests/test_add_contact.py tests/test_contact_persists.py -v

# 3) Clean up stale server processes to avoid device-lock conflicts
pkill -f "maestro-runner.*server" || true
sleep 2

# 4) Start fresh server for e2e tests (from repo root)
./maestro-runner --platform android server --port 9999 &>/tmp/maestro-server.log &
sleep 2
curl -s http://localhost:9999/status

# 5) E2E tests with exclusive device access
cd client/python && ./.venv/bin/python -m pytest tests/test_e2e_android.py -v
```

**Why this order:**
- Unit tests run first (fastest, no device needed)
- Integration tests share the server session
- Device lock is released before e2e tests to prevent "device already in use" errors
- E2E tests run last with a fresh server connection

## Reports

HTML and JUnit XML reports are written to `client/python/reports/` after every pytest run:
- `reports/report.html`
- `reports/junit-report.xml`

## Common Issues

See **`device-integration-testing`** for the shared harness pitfalls
(device lock, stale `/tmp/uia2-*` PID guard, `pkill -f maestro-runner`
self-match, teardown stream races). Python-specific issues:

| Problem | Fix |
|---------|-----|
| `No module named pytest` | Wrong interpreter — use `client/python/.venv/bin/python` |
| `Connection refused` on e2e tests | Server not running — conftest auto-starts it, or set `MAESTRO_RUNNER_BIN` |
| `device ... is already in use` | Reuse the conftest `client` session (don't open a second). Details in `device-integration-testing`. |
| `pytest-html` plugin missing | `pip install pytest-html` (referenced by `pyproject.toml` addopts) |
| `adb: command not found` | Android SDK not on PATH; set `ANDROID_HOME` |
| Lint `E501` line-too-long | Line length limit is 100; wrap long lines |

### Running a single integration file

The conftest auto-starts the server when none is reachable, so you normally
just run pytest — no manual server needed:

```sh
cd client/python && source .venv/bin/activate
MAESTRO_RUNNER_BIN=../../maestro-runner MAESTRO_DEVICE_ID=emulator-5554 \
  MAESTRO_PLATFORM=android python -m pytest tests/test_e2e_android.py -v
```

If you do start a server manually, use a unique port and don't also let
conftest start one; two servers on the same port conflict.

