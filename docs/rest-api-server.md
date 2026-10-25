# REST API Server

maestro-runner can run as an HTTP server that exposes a session-based REST API for executing Maestro steps via JSON. This is useful for remote orchestration, language-agnostic clients, and integrating maestro-runner into custom tooling.

## Start the server

```bash
maestro-runner server
maestro-runner server --port 9999
maestro-runner --platform android server
```

The server prints the available endpoints on startup. By default it listens on port `9999` (configurable via `--port` or `MAESTRO_SERVER_PORT`).

### Web session flags

When running web sessions through the server, you can pass browser options via flags (these apply to all web sessions created by the server):

| Flag | Description | Env var |
|------|-------------|---------|
| `--headed` | Show the browser window (default is headless) | — |
| `--browser` | Browser binary/path or `chrome`/`chromium` | `MAESTRO_BROWSER` |
| `--window-size`, `--screen-size` | Viewport size as `WxH` (default `1280x800`) | `MAESTRO_WINDOW_SIZE` |
| `--user-data-dir` | Chrome user-data directory for persistent profiles | `MAESTRO_USER_DATA_DIR` |

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/status` | Server status and active session count |
| `POST` | `/session` | Create a new driver session |
| `POST` | `/session/{id}/execute` | Execute a Maestro step |
| `GET` | `/session/{id}/screenshot` | Capture a screenshot (PNG) |
| `GET` | `/session/{id}/source` | Get the view hierarchy (XML or JSON) |
| `GET` | `/session/{id}/device-info` | Get device information |
| `DELETE` | `/session/{id}` | Close the session and clean up resources |

## Creating a session

`POST /session` accepts a JSON body:

```json
{
  "platformName": "android",  // "android", "ios", or "web"
  "deviceId": "emulator-5554",
  "appId": "com.example.app",
  "driver": "uiautomator2"    // optional: android/ios driver override
}
```

For web sessions, `appId` is the URL to open (e.g., `http://localhost:3000`).

Example:

```bash
curl -X POST http://localhost:9999/session \
  -H 'Content-Type: application/json' \
  -d '{"platformName":"android","deviceId":"emulator-5554"}'
```

Response:

```json
{
  "sessionId": "9f3e..."
}
```

## Executing steps

`POST /session/{id}/execute` accepts a JSON step object. Steps use the same schema as Maestro flows and are decoded into the same `flow.Step` types used by YAML parsing.

```bash
curl -X POST http://localhost:9999/session/$SESSION_ID/execute \
  -H 'Content-Type: application/json' \
  -d '{"type":"tapOn","selector":{"text":"Login"}}'
```

Response:

```json
{
  "success": true,
  "duration": 123456789,
  "message": "...",
  "element": { ... }  // optional, when applicable
}
```

### Selectors

Selectors can be provided as:
- A plain string (treated as text selector): `"Login"`
- A structured object: `{"text": "Login", "id": "login_btn"}`

All selector fields from Maestro flows are supported (text, id, index, enabled/selected/checked/focused, etc.).

### Supported step types

Any step type supported by the underlying driver can be executed. Common examples: `launchApp`, `tapOn`, `assertVisible`, `inputText`, `swipe`, `back`, `pressKey`, `scrollUntilVisible`, `waitForAnimationToEnd`, and more.

Malformed or unsupported steps return HTTP 400 with an error message.

## Other endpoints

### Get status
```bash
curl http://localhost:9999/status
```
```json
{"sessions": 1, "status": "ok"}
```

### Take screenshot
```bash
curl http://localhost:9999/session/$SESSION_ID/screenshot -o screenshot.png
```
Returns `image/png`.

### Get view hierarchy
```bash
curl http://localhost:9999/session/$SESSION_ID/source
```
Returns XML (`application/xml`) for native Android/iOS hierarchies, or JSON (`application/json`) for web/CDP hierarchies (both object and array forms are supported).

### Get device info
```bash
curl http://localhost:9999/session/$SESSION_ID/device-info
```
```json
{
  "platform": "android",
  "osVersion": "36",
  "deviceName": "Pixel 8",
  "deviceId": "emulator-5554",
  "isSimulator": true,
  "screenWidth": 1080,
  "screenHeight": 2400
}
```

### Delete session
```bash
curl -X DELETE http://localhost:9999/session/$SESSION_ID
```
Returns HTTP 204 on success.

## Driver support

The server uses the same driver factory as the CLI. Sessions behave like a `test` run on the selected platform/driver:

| Platform | Drivers | Notes |
|----------|---------|-------|
| `android` | DeviceLab (default), `uiautomator2`, `appium` | Set `"driver": "uiautomator2"` or `"appium"` in the session request. Appium requires `--appium-url` on the server or appropriate configuration. |
| `ios` | DeviceLab (default), `wda`, `devicelab-legacy`, `appium` | Use `"driver": "wda"` for WebDriverAgent or `"driver": "appium"`. `devicelab-legacy` selects the previous runner. WDA can be configured with flags like `--team-id`, `--wda-url`, `--reuse-wda` passed to `maestro-runner server`. |
| `web` | Chrome/Chromium via CDP | Use `--browser`, `--headed`, `--window-size`. `appId` should be the URL to navigate to. |

## Notes

- Sessions are concurrency-safe and isolated per session ID.
- On shutdown (SIGINT/SIGTERM), the server cleans up all active sessions.
- Worker-aware traces are emitted for requests and step execution with sensitive values redacted.
- The JSON step protocol reuses `flow.Step` types and selector semantics for consistency with YAML flows.
