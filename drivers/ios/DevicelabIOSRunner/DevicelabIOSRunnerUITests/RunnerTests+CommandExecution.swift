import XCTest

extension RunnerTests {
  // MARK: - Main Thread Dispatch

  private func currentUptimeMs() -> Double {
    ProcessInfo.processInfo.systemUptime * 1000
  }

  func measureGesture(_ action: () -> Void) -> (gestureStartUptimeMs: Double, gestureEndUptimeMs: Double) {
    let gestureStartUptimeMs = currentUptimeMs()
    action()
    return (gestureStartUptimeMs, currentUptimeMs())
  }

  func unsupportedResponse(for outcome: RunnerInteractionOutcome) -> Response? {
    switch outcome {
    case .performed:
      return nil
    case .unsupported(let message):
      return Response(
        ok: false,
        error: ErrorPayload(code: "UNSUPPORTED_OPERATION", message: message)
      )
    }
  }

  func execute(command: Command) throws -> Response {
    if Thread.isMainThread {
      return try executeOnMainSafely(command: command)
    }
    var result: Result<Response, Error>?
    let semaphore = DispatchSemaphore(value: 0)
    DispatchQueue.main.async {
      do {
        result = .success(try self.executeOnMainSafely(command: command))
      } catch {
        result = .failure(error)
      }
      semaphore.signal()
    }
    let waitResult = semaphore.wait(timeout: .now() + mainThreadExecutionTimeout)
    if waitResult == .timedOut {
      // The main queue work may still be running; we stop waiting and report timeout.
      throw NSError(
        domain: RunnerErrorDomain.general,
        code: RunnerErrorCode.mainThreadExecutionTimedOut,
        userInfo: [NSLocalizedDescriptionKey: "main thread execution timed out"]
      )
    }
    switch result {
    case .success(let response):
      return response
    case .failure(let error):
      throw error
    case .none:
      throw NSError(
        domain: RunnerErrorDomain.general,
        code: RunnerErrorCode.noResponseFromMainThread,
        userInfo: [NSLocalizedDescriptionKey: "no response from main thread"]
      )
    }
  }

  // MARK: - Command Handling

  private func executeOnMainSafely(command: Command) throws -> Response {
    var hasRetried = false
    while true {
      var response: Response?
      var swiftError: Error?
      let exceptionMessage = RunnerObjCExceptionCatcher.catchException({
        do {
          response = try self.executeOnMain(command: command)
        } catch {
          swiftError = error
        }
      })

      if let exceptionMessage {
        currentApp = nil
        currentBundleId = nil
        if !hasRetried, shouldRetryException(command, message: exceptionMessage) {
          NSLog(
            "AGENT_DEVICE_RUNNER_RETRY command=%@ reason=objc_exception",
            command.command.rawValue
          )
          hasRetried = true
          sleepFor(retryCooldown)
          continue
        }
        throw NSError(
          domain: RunnerErrorDomain.exception,
          code: RunnerErrorCode.objcException,
          userInfo: [NSLocalizedDescriptionKey: exceptionMessage]
        )
      }
      if let swiftError {
        throw swiftError
      }
      guard let response else {
        throw NSError(
          domain: RunnerErrorDomain.general,
          code: RunnerErrorCode.commandReturnedNoResponse,
          userInfo: [NSLocalizedDescriptionKey: "command returned no response"]
        )
      }
      if !hasRetried, shouldRetryCommand(command), shouldRetryResponse(response) {
        NSLog(
          "AGENT_DEVICE_RUNNER_RETRY command=%@ reason=response_unavailable",
          command.command.rawValue
        )
        hasRetried = true
        currentApp = nil
        currentBundleId = nil
        sleepFor(retryCooldown)
        continue
      }
      return response
    }
  }

  private func executeOnMain(command: Command) throws -> Response {
    var activeApp = currentApp ?? app
    if !isRunnerLifecycleCommand(command.command) {
      let normalizedBundleId = command.appBundleId?
        .trimmingCharacters(in: .whitespacesAndNewlines)
      let requestedBundleId = (normalizedBundleId?.isEmpty == true) ? nil : normalizedBundleId
      if let bundleId = requestedBundleId {
        if currentBundleId != bundleId || currentApp == nil {
          _ = activateTarget(bundleId: bundleId, reason: "bundle_changed")
        }
      } else {
        // Do not reuse stale bundle targets when the caller does not explicitly request one.
        currentApp = nil
        currentBundleId = nil
      }

      activeApp = currentApp ?? app
      if let bundleId = requestedBundleId, targetNeedsActivation(activeApp) {
        activeApp = activateTarget(bundleId: bundleId, reason: "stale_target")
      } else if requestedBundleId == nil, targetNeedsActivation(activeApp) {
        ensureRunnerHostAppActive(reason: "missing_app_bundle")
        activeApp = app
      }

      let skipExistenceWait = canUseFastForegroundAppGuard(
        activeApp: activeApp,
        requestedBundleId: requestedBundleId,
        command: command.command
      )
      if !skipExistenceWait && !activeApp.waitForExistence(timeout: appExistenceTimeout) {
        if let bundleId = requestedBundleId {
          activeApp = activateTarget(bundleId: bundleId, reason: "missing_after_wait")
          guard activeApp.waitForExistence(timeout: appExistenceTimeout) else {
            return Response(ok: false, error: ErrorPayload(message: "app '\(bundleId)' is not available"))
          }
        } else {
          return Response(ok: false, error: ErrorPayload(message: "runner app is not available"))
        }
      }

      if isInteractionCommand(command.command) {
        if let bundleId = requestedBundleId, activeApp.state != .runningForeground {
          activeApp = activateTarget(bundleId: bundleId, reason: "interaction_foreground_guard")
        } else if requestedBundleId == nil, activeApp.state != .runningForeground {
          ensureRunnerHostAppActive(reason: "interaction_missing_app_bundle")
          activeApp = app
        }
        let skipInteractionExistenceWait = canUseFastForegroundAppGuard(
          activeApp: activeApp,
          requestedBundleId: requestedBundleId,
          command: command.command
        )
        if !skipInteractionExistenceWait && !activeApp.waitForExistence(timeout: 2) {
          if let bundleId = requestedBundleId {
            return Response(ok: false, error: ErrorPayload(message: "app '\(bundleId)' is not available"))
          }
          return Response(ok: false, error: ErrorPayload(message: "runner app is not available"))
        }
        applyInteractionStabilizationIfNeeded()
      }
    }

    switch command.command {
    case .shutdown:
      stopRecordingIfNeeded()
      return Response(ok: true, data: DataPayload(message: "shutdown"))
    case .idleCheck:
      // Local extension: take two consecutive screenshots in-process,
      // diff them on the runner side, return just the fraction of
      // differing pixels. Reuses the previous iteration's screenshot
      // when it's still fresh (within idleScreenshotCacheTTL) — that
      // halves the screenshot cost for waitForAnimationToEnd loops that
      // need multiple iterations.
      let now = Date()
      let prev: RunnerImage
      if let cached = lastIdleScreenshot,
         now.timeIntervalSince(cached.capturedAt) < Self.idleScreenshotCacheTTL {
        prev = cached.image
      } else {
        prev = XCUIScreen.main.screenshot().image
      }
      let curImg = XCUIScreen.main.screenshot().image
      lastIdleScreenshot = (image: curImg, capturedAt: Date())
      let diff = computePixelDiffFraction(prev, curImg)
      return Response(ok: true, data: DataPayload(diffFraction: diff))
    case .awaitIdle:
      // Local extension: runner-side wait loop. Compares consecutive
      // XCUI accessibility-tree snapshots (NOT screenshots) and returns
      // when two adjacent samples produce the same tree signature.
      //
      // Why tree-diff over pixel-diff:
      //   - 2-4× faster per iteration (app.snapshot() is cheaper than
      //     XCUIScreen.screenshot() + PNG decode + per-pixel compare).
      //   - Ignores visual noise that doesn't affect interactivity:
      //     cursor blink, status-bar clock tick, scroll-indicator
      //     fade, animated GIFs, video playback. Those tripped the
      //     prior 2% pixel threshold and forced us to wait the full
      //     timeout on screens that were functionally settled.
      //   - Catches the signals we actually care about for tap-readiness:
      //     element frames moving (navigation transitions, layout
      //     animations) and elements appearing/disappearing.
      //
      // Caller still passes timeoutMs (via durationMs). `scale` is
      // accepted for wire-compat but ignored — tree equality is exact.
      let timeoutMs = command.durationMs ?? 15000
      let timeout = max(0.0, timeoutMs) / 1000.0
      let started = Date()
      var prevSig: UInt64?
      do {
        prevSig = try treeSignature(of: activeApp.snapshot())
      } catch {
        prevSig = nil
      }
      while Date().timeIntervalSince(started) < timeout {
        let curSig: UInt64
        do {
          curSig = try treeSignature(of: activeApp.snapshot())
        } catch {
          // Snapshot failed (rare — usually app momentarily gone).
          // Treat as "still changing" and keep polling.
          continue
        }
        if curSig == prevSig {
          let elapsed = (Date().timeIntervalSince(started)) * 1000
          return Response(ok: true, data: DataPayload(
            message: "settled",
            currentUptimeMs: elapsed
          ))
        }
        prevSig = curSig
      }
      return Response(ok: true, data: DataPayload(
        message: "wait timed out (proceeding)",
        currentUptimeMs: timeout * 1000
      ))
    case .recordStart:
      guard
        let requestedOutPath = command.outPath?.trimmingCharacters(in: .whitespacesAndNewlines),
        !requestedOutPath.isEmpty
      else {
        return Response(ok: false, error: ErrorPayload(message: "recordStart requires outPath"))
      }
      let hasAppBundleId = !(command.appBundleId?
        .trimmingCharacters(in: .whitespacesAndNewlines)
        .isEmpty ?? true)
      guard hasAppBundleId else {
        return Response(ok: false, error: ErrorPayload(message: "recordStart requires appBundleId"))
      }
      if activeRecording != nil {
        return Response(ok: false, error: ErrorPayload(message: "recording already in progress"))
      }
      if let requestedFps = command.fps, (requestedFps < minRecordingFps || requestedFps > maxRecordingFps) {
        return Response(ok: false, error: ErrorPayload(message: "recordStart fps must be between \(minRecordingFps) and \(maxRecordingFps)"))
      }
      if let requestedQuality = command.quality, (requestedQuality < minRecordingQuality || requestedQuality > maxRecordingQuality) {
        return Response(ok: false, error: ErrorPayload(message: "recordStart quality must be between \(minRecordingQuality) and \(maxRecordingQuality)"))
      }
      do {
        let resolvedOutPath = resolveRecordingOutPath(requestedOutPath)
        let fpsLabel = command.fps.map(String.init) ?? String(RunnerTests.defaultRecordingFps)
        let qualityLabel = command.quality.map(String.init) ?? "native"
        NSLog(
          "AGENT_DEVICE_RUNNER_RECORD_START requestedOutPath=%@ resolvedOutPath=%@ fps=%@ quality=%@",
          requestedOutPath,
          resolvedOutPath,
          fpsLabel,
          qualityLabel
        )
        let recorder = ScreenRecorder(
          outputPath: resolvedOutPath,
          fps: command.fps.map { Int32($0) },
          quality: command.quality
        )
        try recorder.start { [weak self] in
          return self?.captureRunnerFrame()
        }
        activeRecording = recorder
        return Response(ok: true, data: DataPayload(message: "recording started"))
      } catch {
        activeRecording = nil
        return Response(ok: false, error: ErrorPayload(message: "failed to start recording: \(error.localizedDescription)"))
      }
    case .recordStop:
      guard let recorder = activeRecording else {
        return Response(ok: false, error: ErrorPayload(message: "no active recording"))
      }
      do {
        try recorder.stop()
        activeRecording = nil
        return Response(ok: true, data: DataPayload(message: "recording stopped"))
      } catch {
        activeRecording = nil
        return Response(ok: false, error: ErrorPayload(message: "failed to stop recording: \(error.localizedDescription)"))
      }
    case .uptime:
      return Response(
        ok: true,
        data: DataPayload(currentUptimeMs: currentUptimeMs())
      )
    case .tap:
      if let selectorKey = command.selectorKey, let selectorValue = command.selectorValue {
        let match = findElement(app: activeApp, selectorKey: selectorKey, selectorValue: selectorValue)
        if match.isAmbiguous {
          return Response(ok: false, error: ErrorPayload(code: "AMBIGUOUS_MATCH", message: "selector matched multiple elements"))
        }
        if let element = match.element {
          let frame = element.frame
          let touchFrame = frame.isEmpty
            ? nil
            : resolvedTouchVisualizationFrame(app: activeApp, x: frame.midX, y: frame.midY)
          var outcome = RunnerInteractionOutcome.performed
          let timing = measureGesture {
            withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
              outcome = activateElement(app: activeApp, element: element, action: "tap by selector")
            }
          }
          if let response = unsupportedResponse(for: outcome) {
            return response
          }
          return Response(
            ok: true,
            data: DataPayload(
              message: "tapped",
              gestureStartUptimeMs: timing.gestureStartUptimeMs,
              gestureEndUptimeMs: timing.gestureEndUptimeMs,
              x: touchFrame?.x,
              y: touchFrame?.y,
              referenceWidth: touchFrame?.referenceWidth,
              referenceHeight: touchFrame?.referenceHeight
            )
          )
        }
        return Response(ok: false, error: ErrorPayload(code: "ELEMENT_NOT_FOUND", message: "element not found"))
      }
      if let text = command.text {
        if let element = findElement(app: activeApp, text: text) {
          var outcome = RunnerInteractionOutcome.performed
          let timing = measureGesture {
            withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
              outcome = activateElement(app: activeApp, element: element, action: "tap by text")
            }
          }
          if let response = unsupportedResponse(for: outcome) {
            return response
          }
          return Response(
            ok: true,
            data: DataPayload(
              message: "tapped",
              gestureStartUptimeMs: timing.gestureStartUptimeMs,
              gestureEndUptimeMs: timing.gestureEndUptimeMs
            )
          )
        }
        return Response(ok: false, error: ErrorPayload(message: "element not found"))
      }
      if let x = command.x, let y = command.y {
        let touchFrame = resolvedTouchVisualizationFrame(app: activeApp, x: x, y: y)
        var outcome = RunnerInteractionOutcome.performed
        let timing = measureGesture {
          withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
            outcome = tapAt(app: activeApp, x: x, y: y)
          }
        }
        if let response = unsupportedResponse(for: outcome) {
          return response
        }
        return Response(
          ok: true,
          data: DataPayload(
            message: "tapped",
            gestureStartUptimeMs: timing.gestureStartUptimeMs,
            gestureEndUptimeMs: timing.gestureEndUptimeMs,
            x: touchFrame.x,
            y: touchFrame.y,
            referenceWidth: touchFrame.referenceWidth,
            referenceHeight: touchFrame.referenceHeight
          )
        )
      }
      return Response(ok: false, error: ErrorPayload(message: "tap requires text or x/y"))
    case .tapBySelector:
      // Find-and-act path: walk the XCUI tree once, find best liberal match
      // (substring + case-insensitive + prefer-editable-inputs ranking),
      // tap at its center. On miss, return the walked snapshot so Go side
      // can fall back to its own snapshot+filter+tap.
      guard let selectorKey = command.selectorKey, let selectorValue = command.selectorValue else {
        return Response(ok: false, error: ErrorPayload(message: "tapBySelector requires selectorKey and selectorValue"))
      }
      // Alerts live in a separate UIWindow whose frames don't translate
      // to the main app's coordinate space — using tapAt(x,y) on those
      // coordinates misses the button and dismisses the alert. So when
      // an alert is up and one of its buttons matches the selector, tap
      // that XCUIElement directly (XCTest handles the window math).
      if let alertResp = tryTapAlertButton(app: activeApp, selectorKey: selectorKey, selectorValue: selectorValue) {
        return alertResp
      }
      // findLiberalMatch's loop doubles as the animation-end gate:
      // it walks the tree per iteration, computes a tree signature
      // in the same pass, and returns only after two consecutive
      // identical signatures — meaning the screen is settled and the
      // match's frame is trustworthy. No separate refresh pass needed.
      let result = findLiberalMatch(app: activeApp, selectorKey: selectorKey, selectorValue: selectorValue)
      guard let matched = result.matched else {
        // Miss — return walked nodes so Go-side fallback can re-filter.
        return Response(ok: false, data: DataPayload(nodes: result.walkedNodes))
      }
      let x = Double(matched.frame.midX)
      let y = Double(matched.frame.midY)
      let touchFrame = resolvedTouchVisualizationFrame(app: activeApp, x: x, y: y)
      var outcome = RunnerInteractionOutcome.performed
      let timing = measureGesture {
        withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
          outcome = tapAt(app: activeApp, x: x, y: y)
        }
      }
      if let response = unsupportedResponse(for: outcome) {
        return response
      }
      // Note: response returns the raw element-relative x/y, not
      // touchFrame.x/y. touchFrame applies the appFrame origin (screen-
      // absolute), but the Go side feeds these into the next inputText's
      // type command which expects element-relative coords.
      return Response(
        ok: true,
        data: DataPayload(
          message: "tapped by selector",
          gestureStartUptimeMs: timing.gestureStartUptimeMs,
          gestureEndUptimeMs: timing.gestureEndUptimeMs,
          x: x,
          y: y,
          referenceWidth: touchFrame.referenceWidth,
          referenceHeight: touchFrame.referenceHeight,
          identifier: matched.identifier.trimmingCharacters(in: .whitespacesAndNewlines)
        )
      )
    case .mouseClick:
      guard let x = command.x, let y = command.y else {
        return Response(ok: false, error: ErrorPayload(message: "mouseClick requires x and y"))
      }
      let touchFrame = resolvedTouchVisualizationFrame(app: activeApp, x: x, y: y)
      do {
        var clickError: Error?
        let timing = measureGesture {
          do {
            try mouseClickAt(app: activeApp, x: x, y: y, button: command.button ?? "primary")
          } catch {
            clickError = error
          }
        }
        if let clickError {
          throw clickError
        }
        return Response(
          ok: true,
          data: DataPayload(
            message: "clicked",
            gestureStartUptimeMs: timing.gestureStartUptimeMs,
            gestureEndUptimeMs: timing.gestureEndUptimeMs,
            x: touchFrame.x,
            y: touchFrame.y,
            referenceWidth: touchFrame.referenceWidth,
            referenceHeight: touchFrame.referenceHeight
          )
        )
      } catch {
        return Response(ok: false, error: ErrorPayload(message: error.localizedDescription))
      }
    case .tapSeries:
      guard let x = command.x, let y = command.y else {
        return Response(ok: false, error: ErrorPayload(message: "tapSeries requires x and y"))
      }
      let count = max(Int(command.count ?? 1), 1)
      let intervalMs = max(command.intervalMs ?? 0, 0)
      let doubleTap = command.doubleTap ?? false
      let touchFrame = resolvedTouchVisualizationFrame(app: activeApp, x: x, y: y)
      if doubleTap {
        var outcome = RunnerInteractionOutcome.performed
        let timing = measureGesture {
          withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
            runSeries(count: count, pauseMs: intervalMs) { _ in
              if case .performed = outcome {
                outcome = doubleTapAt(app: activeApp, x: x, y: y)
              }
            }
          }
        }
        if let response = unsupportedResponse(for: outcome) {
          return response
        }
        return Response(
          ok: true,
          data: DataPayload(
            message: "tap series",
            gestureStartUptimeMs: timing.gestureStartUptimeMs,
            gestureEndUptimeMs: timing.gestureEndUptimeMs,
            x: touchFrame.x,
            y: touchFrame.y,
            referenceWidth: touchFrame.referenceWidth,
            referenceHeight: touchFrame.referenceHeight
          )
        )
      }
      var outcome = RunnerInteractionOutcome.performed
      let timing = measureGesture {
        withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
          runSeries(count: count, pauseMs: intervalMs) { _ in
            if case .performed = outcome {
              outcome = tapAt(app: activeApp, x: x, y: y)
            }
          }
        }
      }
      if let response = unsupportedResponse(for: outcome) {
        return response
      }
      return Response(
        ok: true,
        data: DataPayload(
          message: "tap series",
          gestureStartUptimeMs: timing.gestureStartUptimeMs,
          gestureEndUptimeMs: timing.gestureEndUptimeMs,
          x: touchFrame.x,
          y: touchFrame.y,
          referenceWidth: touchFrame.referenceWidth,
          referenceHeight: touchFrame.referenceHeight
        )
      )
    case .longPress:
      guard let x = command.x, let y = command.y else {
        return Response(ok: false, error: ErrorPayload(message: "longPress requires x and y"))
      }
      let duration = (command.durationMs ?? 800) / 1000.0
      let touchFrame = resolvedTouchVisualizationFrame(app: activeApp, x: x, y: y)
      var outcome = RunnerInteractionOutcome.performed
      let timing = measureGesture {
        withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
          outcome = longPressAt(app: activeApp, x: x, y: y, duration: duration)
        }
      }
      if let response = unsupportedResponse(for: outcome) {
        return response
      }
      return Response(
        ok: true,
        data: DataPayload(
          message: "long pressed",
          gestureStartUptimeMs: timing.gestureStartUptimeMs,
          gestureEndUptimeMs: timing.gestureEndUptimeMs,
          x: touchFrame.x,
          y: touchFrame.y,
          referenceWidth: touchFrame.referenceWidth,
          referenceHeight: touchFrame.referenceHeight
        )
      )
    case .drag:
      guard let x = command.x, let y = command.y, let x2 = command.x2, let y2 = command.y2 else {
        return Response(ok: false, error: ErrorPayload(message: "drag requires x, y, x2, and y2"))
      }
      let holdDuration = min(max((command.durationMs ?? 60) / 1000.0, 0.016), 10.0)
      let dragPoints = keyboardAvoidingDragPoints(app: activeApp, x: x, y: y, x2: x2, y2: y2)
      let dragFrame = resolvedDragVisualizationFrame(
        app: activeApp,
        x: dragPoints.x,
        y: dragPoints.y,
        x2: dragPoints.x2,
        y2: dragPoints.y2
      )
      var outcome = RunnerInteractionOutcome.performed
      let timing = measureGesture {
        withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
          outcome = dragAt(
            app: activeApp,
            x: dragPoints.x,
            y: dragPoints.y,
            x2: dragPoints.x2,
            y2: dragPoints.y2,
            holdDuration: holdDuration
          )
        }
      }
      if let response = unsupportedResponse(for: outcome) {
        return response
      }
      return Response(
        ok: true,
        data: DataPayload(
          message: "dragged",
          gestureStartUptimeMs: timing.gestureStartUptimeMs,
          gestureEndUptimeMs: timing.gestureEndUptimeMs,
          x: dragFrame.x,
          y: dragFrame.y,
          x2: dragFrame.x2,
          y2: dragFrame.y2,
          referenceWidth: dragFrame.referenceWidth,
          referenceHeight: dragFrame.referenceHeight
        )
      )
    case .dragSeries:
      guard let x = command.x, let y = command.y, let x2 = command.x2, let y2 = command.y2 else {
        return Response(ok: false, error: ErrorPayload(message: "dragSeries requires x, y, x2, and y2"))
      }
      let count = max(Int(command.count ?? 1), 1)
      let pauseMs = max(command.pauseMs ?? 0, 0)
      let pattern = command.pattern ?? "one-way"
      if pattern != "one-way" && pattern != "ping-pong" {
        return Response(ok: false, error: ErrorPayload(message: "dragSeries pattern must be one-way or ping-pong"))
      }
      let holdDuration = min(max((command.durationMs ?? 60) / 1000.0, 0.016), 10.0)
      let dragPoints = keyboardAvoidingDragPoints(app: activeApp, x: x, y: y, x2: x2, y2: y2)
      var outcome = RunnerInteractionOutcome.performed
      let timing = measureGesture {
        withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
          runSeries(count: count, pauseMs: pauseMs) { idx in
            guard case .performed = outcome else {
              return
            }
            let reverse = pattern == "ping-pong" && (idx % 2 == 1)
            if reverse {
              outcome = dragAt(
                app: activeApp,
                x: dragPoints.x2,
                y: dragPoints.y2,
                x2: dragPoints.x,
                y2: dragPoints.y,
                holdDuration: holdDuration
              )
            } else {
              outcome = dragAt(
                app: activeApp,
                x: dragPoints.x,
                y: dragPoints.y,
                x2: dragPoints.x2,
                y2: dragPoints.y2,
                holdDuration: holdDuration
              )
            }
          }
        }
      }
      if let response = unsupportedResponse(for: outcome) {
        return response
      }
      return Response(
        ok: true,
        data: DataPayload(
          message: "drag series",
          gestureStartUptimeMs: timing.gestureStartUptimeMs,
          gestureEndUptimeMs: timing.gestureEndUptimeMs
        )
      )
    case .remotePress:
      guard let button = tvRemoteButton(from: command.remoteButton) else {
        return Response(ok: false, error: ErrorPayload(message: "remotePress requires remoteButton"))
      }
      let duration = (command.durationMs ?? 0) / 1000.0
      guard pressTvRemote(button, duration: duration) else {
        return Response(
          ok: false,
          error: ErrorPayload(code: "UNSUPPORTED_OPERATION", message: "remotePress is only supported on tvOS")
        )
      }
      return Response(ok: true, data: DataPayload(message: "remote pressed"))
    case .type:
      var response: Response?
      withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
        response = executeTypeCommand(activeApp: activeApp, command: command)
      }
      return response ?? Response(ok: false, error: ErrorPayload(message: "type produced no response"))
    case .interactionFrame:
      let frame = resolvedTouchReferenceFrame(app: activeApp, appFrame: activeApp.frame)
      return Response(
        ok: true,
        data: DataPayload(
          x: frame.minX,
          y: frame.minY,
          referenceWidth: frame.width,
          referenceHeight: frame.height
        )
      )
    case .swipe:
      guard let direction = command.direction else {
        return Response(ok: false, error: ErrorPayload(message: "swipe requires direction"))
      }
      var executedFrame: DragVisualizationFrame?
      let timing = measureGesture {
        withTemporaryScrollIdleTimeoutIfSupported(activeApp) {
          executedFrame = swipe(
            app: activeApp,
            direction: direction
          )
        }
      }
      guard let dragFrame = executedFrame else {
        return Response(ok: false, error: ErrorPayload(message: "swipe is only supported on tvOS"))
      }
      return Response(
        ok: true,
        data: DataPayload(
          message: "swiped",
          gestureStartUptimeMs: timing.gestureStartUptimeMs,
          gestureEndUptimeMs: timing.gestureEndUptimeMs,
          x: dragFrame.x,
          y: dragFrame.y,
          x2: dragFrame.x2,
          y2: dragFrame.y2,
          referenceWidth: dragFrame.referenceWidth,
          referenceHeight: dragFrame.referenceHeight
        )
      )
    case .findText:
      guard let text = command.text else {
        return Response(ok: false, error: ErrorPayload(message: "findText requires text"))
      }
      let found = findElement(app: activeApp, text: text) != nil
      return Response(ok: true, data: DataPayload(found: found))
    case .querySelector:
      guard let selectorKey = command.selectorKey, let selectorValue = command.selectorValue else {
        return Response(ok: false, error: ErrorPayload(message: "querySelector requires selectorKey and selectorValue"))
      }
      return queryElement(app: activeApp, selectorKey: selectorKey, selectorValue: selectorValue)
    case .readText:
      guard let x = command.x, let y = command.y else {
        return Response(ok: false, error: ErrorPayload(message: "readText requires x and y"))
      }
      guard let text = readTextAt(app: activeApp, x: x, y: y) else {
        return Response(ok: false, error: ErrorPayload(message: "readText did not resolve text"))
      }
      return Response(ok: true, data: DataPayload(text: text))
    case .snapshot:
      let options = SnapshotOptions(
        interactiveOnly: command.interactiveOnly ?? false,
        compact: command.compact ?? false,
        depth: command.depth,
        scope: command.scope,
        raw: command.raw ?? false
      )
      if options.raw {
        needsPostSnapshotInteractionDelay = true
        return Response(ok: true, data: snapshotRaw(app: activeApp, options: options))
      }
      needsPostSnapshotInteractionDelay = true
      return Response(ok: true, data: snapshotFast(app: activeApp, options: options))
    case .screenshot:
      let screenshot: XCUIScreenshot
#if os(macOS)
      // macOS keeps the app-targeted capture behavior for window-level screenshots.
      if let bundleId = command.appBundleId, !bundleId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
        let targetApp = XCUIApplication(bundleIdentifier: bundleId)
        targetApp.activate()
        activeApp = targetApp
        // Brief wait for the app transition animation to complete
        sleepFor(0.5)
      }
      if command.fullscreen == true {
        screenshot = XCUIScreen.main.screenshot()
      } else if let bundleId = command.appBundleId, !bundleId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
        screenshot = screenshotRoot(app: activeApp).screenshot()
      } else {
        screenshot = XCUIScreen.main.screenshot()
      }
#else
      screenshot = XCUIScreen.main.screenshot()
#endif
      guard let pngData = runnerPngData(for: screenshot.image) else {
        return Response(ok: false, error: ErrorPayload(message: "Failed to encode screenshot as PNG"))
      }
      // Local edit: return PNG bytes inline (base64). Upstream writes
      // the file and returns its path so the TS daemon can read it from
      // the sim's tmp container; maestro-runner needs the bytes back on
      // the host without an extra simctl roundtrip. We skip the file
      // write entirely.
      let base64 = pngData.base64EncodedString()
      return Response(ok: true, data: DataPayload(message: "screenshot", pngBase64: base64))
    case .back, .backInApp:
      if tapInAppBackControl(app: activeApp) {
        let message = command.command == .back ? "back" : "backInApp"
        return Response(ok: true, data: DataPayload(message: message))
      }
      return Response(ok: false, error: ErrorPayload(message: "in-app back control is not available"))
    case .backSystem:
      if performSystemBackAction(app: activeApp) {
        return Response(ok: true, data: DataPayload(message: "backSystem"))
      }
      return Response(ok: false, error: ErrorPayload(message: "system back is not available"))
    case .home:
      pressHomeButton()
      return Response(ok: true, data: DataPayload(message: "home"))
    case .rotate:
      guard let orientation = command.orientation?.trimmingCharacters(in: .whitespacesAndNewlines),
        !orientation.isEmpty
      else {
        return Response(ok: false, error: ErrorPayload(message: "rotate requires orientation"))
      }
      if rotateDevice(to: orientation) {
        return Response(
          ok: true,
          data: DataPayload(message: "rotate", orientation: orientation)
        )
      }
      return Response(
        ok: false,
        error: ErrorPayload(message: "unsupported rotate orientation: \(orientation)")
      )
    case .appSwitcher:
      performAppSwitcherGesture(app: activeApp)
      return Response(ok: true, data: DataPayload(message: "appSwitcher"))
    case .keyboardDismiss:
      let result = dismissKeyboard(app: activeApp)
      if result.wasVisible && !result.dismissed {
        return Response(
          ok: false,
          error: ErrorPayload(
            code: "UNSUPPORTED_OPERATION",
            message: "Unable to dismiss the iOS keyboard without a native dismiss gesture or control"
          )
        )
      }
      return Response(
        ok: true,
        data: DataPayload(
          message: "keyboardDismiss",
          visible: result.visible,
          wasVisible: result.wasVisible,
          dismissed: result.dismissed
        )
      )
    case .alert:
      let action = (command.action ?? "get").lowercased()
      guard let alert = resolveAlert(app: activeApp) else {
        return Response(ok: false, error: ErrorPayload(message: "alert not found"))
      }
      return handleAlert(alert, action: action)
    case .pinch:
      guard let scale = command.scale, scale > 0 else {
        return Response(ok: false, error: ErrorPayload(message: "pinch requires scale > 0"))
      }
      var outcome = RunnerInteractionOutcome.performed
      let timing = measureGesture {
        outcome = pinch(app: activeApp, scale: scale, x: command.x, y: command.y)
      }
      if let response = unsupportedResponse(for: outcome) {
        return response
      }
      return Response(
        ok: true,
        data: DataPayload(
          message: "pinched",
          gestureStartUptimeMs: timing.gestureStartUptimeMs,
          gestureEndUptimeMs: timing.gestureEndUptimeMs
        )
      )
    }
  }

  private func executeTypeCommand(activeApp: XCUIApplication, command: Command) -> Response {
    guard let text = command.text else {
      return Response(ok: false, error: ErrorPayload(message: "type requires text"))
    }
    let delaySeconds = Double(max(command.delayMs ?? 0, 0)) / 1000.0
    let textEntryMode = resolveTextEntryMode(command)
    // Local edit: pass the `id` selector value as an identifier hint so
    // focusTextInputForTextEntry can resolve the target via XCTest's
    // query DSL instead of walking the full descendant tree.
    let identifierHint = command.selectorKey == "id" ? command.selectorValue : nil
    let target = focusTextInputForTextEntry(
      app: activeApp, x: command.x, y: command.y, identifier: identifierHint
    )
    if textEntryMode == .replacement {
      guard target.element != nil else {
        let message =
          (command.x != nil && command.y != nil)
          ? "no text input found at the provided coordinates to clear"
          : "no focused text input to clear"
        return Response(ok: false, error: ErrorPayload(message: message))
      }
    }
    let textResult = typeTextReliably(
      app: activeApp,
      target: target,
      text: text,
      delaySeconds: delaySeconds,
      repairMode: textEntryMode
    )
    if textResult.verified == false {
      let expected = textResult.expectedText ?? ""
      let observed = textResult.observedText ?? ""
      return Response(
        ok: false,
        error: ErrorPayload(
          code: "TEXT_ENTRY_MISMATCH",
          message: "text entry verification failed: expected \"\(expected)\", observed \"\(observed)\""
        )
      )
    }
    return Response(ok: true, data: DataPayload(message: textResult.repaired ? "typed after repair" : "typed"))
  }
}
