"""Test that waitForAnimationToEnd times out when animation never stops.

The emulator's camera preview is a live (continuously changing) feed, so the
driver can never reach a "two consecutive identical screenshots" steady state
and waitForAnimationToEnd must time out and return success=False.

Using the camera avoids any dependency on external network access or a browser,
both of which are unreliable inside CI sandboxes: the emulator often cannot
reach the runner's loopback (so a locally served spinner page never loads), and
Chrome will not render a local file:// from an intent. The camera preview is
always available on the emulator and never settles.

Prerequisites:
  1. Android emulator running  (``adb devices`` shows a device)
  2. maestro-runner binary built and on PATH (or conftest will auto-start)
  3. Python deps installed:  pip install requests pytest

Run:
  pytest tests/test_wait_for_animation_never_ends.py -v
"""

import subprocess
import time

from maestro_runner import MaestroClient, commands

# Pause between the two consecutive screenshots (ms) — must be long enough for
# the preview to advance at least one visible frame
_SLEEP_MS = 500
# Maximum pixel-diff fraction still considered "static".  Far below what a live
# camera preview produces, so any change is detected as animated.
_THRESHOLD = 0.0003


def _launch_camera() -> None:
    # Open the camera capture intent. The emulated camera shows a live preview
    # that never settles, which is exactly what we need.
    subprocess.run(
        ["adb", "shell", "am", "start", "-a", "android.media.action.IMAGE_CAPTURE"],  # noqa: S607
        capture_output=True, text=True, check=True,
    )


def test_wait_for_animation_times_out_on_infinite_spinner(
    client: MaestroClient,
) -> None:
    """
    Open the camera (live preview never settles), then call
    waitForAnimationToEnd.  Because the preview keeps changing the driver must
    time out and return success=False.
    """
    _launch_camera()

    # Give the camera time to start rendering the live preview
    time.sleep(5)

    # Swipe up a bit; assert it worked
    swipe_result = client.swipe("up", duration_ms=400)
    assert swipe_result.success is True, (
        f"Swipe failed: {swipe_result.message}"
    )
    time.sleep(1)

    # Use execute_step directly so that success=False is returned instead of
    # raising StepError — this is what we want to assert on.
    result = client.execute_step(commands.wait_for_animation_to_end(
        sleep_ms=_SLEEP_MS,
        threshold=_THRESHOLD,
        label="wait_for_animation_on_camera_preview",
    ))

    assert result.success is False, (
        "Expected waitForAnimationToEnd to fail (timeout) because the camera "
        f"preview never settles, but got success=True. Message: {result.message}"
    )
    assert "Timed out" in (result.message or ""), (
        f"Expected a timeout message, got: {result.message}"
    )
    print(f"  waitForAnimationToEnd timed out as expected: {result.message}")
