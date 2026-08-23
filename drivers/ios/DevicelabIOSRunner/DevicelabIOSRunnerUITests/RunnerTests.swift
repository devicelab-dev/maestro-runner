//
//  RunnerTests.swift
//  AgentDeviceRunnerUITests
//
//  Created by Michał Pierzchała on 30/01/2026.
//

import XCTest
import Network
#if canImport(UIKit)
import UIKit
typealias RunnerImage = UIImage
#elseif canImport(AppKit)
import AppKit
typealias RunnerImage = NSImage
#endif

final class RunnerTests: XCTestCase {
  enum RunnerErrorDomain {
    static let general = "AgentDeviceRunner"
    static let exception = "AgentDeviceRunner.NSException"
  }

  enum RunnerErrorCode {
    static let noResponseFromMainThread = 1
    static let commandReturnedNoResponse = 2
    static let mainThreadExecutionTimedOut = 3
    static let objcException = 1
  }

  static let springboardBundleId = "com.apple.springboard"
  static let defaultRecordingFps: Int32 = 15
  static let maxListenerRetries = 5
  static let listenerRetryDelay: TimeInterval = 1.0
  var listener: NWListener?
  var listenerRetriesRemaining = RunnerTests.maxListenerRetries
  var doneExpectation: XCTestExpectation?
  let app = XCUIApplication()
  lazy var springboard = XCUIApplication(bundleIdentifier: Self.springboardBundleId)
  var currentApp: XCUIApplication?
  var currentBundleId: String?
  let maxRequestBytes = 2 * 1024 * 1024
  let maxSnapshotElements = 600
  let fastSnapshotLimit = 300
  let mainThreadExecutionTimeout: TimeInterval = 30
  let appExistenceTimeout: TimeInterval = 30
  let retryCooldown: TimeInterval = 0.2
  // Local edit (not in upstream agent-device): post-snapshot/post-activate
  // stabilization windows shortened from 0.2 / 0.25 s. Upstream value gives
  // an AI-driven runner time for layout to settle between a read and the
  // next interaction; maestro-runner's snapshot is part of selector
  // resolution that's immediately followed by an interaction at a known
  // coordinate, so a 50ms window is enough to absorb any in-flight UIKit
  // layout pass without paying for hypothetical agent latency.
  let postSnapshotInteractionDelay: TimeInterval = 0.05
  let firstInteractionAfterActivateDelay: TimeInterval = 0.1
  let scrollInteractionIdleTimeoutDefault: TimeInterval = 1.0
  let tvRemoteDoublePressDelayDefault: TimeInterval = 0.0
  let minRecordingFps = 1
  let maxRecordingFps = 120
  let minRecordingQuality = 5
  let maxRecordingQuality = 10
  var needsPostSnapshotInteractionDelay = false
  var needsFirstInteractionDelay = false
  // Local extension: cache the most recent idleCheck screenshot so
  // subsequent calls in the same wait loop only need ONE fresh capture.
  var lastIdleScreenshot: (image: RunnerImage, capturedAt: Date)?
  static let idleScreenshotCacheTTL: TimeInterval = 1.0
  var activeRecording: ScreenRecorder?
  let interactiveTypes: Set<XCUIElement.ElementType> = [
    .button,
    .cell,
    .checkBox,
    .collectionView,
    .link,
    .menuItem,
    .picker,
    .searchField,
    .segmentedControl,
    .slider,
    .stepper,
    .switch,
    .tabBar,
    .textField,
    .secureTextField,
    .textView
  ]
  // Keep blocker actions narrow to avoid false positives from generic hittable containers.
  let actionableTypes: Set<XCUIElement.ElementType> = [
    .button,
    .cell,
    .link,
    .menuItem,
    .checkBox,
    .switch
  ]

  // MARK: - XCTest Entry

  override func setUp() {
    continueAfterFailure = true
    // Override XCTest's clipped snapshot request params so element trees are
    // complete on deep React Native hierarchies (matches WDA / devicekit-ios).
    DLApplyCompleteSnapshotParams()
  }

  @MainActor
  func testCommand() throws {
    doneExpectation = expectation(description: "agent-device command handled")
    NSLog("AGENT_DEVICE_RUNNER_HEADLESS_STARTUP=1")
    let queue = DispatchQueue(label: "agent-device.runner")
    let desiredPort = RunnerEnv.resolvePort()
    NSLog("AGENT_DEVICE_RUNNER_DESIRED_PORT=%d", desiredPort)
    startListener(desiredPort: desiredPort, queue: queue)

    guard let expectation = doneExpectation else {
      XCTFail("runner expectation was not initialized")
      return
    }
    NSLog("AGENT_DEVICE_RUNNER_WAITING")
    let result = XCTWaiter.wait(for: [expectation], timeout: 24 * 60 * 60)
    NSLog("AGENT_DEVICE_RUNNER_WAIT_RESULT=%@", String(describing: result))
    if result != .completed {
      XCTFail("runner wait ended with \(result)")
    }
  }

  private func startListener(desiredPort: UInt16, queue: DispatchQueue) {
    do {
      let newListener = try makeRunnerListener(desiredPort: desiredPort)
      listener = newListener
      newListener.stateUpdateHandler = { [weak self] state in
        switch state {
        case .ready:
          NSLog("AGENT_DEVICE_RUNNER_LISTENER_READY")
          // A working listener proves recovery — restore the full retry
          // budget so an occasional network-stack hiccup hours apart
          // never accumulates into an exit.
          self?.listenerRetriesRemaining = Self.maxListenerRetries
          if let listenerPort = self?.listener?.port {
            NSLog("AGENT_DEVICE_RUNNER_PORT=%d", listenerPort.rawValue)
          } else {
            NSLog("AGENT_DEVICE_RUNNER_PORT_NOT_SET")
          }
        case .failed(let error):
          self?.handleListenerFailure(error, desiredPort: desiredPort, queue: queue)
        default:
          break
        }
      }
      newListener.newConnectionHandler = { [weak self] conn in
        conn.start(queue: queue)
        self?.handle(connection: conn)
      }
      newListener.start(queue: queue)
    } catch {
      handleListenerFailure(error, desiredPort: desiredPort, queue: queue)
    }
  }

  // The listener failing used to end the session immediately, so any
  // transient failure in the sim's network stack (whose daemons are
  // known to crash-loop on some runtime/Xcode combinations) silently
  // killed the runner mid-session. Rebind on the same port instead —
  // the host only knows this port — and exit only once the budget is
  // exhausted, with the reason in the log for post-mortems.
  private func handleListenerFailure(_ error: Error, desiredPort: UInt16, queue: DispatchQueue) {
    NSLog(
      "AGENT_DEVICE_RUNNER_LISTENER_FAILED=%@ retriesLeft=%d",
      String(describing: error), listenerRetriesRemaining)
    listener?.cancel()
    listener = nil
    guard listenerRetriesRemaining > 0 else {
      NSLog("AGENT_DEVICE_RUNNER_EXIT reason=listener_failed error=%@", String(describing: error))
      doneExpectation?.fulfill()
      return
    }
    listenerRetriesRemaining -= 1
    queue.asyncAfter(deadline: .now() + Self.listenerRetryDelay) { [weak self] in
      self?.startListener(desiredPort: desiredPort, queue: queue)
    }
  }

  private func makeRunnerListener(desiredPort: UInt16) throws -> NWListener {
    if desiredPort > 0, let port = NWEndpoint.Port(rawValue: desiredPort) {
      #if os(macOS)
        let parameters = NWParameters.tcp
        parameters.allowLocalEndpointReuse = true
        parameters.requiredLocalEndpoint = .hostPort(host: "127.0.0.1", port: port)
        return try NWListener(using: parameters)
      #else
        return try NWListener(using: .tcp, on: port)
      #endif
    }
    return try NWListener(using: .tcp)
  }
}
