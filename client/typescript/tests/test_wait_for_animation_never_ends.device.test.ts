/**
 * Test that waitForAnimationToEnd times out when the animation never stops.
 *
 * A tiny local HTTP server serves a page with a perpetually rotating CSS
 * spinner. Because the animation never ceases, the driver cannot reach a
 * "two consecutive identical screenshots" steady state, so
 * waitForAnimationToEnd must time out and return success=false.
 *
 * The server is bound on the same machine that runs the emulator and is
 * reached from inside the emulator via 10.0.2.2 (the Android emulator's
 * host-loopback alias). This avoids any dependency on external network
 * access, which is unreliable in CI sandboxes.
 *
 * Configuration:
 *   sleepMs=500     — half a second between comparison screenshots; a CSS
 *                     spinner running at ~60 fps will have rotated noticeably.
 *   threshold=0.0003 — only 0.03% pixel diff allowed; far below what a rotating
 *                      spinner produces, so any rotation is detected as animated.
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
 *   MAESTRO_ANIMATION_HOST (default: 10.0.2.2 — host loopback from the emulator)
 *
 * Run (Android):
 *   cd client/typescript && npx jest tests/test_wait_for_animation_never_ends.device.test.ts --runInBand
 *
 * Run (iOS):
 *   cd client/typescript && MAESTRO_PLATFORM=ios MAESTRO_DEVICE_ID=<UDID> \
 *     npx jest tests/test_wait_for_animation_never_ends.device.test.ts --runInBand
 */

import http from "http";
import { afterAll, beforeAll, describe, expect, it } from "@jest/globals";

import { getClient, teardown } from "./setup";

// Host the emulator uses to reach the test machine. 10.0.2.2 is the Android
// emulator's alias for the host loopback.
const HOST = process.env.MAESTRO_ANIMATION_HOST ?? "10.0.2.2";

// A self-contained page with a perpetually rotating spinner — animation never
// settles, so waitForAnimationToEnd must time out.
const SPINNER_HTML = `<!doctype html>
<html><head><meta charset="utf-8">
<style>
  html,body{margin:0;height:100%;background:#fff;display:flex;align-items:center;justify-content:center}
  .spinner{width:120px;height:120px;border:16px solid #eee;border-top-color:#3498db;border-radius:50%;animation:spin 1s linear infinite}
  @keyframes spin{to{transform:rotate(360deg)}}
</style></head><body><div class="spinner"></div></body></html>`;

// Pause between the two consecutive screenshots (ms) — must be long enough for
// the spinner to advance at least one visible frame
const SLEEP_MS = 500;

// Maximum pixel-diff fraction still considered "static". Far below what a
// rotating spinner produces, so any rotation is detected as animated.
const THRESHOLD = 0.0003;

let server: http.Server;
let port = 0;

beforeAll((done) => {
  server = http.createServer((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(SPINNER_HTML);
  });
  server.listen(0, "0.0.0.0", () => {
    port = (server.address() as { port: number }).port;
    done();
  });
});

afterAll(async () => {
  await teardown();
  // The emulator browser keeps a keep-alive connection to the spinner server,
  // which makes server.close() block until that connection drains. Force it
  // closed so the hook doesn't hang (same class of bug as setup.ts teardown).
  const s = server as unknown as { closeAllConnections?: () => void };
  s.closeAllConnections?.();
  await new Promise<void>((resolve) => server.close(() => resolve()));
});

describe("WaitForAnimationToEnd", () => {
  it(
    "should time out on an infinite spinning animation",
    async () => {
      const client = await getClient();
      const spinnerUrl = `http://${HOST}:${port}/`;

      const openResult = await client.openLink(spinnerUrl);
      expect(openResult.success).toBe(true);

      // Give the browser time to load the page and start rendering the spinner
      await new Promise((resolve) => setTimeout(resolve, 5_000));

      // Swipe up a bit to ensure the spinner is in view
      const swipeResult = await client.swipe("up", 400);
      expect(swipeResult.success).toBe(true);

      await new Promise((resolve) => setTimeout(resolve, 1_000));

      await expect(
        client.waitForAnimationToEnd(SLEEP_MS, THRESHOLD, "wait_for_animation_on_infinite_spinner"),
      ).rejects.toThrow("Timed out");
    },
    // The server default timeout is 15 s; allow 60 s for page load + swipe + animation check
    60_000,
  );
});
