import XCTest

extension RunnerTests {
  // Per-request depth cap. Kept moderate on purpose: the AX server rejects a
  // bulk request outright (kAXErrorIllegalArgument) once depth crosses a
  // tree-size-dependent limit, and frontier re-rooting reaches deeper content
  // without ever passing a larger depth. 20 clears every React Navigation
  // screen measured; deeper subtrees arrive through re-rooting.
  private static let privateAXMaxDepth = 20
  // Whole-capture node budget. Higher than the public path's fastSnapshotLimit
  // because the point of this path is completeness on a screen the public
  // serializer could not deliver at all.
  private static let privateAXMaxNodes = 4000
  // Re-root requests per capture. A depth-capped RN tree resolves in 1-3 chained
  // requests; the bound stops a pathological tree from stacking requests.
  private static let privateAXDeepExtensionCallLimit = 12
  // Whole-capture wall-clock bound, seconds. A responsive capture is ~100-400ms;
  // this only contains an unresponsive app.
  private static let privateAXTimeout: TimeInterval = 4.0

  /// Snapshot via the private AX client, used when the public
  /// `XCUIElement.snapshot()` path threw or returned a childless root on a deep
  /// React Native tree. Returns nil when the bridge is unavailable or the
  /// capture failed, so the caller keeps its existing (empty) result.
  func privateAXFallbackPayload(app: XCUIApplication, options: SnapshotOptions) -> DataPayload? {
    // The bridge's timeout only stops it starting new requests. Each request
    // also gets a bound here, or one sent to an app that leaves the screen
    // mid-capture waits out XCTest's 60s AX timeout.
    var response: [String: Any] = [:]
    RunnerXCTestTimeouts.withAXTimeout(RunnerTests.privateAXTimeout) {
      response = RunnerAXSnapshotBridge.snapshotTree(
        for: app,
        maxDepth: RunnerTests.privateAXMaxDepth,
        maxNodes: RunnerTests.privateAXMaxNodes,
        deepExtensionCallLimit: RunnerTests.privateAXDeepExtensionCallLimit,
        timeout: RunnerTests.privateAXTimeout
      )
    }
    guard (response["ok"] as? Bool) == true,
          let rootDict = response["root"] as? [String: Any] else {
      if let error = response["error"] as? String {
        NSLog("DL_PRIVATE_AX_FALLBACK: unavailable (%@)", error)
      }
      return nil
    }

    let viewport = axFrame(from: rootDict["frame"])
    var nodes: [SnapshotNode] = []
    let depthLimit = options.depth ?? Int.max
    appendAXNode(rootDict, depth: 0, parentIndex: nil, viewport: viewport,
                 depthLimit: depthLimit, into: &nodes)
    let truncated = (response["truncated"] as? Bool) ?? false
    NSLog("DL_PRIVATE_AX_FALLBACK: recovered %ld nodes (truncated=%d)",
          nodes.count, truncated ? 1 : 0)
    guard !nodes.isEmpty else { return nil }
    return DataPayload(nodes: nodes, truncated: truncated, appState: appStateString(app))
  }

  /// Flattens the bridge's nested node dict into the flat, parent-indexed
  /// SnapshotNode list the rest of the runner and the Go host expect.
  private func appendAXNode(_ node: [String: Any], depth: Int, parentIndex: Int?,
                            viewport: CGRect, depthLimit: Int, into nodes: inout [SnapshotNode]) {
    if nodes.count >= RunnerTests.privateAXMaxNodes { return }
    if depth > depthLimit { return }

    let index = nodes.count
    let typeRaw = (node["type"] as? NSNumber)?.uintValue ?? XCUIElement.ElementType.other.rawValue
    let type = elementTypeName(XCUIElement.ElementType(rawValue: typeRaw) ?? .other)
    let frame = axFrame(from: node["frame"])
    let enabled = (node["enabled"] as? NSNumber)?.boolValue ?? true
    // No occlusion pass here (a re-rooted tree has no reliable sibling order);
    // hittable is the honest geometric answer: a real, enabled rect on screen.
    let onScreen = !frame.isNull && !frame.isEmpty && frame.intersects(viewport)
    let hittable = enabled && onScreen
    let label = axString(node["label"])
    let identifier = axString(node["identifier"])
    let value = axString(node["value"])
    let selected = (node["selected"] as? NSNumber)?.boolValue ?? false
    let focused = (node["focused"] as? NSNumber)?.boolValue ?? false

    nodes.append(
      SnapshotNode(
        index: index,
        type: type,
        label: label,
        identifier: identifier,
        value: value,
        placeholderValue: nil,
        rect: SnapshotRect(x: Double(frame.origin.x), y: Double(frame.origin.y),
                           width: Double(frame.size.width), height: Double(frame.size.height)),
        enabled: enabled,
        focused: focused ? true : nil,
        selected: selected ? true : nil,
        hittable: hittable,
        depth: depth,
        parentIndex: parentIndex,
        hiddenContentAbove: nil,
        hiddenContentBelow: nil
      )
    )

    guard let children = node["children"] as? [[String: Any]] else { return }
    for child in children {
      appendAXNode(child, depth: depth + 1, parentIndex: index, viewport: viewport,
                   depthLimit: depthLimit, into: &nodes)
      if nodes.count >= RunnerTests.privateAXMaxNodes { break }
    }
  }

  private func axFrame(from value: Any?) -> CGRect {
    guard let dict = value as? [String: Any] else { return .zero }
    let x = (dict["x"] as? NSNumber)?.doubleValue ?? 0
    let y = (dict["y"] as? NSNumber)?.doubleValue ?? 0
    let w = (dict["width"] as? NSNumber)?.doubleValue ?? 0
    let h = (dict["height"] as? NSNumber)?.doubleValue ?? 0
    return CGRect(x: x, y: y, width: w, height: h)
  }

  private func axString(_ value: Any?) -> String? {
    guard let s = value as? String, !s.isEmpty else { return nil }
    return s
  }
}
