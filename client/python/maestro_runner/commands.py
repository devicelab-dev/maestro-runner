"""Command builders — produce Go step JSON for the REST API."""

from __future__ import annotations

from typing import Any

from maestro_runner.models import ElementSelector


def _selector_value(
    *,
    text: str | None = None,
    id: str | None = None,
    index: int | None = None,
    selector: ElementSelector | None = None,
    enabled: bool | None = None,
    checked: bool | None = None,
    focused: bool | None = None,
    selected: bool | None = None,
) -> str | dict[str, Any]:
    """Build a selector value (string for text-only, object otherwise)."""
    d: dict[str, Any] = {}
    if selector is not None:
        d.update(selector.to_dict())
    if text is not None:
        d["text"] = text
    if id is not None:
        d["id"] = id
    if index is not None:
        d["index"] = str(index)
    if enabled is not None:
        d["enabled"] = enabled
    if checked is not None:
        d["checked"] = checked
    if focused is not None:
        d["focused"] = focused
    if selected is not None:
        d["selected"] = selected
    # Compact form: text-only selector → plain string
    if list(d.keys()) == ["text"]:
        return str(d["text"])
    return d


def tap_on(
    *,
    text: str | None = None,
    id: str | None = None,
    index: int | None = None,
    selector: ElementSelector | None = None,
    long_press: bool = False,
    wait_until_visible: bool | None = None,
    retry_if_no_change: bool | None = None,
    enabled: bool | None = None,
    checked: bool | None = None,
    focused: bool | None = None,
    selected: bool | None = None,
    optional: bool = False,
    timeout: int | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "tapOn"}
    step["selector"] = _selector_value(
        text=text, id=id, index=index, selector=selector,
        enabled=enabled, checked=checked, focused=focused, selected=selected,
    )
    if long_press:
        step["longPress"] = True
    if wait_until_visible is not None:
        step["waitUntilVisible"] = wait_until_visible
    if retry_if_no_change is not None:
        step["retryTapIfNoChange"] = retry_if_no_change
    if optional:
        step["optional"] = True
    if timeout is not None:
        step["timeout"] = timeout
    if label is not None:
        step["label"] = label
    return step


def input_text(text: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "inputText", "text": text}
    if label is not None:
        step["label"] = label
    return step


def erase_text(characters: int | None = None, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "eraseText"}
    if characters is not None:
        step["charactersToErase"] = characters
    if label is not None:
        step["label"] = label
    return step


def press_key(code: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "pressKey", "key": code}
    if label is not None:
        step["label"] = label
    return step


def back(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "back"}
    if label is not None:
        step["label"] = label
    return step


def scroll(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "scroll"}
    if label is not None:
        step["label"] = label
    return step


def swipe(direction: str, *, duration_ms: int = 400, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {
        "type": "swipe",
        "direction": direction.upper(),
        "duration": duration_ms,
    }
    if label is not None:
        step["label"] = label
    return step


def assert_visible(
    *,
    text: str | None = None,
    id: str | None = None,
    selector: ElementSelector | None = None,
    timeout_ms: int | None = None,
    optional: bool = False,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "assertVisible"}
    step["selector"] = _selector_value(text=text, id=id, selector=selector)
    if timeout_ms is not None:
        step["timeout"] = timeout_ms
    if optional:
        step["optional"] = True
    if label is not None:
        step["label"] = label
    return step


def assert_not_visible(
    *,
    text: str | None = None,
    id: str | None = None,
    selector: ElementSelector | None = None,
    timeout_ms: int | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "assertNotVisible"}
    step["selector"] = _selector_value(text=text, id=id, selector=selector)
    if timeout_ms is not None:
        step["timeout"] = timeout_ms
    if label is not None:
        step["label"] = label
    return step


def launch_app(
    app_id: str,
    *,
    clear_state: bool | None = None,
    stop_app: bool | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "launchApp", "appId": app_id}
    if clear_state is not None:
        step["clearState"] = clear_state
    if stop_app is not None:
        step["stopApp"] = stop_app
    if label is not None:
        step["label"] = label
    return step


def stop_app(app_id: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "stopApp", "appId": app_id}
    if label is not None:
        step["label"] = label
    return step


def clear_state(app_id: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "clearState", "appId": app_id}
    if label is not None:
        step["label"] = label
    return step


def open_link(link: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "openLink", "link": link}
    if label is not None:
        step["label"] = label
    return step


def set_permissions(
    app_id: str,
    permissions: dict[str, str],
    *,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {
        "type": "setPermissions",
        "appId": app_id,
        "permissions": permissions,
    }
    if label is not None:
        step["label"] = label
    return step


def reset_permissions(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "resetPermissions"}
    if label is not None:
        step["label"] = label
    return step


def eval_webview_script(
    script: str,
    *,
    output: str | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "evalWebViewScript", "script": script}
    if output is not None:
        step["output"] = output
    if label is not None:
        step["label"] = label
    return step


def run_webview_script(
    file: str,
    *,
    env: dict[str, str] | None = None,
    output: str | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "runWebViewScript", "file": file}
    if env is not None:
        step["env"] = env
    if output is not None:
        step["output"] = output
    if label is not None:
        step["label"] = label
    return step


def _coord_or_selector(
    value: str | dict[str, Any] | ElementSelector,
) -> str | dict[str, Any]:
    if isinstance(value, str):
        return value
    if isinstance(value, ElementSelector):
        return value.to_dict()
    return _selector_value(**value)


# ---------------------------------------------------------------------------
# Gestures
# ---------------------------------------------------------------------------


def double_tap_on(
    *,
    text: str | None = None,
    id: str | None = None,
    selector: ElementSelector | None = None,
    optional: bool = False,
    retry_tap_if_no_change: bool | None = None,
    wait_until_visible: bool | None = None,
    wait_to_settle_timeout_ms: int | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "doubleTapOn"}
    step["selector"] = _selector_value(
        text=text, id=id, selector=selector,
    )
    if optional:
        step["optional"] = True
    if retry_tap_if_no_change is not None:
        step["retryTapIfNoChange"] = retry_tap_if_no_change
    if wait_until_visible is not None:
        step["waitUntilVisible"] = wait_until_visible
    if wait_to_settle_timeout_ms is not None:
        step["waitToSettleTimeoutMs"] = wait_to_settle_timeout_ms
    if label is not None:
        step["label"] = label
    return step


def long_press_on(
    *,
    text: str | None = None,
    id: str | None = None,
    selector: ElementSelector | None = None,
    duration_ms: int | None = None,
    optional: bool = False,
    retry_tap_if_no_change: bool | None = None,
    wait_until_visible: bool | None = None,
    wait_to_settle_timeout_ms: int | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "longPressOn"}
    step["selector"] = _selector_value(text=text, id=id, selector=selector)
    if duration_ms is not None:
        step["duration"] = duration_ms
    if optional:
        step["optional"] = True
    if retry_tap_if_no_change is not None:
        step["retryTapIfNoChange"] = retry_tap_if_no_change
    if wait_until_visible is not None:
        step["waitUntilVisible"] = wait_until_visible
    if wait_to_settle_timeout_ms is not None:
        step["waitToSettleTimeoutMs"] = wait_to_settle_timeout_ms
    if label is not None:
        step["label"] = label
    return step


def drag_and_drop(
    *,
    from_: str | dict[str, Any] | ElementSelector,
    to: str | dict[str, Any] | ElementSelector,
    hold_duration: int | None = None,
    duration: int | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "dragAndDrop"}
    step["from"] = _coord_or_selector(from_)
    step["to"] = _coord_or_selector(to)
    if hold_duration is not None:
        step["holdDuration"] = hold_duration
    if duration is not None:
        step["duration"] = duration
    if label is not None:
        step["label"] = label
    return step


def scroll_until_visible(
    *,
    element: str | dict[str, Any] | ElementSelector,
    from_: str | dict[str, Any] | ElementSelector | None = None,
    direction: str | None = None,
    max_scrolls: int | None = None,
    speed: int | None = None,
    visibility_percentage: int | None = None,
    center_element: bool | None = None,
    wait_to_settle_timeout_ms: int | None = None,
    optional: bool = False,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "scrollUntilVisible"}
    step["element"] = _coord_or_selector(element)
    if from_ is not None:
        step["from"] = _coord_or_selector(from_)
    if direction is not None:
        step["direction"] = direction
    if max_scrolls is not None:
        step["maxScrolls"] = max_scrolls
    if speed is not None:
        step["speed"] = speed
    if visibility_percentage is not None:
        step["visibilityPercentage"] = visibility_percentage
    if center_element is not None:
        step["centerElement"] = center_element
    if wait_to_settle_timeout_ms is not None:
        step["waitToSettleTimeoutMs"] = wait_to_settle_timeout_ms
    if optional:
        step["optional"] = True
    if label is not None:
        step["label"] = label
    return step


# ---------------------------------------------------------------------------
# Assertions & media
# ---------------------------------------------------------------------------


def _crop_on(value: str | dict[str, Any] | ElementSelector) -> str | dict[str, Any]:
    if isinstance(value, str):
        return value
    if isinstance(value, ElementSelector):
        return value.to_dict()
    return _selector_value(**value)


def assert_screenshot(
    *,
    path: str | None = None,
    crop_on: str | dict[str, Any] | ElementSelector | None = None,
    threshold_percentage: float | None = None,
    optional: bool = False,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "assertScreenshot"}
    if path is not None:
        step["path"] = path
    if crop_on is not None:
        step["cropOn"] = _crop_on(crop_on)
    if threshold_percentage is not None:
        step["thresholdPercentage"] = threshold_percentage
    if optional:
        step["optional"] = True
    if label is not None:
        step["label"] = label
    return step


def take_screenshot(
    *,
    path: str | None = None,
    crop_on: str | dict[str, Any] | ElementSelector | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "takeScreenshot"}
    if path is not None:
        step["path"] = path
    if crop_on is not None:
        step["cropOn"] = _crop_on(crop_on)
    if label is not None:
        step["label"] = label
    return step


def copy_text_from(
    *,
    text: str | None = None,
    id: str | None = None,
    selector: ElementSelector | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "copyTextFrom"}
    step["selector"] = _selector_value(text=text, id=id, selector=selector)
    if label is not None:
        step["label"] = label
    return step


def paste_text(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "pasteText"}
    if label is not None:
        step["label"] = label
    return step


def set_clipboard(text: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "setClipboard", "text": text}
    if label is not None:
        step["label"] = label
    return step


# ---------------------------------------------------------------------------
# AI & scripting
# ---------------------------------------------------------------------------


def assert_with_ai(assertion: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "assertWithAI", "assertion": assertion}
    if label is not None:
        step["label"] = label
    return step


def eval_script(script: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "evalScript", "script": script}
    if label is not None:
        step["label"] = label
    return step


def run_script(
    *,
    script: str | None = None,
    file: str | None = None,
    env: dict[str, str] | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "runScript"}
    if script is not None:
        step["script"] = script
    if file is not None:
        step["file"] = file
    if env is not None:
        step["env"] = env
    if label is not None:
        step["label"] = label
    return step


def eval_browser_script(
    script: str,
    *,
    output: str | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "evalBrowserScript", "script": script}
    if output is not None:
        step["output"] = output
    if label is not None:
        step["label"] = label
    return step


# ---------------------------------------------------------------------------
# Device control
# ---------------------------------------------------------------------------


def set_location(latitude: str, longitude: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "setLocation", "latitude": latitude, "longitude": longitude}
    if label is not None:
        step["label"] = label
    return step


def set_airplane_mode(enabled: bool, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "setAirplaneMode", "enabled": enabled}
    if label is not None:
        step["label"] = label
    return step


def toggle_airplane_mode(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "toggleAirplaneMode"}
    if label is not None:
        step["label"] = label
    return step


def set_network_conditions(
    *,
    offline: bool | None = None,
    latency: float | None = None,
    download_speed: float | None = None,
    upload_speed: float | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "setNetworkConditions"}
    if offline is not None:
        step["offline"] = offline
    if latency is not None:
        step["latency"] = latency
    if download_speed is not None:
        step["downloadSpeed"] = download_speed
    if upload_speed is not None:
        step["uploadSpeed"] = upload_speed
    if label is not None:
        step["label"] = label
    return step


def open_notifications(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "openNotifications"}
    if label is not None:
        step["label"] = label
    return step


def set_dark_mode(enabled: bool, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "setDarkMode", "enabled": enabled}
    if label is not None:
        step["label"] = label
    return step


def set_orientation(orientation: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "setOrientation", "orientation": orientation}
    if label is not None:
        step["label"] = label
    return step


# ---------------------------------------------------------------------------
# Browser (web platform)
# ---------------------------------------------------------------------------


def open_browser(url: str | None = None, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "openBrowser"}
    if url is not None:
        step["url"] = url
    if label is not None:
        step["label"] = label
    return step


def switch_tab(
    *,
    tab_label: str | None = None,
    index: int | None = None,
    url: str | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "switchTab"}
    if tab_label is not None:
        step["tabLabel"] = tab_label
    if index is not None:
        step["index"] = index
    if url is not None:
        step["url"] = url
    if label is not None:
        step["label"] = label
    return step


def close_tab(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "closeTab"}
    if label is not None:
        step["label"] = label
    return step


def get_console_logs(output: str, *, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "getConsoleLogs", "output": output}
    if label is not None:
        step["label"] = label
    return step


def clear_console_logs(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "clearConsoleLogs"}
    if label is not None:
        step["label"] = label
    return step


def assert_no_js_errors(*, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "assertNoJSErrors"}
    if label is not None:
        step["label"] = label
    return step


def mock_network(
    *,
    url: str,
    method: str | None = None,
    response: dict[str, Any],
    label: str | None = None,
) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "mockNetwork", "url": url}
    if method is not None:
        step["method"] = method
    resp: dict[str, Any] = {}
    if response.get("status") is not None:
        resp["status"] = response["status"]
    if response.get("headers") is not None:
        resp["headers"] = response["headers"]
    if response.get("body") is not None:
        resp["body"] = response["body"]
    step["response"] = resp
    if label is not None:
        step["label"] = label
    return step


def hide_keyboard(*, strategy: str | None = None, label: str | None = None) -> dict[str, Any]:
    step: dict[str, Any] = {"type": "hideKeyboard"}
    if strategy is not None:
        step["strategy"] = strategy
    if label is not None:
        step["label"] = label
    return step


def wait_for_animation_to_end(
    *,
    sleep_ms: int | None = None,
    threshold: float | None = None,
    label: str | None = None,
) -> dict[str, Any]:
    """Build a waitForAnimationToEnd step.

    Args:
        sleep_ms:  Milliseconds to pause between the two comparison screenshots.
                   Longer values catch slow-moving animations.  Defaults to 200 ms
                   on the server side.
        threshold: Maximum pixel-diff percentage (0.0-1.0) still considered static.
                   Lower is stricter.  Defaults to 0.005 (0.5 %) on the server side.
        label:     Optional step label shown in reports.
    """
    step: dict[str, Any] = {"type": "waitForAnimationToEnd"}
    if sleep_ms is not None:
        step["sleepMs"] = sleep_ms
    if threshold is not None:
        step["threshold"] = threshold
    if label is not None:
        step["label"] = label
    return step
