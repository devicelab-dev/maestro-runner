# maestro-runner Python Client

Python client for the `maestro-runner` REST API server.

## Installation

```bash
pip install -e .
```

## Quick Start

```python
from maestro_runner import MaestroClient

# Start maestro-runner server first:
#   maestro-runner server --port 9999

with MaestroClient(
    "http://localhost:9999",
    capabilities={"platformName": "android", "appId": "com.example.app"},
) as c:
    c.launch_app("com.example.app")
    c.tap(text="Login")
    c.input_text("user@example.com")
    c.assert_visible(text="Dashboard", timeout_ms=10000)

    info = c.device_info()
    print(f"Device: {info.device_name} ({info.platform} {info.os_version})")
```

## Running Tests

### Sequential (single device)

```bash
cd client/python
python3 -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"

pytest tests/test_add_contact.py tests/test_contact_persists.py -v
```

### Parallel (multiple devices)

Run tests in parallel across multiple Android emulators using
[pytest-xdist](https://pypi.org/project/pytest-xdist/). Each worker
automatically starts its own `maestro-runner` server on a unique port and
targets a specific device.

**Prerequisites:**

1. Two or more Android emulators running (`adb devices` shows them).
2. `pytest-xdist` installed:

   ```bash
   pip install pytest-xdist
   ```

**Run with `-n <workers>`:**

```bash
# Run on 2 emulators in parallel
pytest tests/test_add_contact.py tests/test_contact_persists.py -n 2 -v
```

Worker `gw0` gets the first device (e.g. `emulator-5554`) on port 9999,
`gw1` gets the second device (e.g. `emulator-5556`) on port 10000, and so on.

**Environment variables:**

| Variable             | Default                  | Description                        |
|----------------------|--------------------------|------------------------------------|
| `MAESTRO_SERVER_URL` | `http://localhost:9999`  | Base URL (port used as starting port in parallel mode) |
| `MAESTRO_PLATFORM`   | `android`                | Target platform                    |
| `MAESTRO_RUNNER_BIN` | `../../maestro-runner`   | Path to the maestro-runner binary  |

## API

See `maestro_runner/client.py` for the full API. Highlights beyond the basics above:

```python
# Grant/deny app permissions (omitted appId falls back to the flow's appId)
c.set_permissions("com.example.app", {"camera": "allow", "microphone": "deny"})

# Reset browser permissions (web platform only)
c.reset_permissions()

# Run JavaScript inside a mobile WebView via CDP
c.eval_webview_script("document.title", output="title")
c.run_webview_script("scripts/login.js", env={"USERNAME": "alice"}, output="result")
```

### Wrapped step types

| Method | Server step |
|--------|-------------|
| `set_permissions` / `reset_permissions` | `setPermissions` / `resetPermissions` |
| `eval_webview_script` / `run_webview_script` | `evalWebViewScript` / `runWebViewScript` |
| `double_tap_on` / `long_press_on` | `doubleTapOn` / `longPressOn` |
| `drag_and_drop` / `scroll_until_visible` | `dragAndDrop` / `scrollUntilVisible` |
| `assert_screenshot` / `take_screenshot` | `assertScreenshot` / `takeScreenshot` |
| `copy_text_from` / `paste_text` / `set_clipboard` | `copyTextFrom` / `pasteText` / `setClipboard` |
| `assert_with_ai` / `eval_script` / `run_script` | `assertWithAI` / `evalScript` / `runScript` |
| `eval_browser_script` | `evalBrowserScript` |
| `set_location` / `set_airplane_mode` / `toggle_airplane_mode` | `setLocation` / `setAirplaneMode` / `toggleAirplaneMode` |
| `set_network_conditions` / `open_notifications` | `setNetworkConditions` / `openNotifications` |
| `set_dark_mode` / `set_orientation` | `setDarkMode` / `setOrientation` |
| `open_browser` / `switch_tab` / `close_tab` | `openBrowser` / `switchTab` / `closeTab` |
| `get_console_logs` / `clear_console_logs` / `assert_no_js_errors` | `getConsoleLogs` / `clearConsoleLogs` / `assertNoJSErrors` |
| `mock_network` | `mockNetwork` |

Every method maps to a server step type; any step not yet wrapped as a typed method can
still be sent via `c.execute_step({"type": "...", ...})`.
