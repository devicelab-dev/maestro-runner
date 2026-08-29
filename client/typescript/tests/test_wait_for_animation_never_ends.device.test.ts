/**
 * Test that waitForAnimationToEnd times out when the animation never stops.
 *
 * A tiny offline Android app (dev.maestro.animationtest) shows a full-screen
 * WebView rendering a perpetually rotating CSS spinner. Because the animation
 * never ceases, the driver cannot reach a "two consecutive identical
 * screenshots" steady state, so waitForAnimationToEnd must time out and return
 * success=false.
 *
 * The WebView's CSS animation is independent of the system animator/window/
 * transition scales, so it keeps rotating even when the emulator runs with
 * `disable-animations: true` (which would suppress a native Android animation).
 * No network or local HTTP server is involved — the HTML is embedded in the app.
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

// Package/activity of the offline WebView spinner test app.
const APP_PKG = "dev.maestro.animationtest";
const APP_ACTIVITY = "dev.maestro.animationtest/.MainActivity";

// Pause between the two consecutive screenshots (ms) — must be long enough for
// the spinner to advance at least one visible frame
const SLEEP_MS = 500;

// Maximum pixel-diff fraction still considered "static". Far below what a
// rotating spinner produces, so any rotation is detected as animated.
const THRESHOLD = 0.0003;

// How long to wait for the app to come to the foreground before giving up (ms)
const FOCUS_TIMEOUT_MS = 10_000;

function launchApp(): void {
  // Bring the WebView spinner app to the foreground.
  execSync(`adb shell am start -n ${APP_ACTIVITY}`, { stdio: "ignore" });
}

function waitForAppFocused(): void {
  // Ensure the app actually reached the foreground before we screenshot it;
  // otherwise waitForAnimationToEnd could sample a stale/previous screen.
  const deadline = Date.now() + FOCUS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    const out = execSync("adb shell dumpsys window", { encoding: "utf-8" });
    if (out.includes(APP_PKG)) {
      return;
    }
    execSync("sleep 0.5", { stdio: "ignore" });
  }
  throw new Error(
    `${APP_PKG} did not become the focused app within ${FOCUS_TIMEOUT_MS}ms; ` +
      "cannot run the animation-timeout test",
  );
}

afterAll(async () => {
  await teardown();
});

describe("WaitForAnimationToEnd", () => {
  it(
    "should time out on an infinite (WebView spinner) animation",
    async () => {
      const client = await getClient();

      launchApp();
      waitForAppFocused();

      // Give the WebView a moment to render the first spinner frames
      await new Promise((resolve) => setTimeout(resolve, 2_000));

      // Swipe up a bit; assert it worked
      const swipeResult = await client.swipe("up", 400);
      expect(swipeResult.success).toBe(true);

      await new Promise((resolve) => setTimeout(resolve, 1_000));

      await expect(
        client.waitForAnimationToEnd(SLEEP_MS, THRESHOLD, "wait_for_animation_on_infinite_spinner"),
      ).rejects.toThrow("Timed out");
    },
    // The server default timeout is 15 s; allow 60 s for app launch + swipe + animation check
    60_000,
  );
});
