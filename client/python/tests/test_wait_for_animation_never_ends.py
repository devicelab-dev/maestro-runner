"""Test that waitForAnimationToEnd times out when animation never stops.

A tiny offline Android app (dev.maestro.animationtest) shows a full-screen
WebView rendering a perpetually rotating CSS spinner. Because the animation
never ceases, the driver can never reach a "two consecutive identical
screenshots" steady state, so waitForAnimationToEnd must time out and return
success=False.

The WebView's CSS animation is independent of the system animator/window/
transition scales, so it keeps rotating even when the emulator runs with
`disable-animations: true` (which would suppress a native Android animation).
No network or local HTTP server is involved -- the HTML is embedded in the app.

Prerequisites:
  1. Android emulator running  (``adb devices`` shows a device)
  2. maestro-runner binary built and on PATH (or conftest will auto-start)
  3. Python deps installed:  pip install requests pytest
  4. drivers/android/animation-test-app-debug.apk installed on the emulator

Run:
  pytest tests/test_wait_for_animation_never_ends.py -v
"""

import subprocess
import time

from maestro_runner import MaestroClient, commands

# Package/activity of the offline WebView spinner test app.
_APP_PKG = "dev.maestro.animationtest"
_APP_ACTIVITY = "dev.maestro.animationtest/.MainActivity"

# Pause between the two consecutive screenshots (ms) -- must be long enough for
# the spinner to advance at least one visible frame
_SLEEP_MS = 500
# Maximum pixel-diff fraction still considered "static".  Far below what a
# rotating spinner produces, so any rotation is detected as animated.
_THRESHOLD = 0.0003

# How long to wait for the app to come to the foreground before giving up (s)
_FOCUS_TIMEOUT_S = 10


def _launch_app() -> None:
    # Bring the WebView spinner app to the foreground.
    subprocess.run(
        ["adb", "shell", "am", "start", "-n", _APP_ACTIVITY],  # noqa: S607
        capture_output=True, text=True, check=True,
    )


def _wait_for_app_focused() -> None:
    # Ensure the app actually reached the foreground before we screenshot it;
    # otherwise waitForAnimationToEnd could sample a stale/previous screen.
    deadline = time.time() + _FOCUS_TIMEOUT_S
    while time.time() < deadline:
        out = subprocess.run(
            ["adb", "shell", "dumpsys", "window"],  # noqa: S607
            capture_output=True, text=True, check=True,
        ).stdout
        if _APP_PKG in out:
            return
        time.sleep(0.5)
    raise RuntimeError(
        f"{_APP_PKG} did not become the focused app within "
        f"{_FOCUS_TIMEOUT_S}s; cannot run the animation-timeout test"
    )


def test_wait_for_animation_times_out_on_infinite_spinner(
    client: MaestroClient,
) -> None:
    """
    Launch the WebView spinner app (animation never settles), then call
    waitForAnimationToEnd.  Because the animation keeps changing the driver must
    time out and return success=False.
    """
    _launch_app()
    _wait_for_app_focused()

    # Give the WebView a moment to render the first spinner frames
    time.sleep(2)

    # Swipe up a bit; assert it worked
    swipe_result = client.swipe("up", duration_ms=400)
    assert swipe_result.success is True, (
        f"Swipe failed: {swipe_result.message}"
    )
    time.sleep(1)

    # Use execute_step directly so that success=False is returned instead of
    # raising StepError -- this is what we want to assert on.
    result = client.execute_step(commands.wait_for_animation_to_end(
        sleep_ms=_SLEEP_MS,
        threshold=_THRESHOLD,
        label="wait_for_animation_on_infinite_spinner",
    ))

    assert result.success is False, (
        "Expected waitForAnimationToEnd to fail (timeout) because the spinner "
        f"never stops, but got success=True. Message: {result.message}"
    )
    assert "Timed out" in (result.message or ""), (
        f"Expected a timeout message, got: {result.message}"
    )
    print(f"  waitForAnimationToEnd timed out as expected: {result.message}")
