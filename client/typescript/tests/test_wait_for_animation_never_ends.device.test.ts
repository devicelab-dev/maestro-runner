/**
 * Test that waitForAnimationToEnd times out when the animation never stops.
 *
 * The emulator's camera preview is a live (continuously changing) feed, so the
 * driver can never reach a "two consecutive identical screenshots" steady
 * state and waitForAnimationToEnd must time out and return success=false.
 *
 * Using the camera avoids any dependency on external network access or a
 * browser, both of which are unreliable inside CI sandboxes: the emulator often
 * cannot reach the runner's loopback (so a locally served spinner page never
 * loads), and Chrome will not render a local file:// from an intent. The
 * camera preview is always available on the emulator and never settles.
 *
 * Prerequisites:
 *   1. Android emulator OR iOS simulator running
 *   2. Node deps installed (from client/typescript): npm install
 *   3. (Optional) Start maestro-runner server manually:
 *        ./maestro-runner --platform android server --port 9999
 *        ./maestro-runner --platform ios --device <UDID> server --port 9999
 *      If not running, the server is auto-started by the test setup.
 *
 * Override via env vars:
 *   MAESTRO_SERVER_URL   (default: http://localhost:9999)
 *   MAESTRO_PLATFORM     (default: android)
 *   MAESTRO_DEVICE_ID    (recommended for explicit iOS simulator targeting)
 *
 * Run (Android):
 *   cd client/typescript && npx jest tests/test_wait_for_animation_never_ends.device.test.ts --runInBand
 *
 * Run (iOS):
 *   cd client/typescript && MAESTRO_PLATFORM=ios MAESTRO_DEVICE_ID=<UDID> \
 *     npx jest tests/test_wait_for_animation_never_ends.device.test.ts --runInBand
 */

import { execSync } from "child_process";
import { afterAll, describe, expect, it } from "@jest/globals";

import { getClient, teardown } from "./setup";

// Pause between the two consecutive screenshots (ms) — must be long enough for
// the preview to advance at least one visible frame
const SLEEP_MS = 500;

// Maximum pixel-diff fraction still considered "static". Far below what a live
// camera preview produces, so any change is detected as animated.
const THRESHOLD = 0.0003;

function launchCamera(): void {
  // Open the camera capture intent. The emulated camera shows a live preview
  // that never settles, which is exactly what we need.
  execSync("adb shell am start -a android.media.action.IMAGE_CAPTURE", {
    stdio: "ignore",
  });
}

afterAll(async () => {
  await teardown();
});

describe("WaitForAnimationToEnd", () => {
  it(
    "should time out on an infinite (camera preview) animation",
    async () => {
      const client = await getClient();

      launchCamera();

      // Give the camera time to start rendering the live preview
      await new Promise((resolve) => setTimeout(resolve, 5_000));

      // Swipe up a bit; assert it worked
      const swipeResult = await client.swipe("up", 400);
      expect(swipeResult.success).toBe(true);

      await new Promise((resolve) => setTimeout(resolve, 1_000));

      await expect(
        client.waitForAnimationToEnd(SLEEP_MS, THRESHOLD, "wait_for_animation_on_camera_preview"),
      ).rejects.toThrow("Timed out");
    },
    // The server default timeout is 15 s; allow 60 s for camera start + swipe + animation check
    60_000,
  );
});
