"""Test that waitForAnimationToEnd times out when animation never stops.

A tiny local HTTP server serves a page with a perpetually rotating CSS
spinner.  Because the animation never ceases, the driver can never reach a
"two consecutive identical screenshots" steady state, so waitForAnimationToEnd
must time out and return success=False.

The server is bound on the same machine that runs the emulator and is reached
from inside the emulator via 10.0.2.2 (the Android emulator's host-loopback
alias).  This avoids any dependency on external network access, which is
unreliable in CI sandboxes.

Configuration knobs used by this test
--------------------------------------
sleep_ms=500
    Half a second between the two comparison screenshots.  A CSS spinner
    running at ~60 fps will have rotated noticeably in that window, ensuring
    the pixel diff stays consistently above the threshold.

threshold=0.0003
    Only 0.03 % of pixels may differ for the screen to be considered static.
    This is far below the diff a rotating spinner produces, so any rotation is
    detected as animated.

timeout=5000 (5 s)
    Long enough to take several comparison pairs, short enough that the test
    completes quickly.

Prerequisites:
  1. Android emulator running  (``adb devices`` shows a device)
  2. maestro-runner binary built and on PATH (or conftest will auto-start)
  3. Python deps installed:  pip install requests pytest

Run:
  pytest tests/test_wait_for_animation_never_ends.py -v
"""

import os
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

from maestro_runner import MaestroClient, commands

# Host the emulator uses to reach the test machine. 10.0.2.2 is the Android
# emulator's alias for the host loopback. Override with MAESTRO_ANIMATION_HOST
# when running against a non-emulator device.
HOST = os.environ.get("MAESTRO_ANIMATION_HOST", "10.0.2.2")

# A self-contained page with a perpetually rotating spinner — animation never
# settles, so waitForAnimationToEnd must time out.
_SPINNER_HTML = """<!doctype html>
<html><head><meta charset="utf-8">
<style>
  html,body{margin:0;height:100%;background:#fff;display:flex;align-items:center;justify-content:center}
  .spinner{width:120px;height:120px;border:16px solid #eee;border-top-color:#3498db;border-radius:50%;animation:spin 1s linear infinite}
  @keyframes spin{to{transform:rotate(360deg)}}
</style></head><body><div class="spinner"></div></body></html>"""

# How long the server waits before giving up (ms)
_TIMEOUT_MS = 5_000
# Pause between the two consecutive screenshots (ms) — must be long enough for
# the spinner to advance at least one visible frame
_SLEEP_MS = 500
# Maximum pixel-diff fraction still considered "static".  Far below what a
# rotating spinner produces, so any rotation is detected as animated.
_THRESHOLD = 0.0003


class _SpinnerHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.end_headers()
        self.wfile.write(_SPINNER_HTML.encode())

    def log_message(self, *args) -> None:  # silence noisy request logs
        pass


def _start_server() -> HTTPServer:
    server = HTTPServer(("0.0.0.0", 0), _SpinnerHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


def test_wait_for_animation_times_out_on_infinite_spinner(
    client: MaestroClient,
) -> None:
    """
    Serve a page with a continuously running spinner, open it in the emulator
    browser, then call waitForAnimationToEnd.  Because the animation never
    stops the driver must time out and return success=False.
    """
    server = _start_server()
    port = server.server_address[1]
    spinner_url = f"http://{HOST}:{port}/"
    try:
        open_result = client.open_link(spinner_url)
        assert open_result.success is True, (
            f"Failed to open spinner URL: {open_result.message}"
        )

        # Give the browser time to load the page and start rendering the spinner
        time.sleep(5)

        # Swipe up a bit to ensure the spinner is in view; assert it worked
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
    finally:
        server.shutdown()
        server.server_close()
