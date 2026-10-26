/**
 * Copyright (c) 2015-present, Facebook, Inc.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

import XCTest

/// Covers the app lifecycle routes (launch/activate/terminate/state/list). FBSession's
/// implementations of these are themselves thin wrappers over plain XCTest XCUIApplication
/// APIs (see FBSession.m), so this calls `.launch()`/`.activate()`/`.terminate()`/`.state`
/// directly instead of going through the HTTP server; only the apps-list route has a real WDA
/// category behind it (`+fb_activeAppsInfo`).
final class WDAAppLifecycleIntegrationTests: WDAWatchInProcessTestCase {
  func testAppsListIncludesTheAppUnderTest() {
    let apps = XCUIApplication.fb_activeAppsInfo()
    XCTAssertTrue(apps.contains { $0["bundleId"] as? String == WDAWatchInProcessTestCase.integrationAppBundleId })
  }

  func testTerminateThenRelaunchLifecycle() {
    app.terminate()
    XCTAssertEqual(app.state, .notRunning)

    app.launch()
    XCTAssertEqual(app.state, .runningForeground)

    // The app relaunched fresh, so its accessibility tree should be back too.
    XCTAssertTrue(app.buttons["tapMeButton"].waitForExistence(timeout: 15))
  }

  func testActivateBringsTheAppToTheForeground() {
    app.activate()
    XCTAssertEqual(app.state, .runningForeground)
  }
  func testHomeAndDeactivate() throws {
    try XCUIDevice.shared.fb_goToHomescreen()
    XCTAssertNotEqual(app.state, .runningForeground)
    try XCUIDevice.shared.fb_goToHomescreen()
    XCTAssertNotEqual(app.state, .runningForeground)
    for duration in [0.0, 2.0] {
      app.activate()
      try app.fb_deactivate(withDuration: duration)
      XCTAssertEqual(app.state, .runningForeground)
    }
  }

}
