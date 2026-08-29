# Python Client Tutorial

The Python client wraps the maestro-runner [REST API Server](../README.md#rest-api-server)
so you can drive Android, iOS, and Web devices from pytest — with the same selectors and
assertions as YAML flows. It adds first-class parallel execution via `pytest-xdist`,
where each worker spins up its own server and targets its own device automatically.

Source: [`client/python`](../../client/python).

## 1. Prerequisites

- A built `maestro-runner` binary on your `PATH` (or set `MAESTRO_RUNNER_BIN` to its path).
- Python 3.9+.
- An emulator/simulator/device available, **or** let the tests auto-start a server.

## 2. Install

```bash
cd client/python
python3 -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
```

The package name is `maestro-runner`; import the client with:

```python
from maestro_runner import MaestroClient
```

## 3. Start the server (or let the tests do it)

Start the REST server once, in a separate terminal:

```bash
maestro-runner server --port 9999
```

> The pytest device suites can auto-start a server per worker (see Parallel mode below),
> so you only need to start it manually for your own scripts.

## 4. Your first script

The client is a context manager: pass `capabilities` to create the session immediately,
and it closes on exit.

```python
from maestro_runner import MaestroClient

with MaestroClient(
    "http://localhost:9999",
    capabilities={"platformName": "android", "appId": "com.example.app"},
) as c:
    c.launch_app("com.example.app", clear_state=True)
    c.tap(text="Login")
    c.input_text("user@example.com")
    c.input_text("secret", id="password")
    c.tap(text="Sign in")
    c.assert_visible(text="Welcome", timeout_ms=10000)

    info = c.device_info()
    print(f"Device: {info.device_name} ({info.platform} {info.os_version})")
```

If you don't pass `capabilities`, call `create_session` yourself and `close()` when done:

```python
c = MaestroClient("http://localhost:9999")
c.create_session({"platformName": "android"})
try:
    ...
finally:
    c.close()
```

## 5. Selectors

`tap`, `long_press`, `assert_visible`, `assert_not_visible`, and `element_exists` take
keyword arguments for the selector:

| Argument              | Type      | Meaning                                         |
|-----------------------|-----------|-------------------------------------------------|
| `text`                | `str`     | Match by visible text                           |
| `id`                  | `str`     | Match by resource id / accessibility id         |
| `index`               | `int`     | Disambiguate multiple matches                   |
| `selector`            | `object`  | A raw `ElementSelector` (`{"text": ..., ...}`)  |
| `long_press`          | `bool`    | Long-press instead of tap                       |
| `optional`            | `bool`    | Don't raise if the step fails                   |
| `timeout_ms`          | `int`     | Per-call timeout for assertions                 |
| `wait_until_visible`  | `bool`    | Wait for the element before acting              |
| `enabled`/`checked`/… | `bool`    | State filters for the matched element           |

Example:

```python
c.tap(id="com.example.app:id/submit", enabled=True)
c.assert_visible(text="Saved", timeout_ms=10000)
```

## 6. Page Object Model

Encapsulate screens behind page objects for maintainable suites:

```python
# pages.py
class ContactListPage:
    def __init__(self, client):
        self.client = client

    def launch(self, clear=False):
        self.client.launch_app("com.example.app", clear_state=clear)

    def open_create_contact(self):
        self.client.tap(text="Add contact")
        return ContactEditPage(self.client)

    def assert_contact_visible(self, name):
        self.client.assert_visible(text=name)


class ContactEditPage:
    def __init__(self, client):
        self.client = client

    def set_first_name(self, v):
        self.client.input_text(v, id="first_name")

    def set_last_name(self, v):
        self.client.input_text(v, id="last_name")

    def set_phone(self, v):
        self.client.input_text(v, id="phone")

    def save(self):
        self.client.tap(text="Save")
```

```python
# test_add_contact.py
from maestro_runner import MaestroClient
from pages import ContactListPage, ContactEditPage

def test_add_contact():
    with MaestroClient("http://localhost:9999",
                       capabilities={"platformName": "android"}) as c:
        lst = ContactListPage(c)
        lst.launch(clear=True)
        edit = lst.open_create_contact()
        edit.set_first_name("Alice")
        edit.set_last_name("Tester")
        edit.set_phone("5550100")
        edit.save()
        lst.assert_contact_visible("Alice Tester")
```

## 7. Advanced calls

```python
# Self-healing: try each selector in order, stop at the first match.
c.tap_first_match(
    [{"text": "Continue"}, {"id": "continue_button"}, {"text": "Next"}],
    step="dismiss onboarding",
)

# Tap a coordinate.
c.tap_on_point("50%,50%", long_press=True)

# Swipe on an element or direction.
c.swipe_on(text="Feed", direction="UP", duration_ms=500)
c.swipe("DOWN", duration_ms=400)

# Screenshot / view hierarchy as raw data.
png = c.screenshot()        # bytes
xml = c.view_hierarchy()    # str

# Device metadata.
info = c.device_info()
```

## 8. Running the suite

### Sequential (single device)

```bash
pytest tests/test_add_contact.py tests/test_contact_persists.py -v
```

### Parallel (multiple devices)

Run across several Android emulators with [pytest-xdist](https://pypi.org/project/pytest-xdist/).
Each worker auto-starts its own `maestro-runner` server on a unique port and targets a
specific device.

**Prerequisites:**

1. Two or more emulators running (`adb devices` lists them).
2. `pytest-xdist` installed (included in the `[dev]` extra).

**Run:**

```bash
# 2 emulators in parallel
pytest tests/test_add_contact.py tests/test_contact_persists.py -n 2 -v
```

Worker `gw0` gets the first device (e.g. `emulator-5554`) on port `9999`, `gw1` gets the
next device (e.g. `emulator-5556`) on port `10000`, and so on.

## 9. Environment variables

| Variable             | Default                  | Description                        |
|----------------------|--------------------------|------------------------------------|
| `MAESTRO_SERVER_URL` | `http://localhost:9999`  | Base URL (port used as starting port in parallel mode) |
| `MAESTRO_PLATFORM`   | `android`                | Target platform                    |
| `MAESTRO_RUNNER_BIN` | `../../maestro-runner`   | Path to the maestro-runner binary  |

## Full API reference

See [`client/python/maestro_runner/client.py`](../../client/python/maestro_runner/client.py)
for the complete `MaestroClient` API, and the client
[`README.md`](../../client/python/README.md) for the install and run summary.
