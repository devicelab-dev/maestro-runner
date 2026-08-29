# Language Clients

maestro-runner exposes two official clients that wrap the [REST API Server](../README.md#rest-api-server).
Both let you write Maestro tests in code instead of YAML — with IDE autocomplete,
type checking, and the Page Object Model pattern — while reusing the exact same
drivers, selectors, and assertions as the CLI.

## Clients

- [TypeScript](typescript.md) — `MaestroClient` for Node.js test runners (Jest, Vitest, Playwright).
- [Python](python.md) — `MaestroClient` for pytest-based E2E suites, with built-in `pytest-xdist` parallel support.

## How they fit together

```
┌──────────────┐   JSON over HTTP   ┌────────────────────────┐   Maestro steps   ┌──────────────┐
│ TS / Python  │ ─────────────────▶ │  maestro-runner server  │ ────────────────▶ │ Android/iOS/ │
│   client     │                    │  (maestro-runner server)│                   │   Web device │
└──────────────┘                    └────────────────────────┘                   └──────────────┘
```

Every client call maps to a session endpoint (`POST /session`, `POST /session/{id}/execute`,
`GET /session/{id}/screenshot`, …). Start the server once, then drive it from either client:

```bash
maestro-runner server --port 9999
```

## Common setup

Both clients share the same environment variables:

| Variable             | Default                  | Description                       |
|----------------------|--------------------------|-----------------------------------|
| `MAESTRO_SERVER_URL` | `http://localhost:9999`  | Base URL of the running server    |
| `MAESTRO_PLATFORM`   | `android`                | Target platform (`android`/`ios`/`web`) |
| `MAESTRO_RUNNER_BIN` | `../../maestro-runner`   | Path to the `maestro-runner` binary (used to auto-start a server in tests) |
