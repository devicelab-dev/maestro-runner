# REST API

`maestro-runner server` exposes the runner over HTTP so any language can drive a device: create a session, send Maestro steps as JSON, read screenshots and the UI hierarchy, then tear the session down. It is the transport behind the Python/TypeScript bindings and is intended for localhost or CI use.

If you are writing a Go program instead, the in-process API in [go-library.md](go-library.md) is the better fit — no server, no HTTP.

## Start the server

```bash
maestro-runner server                    # listens on :9999
maestro-runner server --port 8080        # custom port
MAESTRO_SERVER_PORT=8080 maestro-runner server
```

```
maestro-runner server listening on :9999
  POST   /session              - Create a new session
  POST   /session/{id}/execute  - Execute a step
  GET    /session/{id}/screenshot - Take screenshot
  GET    /session/{id}/source    - Get view hierarchy
  GET    /session/{id}/device-info - Get device info
  DELETE /session/{id}           - Delete session
  GET    /status                 - Server status
```

The server inherits these global flags so every session it creates is configured the same way:

| Flag | Used for |
|---|---|
| `--app-file` | App binary installed before the session (`apk`, `app`, `ipa`) |
| `--appium-url` | Appium server URL for the `appium` driver (default `http://127.0.0.1:4723`) |
| `--caps` | Appium capabilities JSON file |
| `--team-id` | Apple Development Team ID (iOS) |

`SIGINT`/`SIGTERM` shuts the server down gracefully — every live session is cleaned up.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/status` | Readiness + number of live sessions |
| `POST` | `/session` | Create a session |
| `POST` | `/session/{id}/execute` | Execute one step |
| `GET` | `/session/{id}/screenshot` | PNG screenshot |
| `GET` | `/session/{id}/source` | UI hierarchy (XML or JSON) |
| `GET` | `/session/{id}/device-info` | Platform/device metadata |
| `DELETE` | `/session/{id}` | Delete the session (runs cleanup) |

### `POST /session`

```json
{
  "platformName": "android",
  "deviceId": "emulator-5554",
  "appId": "com.example.app",
  "driver": "devicelab"
}
```

| Field | Required | Notes |
|---|---|---|
| `platformName` | yes | `android` or `ios` |
| `deviceId` | no | Device serial/UDID; the runner picks a device when omitted |
| `appId` | no | Bundle id / package; used by `launchApp` and returned by device info |
| `driver` | no | Per-session driver override; see [Choosing a driver](#choosing-a-driver) |

Response:

```json
{ "sessionId": "cb86147ba156d7f801ebe4c5f8025620" }
```

Sessions are independent: one server can hold sessions on different devices and different drivers at the same time. `driver` is a **per-session** field, not a server flag.

### Choosing a driver

| Platform | `driver` values |
|---|---|
| Android | `devicelab` (default), `uiautomator2`, `appium` |
| iOS | `devicelab` (default), `wda`, `devicelab-legacy`, `appium` |

The `appium` driver talks to an **already-running** Appium server (`--appium-url`, default `http://127.0.0.1:4723`); the runner never starts Appium for you. `uiautomator2` starts its own on-device server — no external process.

### `POST /session/{id}/execute`

The body is one step as JSON. A `type` discriminator selects the step, and the remaining fields mirror the YAML keys.

```json
{ "type": "launchApp", "appId": "com.example.app" }
```

```json
{ "type": "assertVisible", "selector": "Search" }
```

```json
{
  "type": "tapOn",
  "selector": { "text": "Login", "id": "btn_login" },
  "timeout": 5000,
  "optional": true
}
```

```json
{ "type": "inputText", "selector": { "text": "First name" }, "text": "Ada" }
```

- `selector` accepts a plain string (text match) or an object (`text`, `id`, `css`, `index`, `childOf`, `below`, … — the same selector model as YAML).
- `timeout` (ms) and `optional` are supported wherever YAML supports them.

Response — HTTP `200` for a step that executed, **even when the step failed**:

```json
{
  "success": true,
  "duration": 114291250,
  "message": "Entered text: Ada",
  "element": { "text": "First name", "bounds": { "x": 137, "y": 793, "width": 796, "height": 154 }, "visible": true, "enabled": true }
}
```

```json
{ "success": false, "duration": 17105648792, "message": "Element not visible: element 'REST API' not found" }
```

Check `success`, not the status code. `duration` is nanoseconds (a Go `time.Duration`). `element` is present when the step resolved an element; its extra fields (`id`, `class`, …) are driver-specific.

Supported step types (36): `tapOn`, `doubleTapOn`, `longPressOn`, `tapOnPoint`, `swipe`, `scroll`, `scrollUntilVisible`, `back`, `hideKeyboard`, `acceptAlert`, `isKeyboardVisible`, `dismissAlert`, `inputText`, `inputRandom`, `eraseText`, `copyTextFrom`, `pasteText`, `setClipboard`, `assertVisible`, `assertNotVisible`, `assertTrue`, `assertCondition`, `launchApp`, `stopApp`, `killApp`, `clearState`, `clearKeychain`, `setPermissions`, `setLocation`, `setOrientation`, `openLink`, `openBrowser`, `pressKey`, `sleep`, `waitForAnimationToEnd`, `takeScreenshot`.

Any other type (for example `dragAndDrop`, `openNotifications`, `runFlow`, `repeat`, `assertWithAI`) fails **loudly** with HTTP `400` and an error naming the type — the server never silently accepts a step it cannot execute.

### `GET /session/{id}/screenshot`

Returns `image/png` bytes.

### `GET /session/{id}/source`

Returns the view hierarchy as `application/xml` (Android `devicelab`/`uiautomator2`, both XML) or `application/json` when the driver returns a JSON hierarchy. The content type is chosen from the payload.

### `GET /session/{id}/device-info`

Proxies the driver's platform info. The exact fields vary by driver:

```json
{
  "platform": "android",
  "osVersion": "35",
  "deviceName": "google sdk_gphone64_arm64",
  "deviceId": "emulator-5554",
  "isSimulator": true,
  "screenWidth": 1080,
  "screenHeight": 2424,
  "appId": "com.example.app",
  "appVersion": "4.32.50.633180122",
  "appBuild": "3176399"
}
```

### `DELETE /session/{id}`

Cleanup runs (on-device runner stopped, emulator shut down when the runner started it) and the server replies `204 No Content`.

## Errors

| Status | When | Body |
|---|---|---|
| `400` | Session body is not JSON | `{"error":"invalid request body: …"}` |
| `400` | `platformName` missing | `{"error":"platformName is required"}` |
| `400` | Step body is not JSON | `{"error":"invalid step: unmarshal step type: …"}` |
| `400` | Step has no `type` | `{"error":"invalid step: missing \"type\" field in step JSON"}` |
| `400` | Step type unsupported | `{"error":"invalid step: unsupported step type: openNotifications"}` |
| `404` | No such session (all `/session/{id}/*` routes) | `{"error":"session deadbeef not found"}` |
| `500` | Driver creation / screenshot / hierarchy failed | `{"error":"failed to create driver: …"}` |

## Full example

Create a contact through the API (Android, default `devicelab` driver):

```bash
BASE=http://127.0.0.1:9999

# 1. Create the session
SID=$(curl -s -X POST $BASE/session -H 'Content-Type: application/json' \
  -d '{"platformName":"android","deviceId":"emulator-5554","appId":"com.google.android.contacts"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["sessionId"])')

# 2. Execute steps
step() { curl -s -X POST $BASE/session/$SID/execute -H 'Content-Type: application/json' -d "$1"; echo; }

step '{"type":"launchApp","appId":"com.google.android.contacts"}'
step '{"type":"assertVisible","selector":"Create contact"}'
step '{"type":"tapOn","selector":"Create contact"}'
step '{"type":"inputText","selector":{"text":"First name"},"text":"Ada Lovelace"}'
step '{"type":"tapOn","selector":"Save"}'
step '{"type":"assertVisible","selector":"Ada Lovelace"}'

# 3. Inspect the device
curl -s $BASE/session/$SID/device-info; echo
curl -s $BASE/session/$SID/screenshot -o screen.png
curl -s $BASE/session/$SID/source > hierarchy.xml

# 4. Tear down
curl -s -X DELETE $BASE/session/$SID -o /dev/null -w '%{http_code}\n'   # 204
```

## Limitations

- Sessions are **in-memory**: restarting the server drops every session. Recreate after a reconnect.
- No authentication layer — bind to localhost or a trusted CI network.
- Steps are executed **one at a time**; flow-level constructs (`repeat`, `retry`, `runFlow`, `runScript`) are not part of the JSON protocol, so compose them in your client.
- `platformName` other than `android`/`ios` is rejected (`web` is not served by the REST API).
