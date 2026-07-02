// MARK: - Wire Models

enum CommandType: String, Codable {
  case tap
  // Local extension: walk the XCUI tree, find best liberal match (substring
  // + case-insensitive + prefer-editable-inputs ranking), tap at its center.
  // Returns small {ok:true} on hit, or {ok:false, snapshot:[walked nodes]}
  // on miss so the Go side can fall back to its own snapshot+filter+tap.
  case tapBySelector
  case mouseClick
  case tapSeries
  case longPress
  case interactionFrame
  case drag
  case dragSeries
  case remotePress
  case type
  case swipe
  case findText
  case querySelector
  case readText
  case snapshot
  case screenshot
  case back
  case backInApp
  case backSystem
  case home
  case rotate
  case appSwitcher
  case keyboardDismiss
  case alert
  case pinch
  case recordStart
  case recordStop
  case uptime
  case shutdown
  // Local extension (not in upstream agent-device): take two screenshots
  // in-process and return the fraction of differing pixels. Lets the Go
  // side's waitForAnimationToEnd avoid two full PNG roundtrips +
  // re-decode + pixel-walk per iteration.
  case idleCheck
  // Local extension: runner-side wait-for-idle loop. Takes timeoutMs +
  // diff threshold; loops idleCheck internally until settled or timeout.
  // Avoids HTTP roundtrips per poll iteration.
  case awaitIdle
}

struct Command: Codable {
  let command: CommandType
  let appBundleId: String?
  let text: String?
  let selectorKey: String?
  let selectorValue: String?
  let delayMs: Int?
  let textEntryMode: String?
  let clearFirst: Bool?
  let action: String?
  let x: Double?
  let y: Double?
  let button: String?
  let remoteButton: String?
  let count: Double?
  let intervalMs: Double?
  let doubleTap: Bool?
  let pauseMs: Double?
  let pattern: String?
  let x2: Double?
  let y2: Double?
  let durationMs: Double?
  let direction: String?
  let orientation: String?
  let scale: Double?
  let outPath: String?
  let fps: Int?
  let quality: Int?
  let interactiveOnly: Bool?
  let compact: Bool?
  let depth: Int?
  let scope: String?
  let raw: Bool?
  let fullscreen: Bool?
}

struct Response: Codable {
  let ok: Bool
  let data: DataPayload?
  let error: ErrorPayload?

  init(ok: Bool, data: DataPayload? = nil, error: ErrorPayload? = nil) {
    self.ok = ok
    self.data = data
    self.error = error
  }
}

struct DataPayload: Codable {
  let message: String?
  let text: String?
  let found: Bool?
  let items: [String]?
  let nodes: [SnapshotNode]?
  let truncated: Bool?
  let gestureStartUptimeMs: Double?
  let gestureEndUptimeMs: Double?
  let x: Double?
  let y: Double?
  let x2: Double?
  let y2: Double?
  let referenceWidth: Double?
  let referenceHeight: Double?
  let currentUptimeMs: Double?
  let visible: Bool?
  let wasVisible: Bool?
  let dismissed: Bool?
  let orientation: String?
  // Local extension: identifier of the matched element when applicable
  // (tapBySelector uses this so the Go side can populate
  // lastTappedIdentifier for the next inputText to use as a typing hint).
  let identifier: String?
  // Local extension (not in upstream agent-device): inline PNG bytes
  // (base64-encoded) for screenshot responses. Upstream writes a file
  // and returns the path in `message`; for maestro-runner we need the
  // bytes back on the host without an extra simctl roundtrip.
  let pngBase64: String?
  // Local extension: fraction of differing pixels from idleCheck.
  let diffFraction: Double?

  init(
    message: String? = nil,
    text: String? = nil,
    found: Bool? = nil,
    items: [String]? = nil,
    nodes: [SnapshotNode]? = nil,
    truncated: Bool? = nil,
    gestureStartUptimeMs: Double? = nil,
    gestureEndUptimeMs: Double? = nil,
    x: Double? = nil,
    y: Double? = nil,
    x2: Double? = nil,
    y2: Double? = nil,
    referenceWidth: Double? = nil,
    referenceHeight: Double? = nil,
    currentUptimeMs: Double? = nil,
    visible: Bool? = nil,
    wasVisible: Bool? = nil,
    dismissed: Bool? = nil,
    orientation: String? = nil,
    pngBase64: String? = nil,
    diffFraction: Double? = nil,
    identifier: String? = nil
  ) {
    self.message = message
    self.text = text
    self.found = found
    self.items = items
    self.nodes = nodes
    self.truncated = truncated
    self.gestureStartUptimeMs = gestureStartUptimeMs
    self.gestureEndUptimeMs = gestureEndUptimeMs
    self.x = x
    self.y = y
    self.x2 = x2
    self.y2 = y2
    self.referenceWidth = referenceWidth
    self.referenceHeight = referenceHeight
    self.currentUptimeMs = currentUptimeMs
    self.visible = visible
    self.wasVisible = wasVisible
    self.dismissed = dismissed
    self.orientation = orientation
    self.pngBase64 = pngBase64
    self.diffFraction = diffFraction
    self.identifier = identifier
  }
}

struct ErrorPayload: Codable {
  let code: String?
  let message: String

  init(code: String? = nil, message: String) {
    self.code = code
    self.message = message
  }
}

struct SnapshotRect: Codable {
  let x: Double
  let y: Double
  let width: Double
  let height: Double
}

struct SnapshotNode: Codable {
  let index: Int
  let type: String
  let label: String?
  let identifier: String?
  let value: String?
  // Local extension (not in upstream agent-device): placeholder text for
  // editable fields. maestro-runner YAML flows commonly match on
  // placeholder text (e.g. RN TextInput placeholder="Username"); without
  // this field, that text isn't visible in the snapshot.
  let placeholderValue: String?
  let rect: SnapshotRect
  let enabled: Bool
  let focused: Bool?
  let selected: Bool?
  let hittable: Bool
  let depth: Int
  let parentIndex: Int?
  let hiddenContentAbove: Bool?
  let hiddenContentBelow: Bool?
}

struct SnapshotOptions {
  let interactiveOnly: Bool
  let compact: Bool
  let depth: Int?
  let scope: String?
  let raw: Bool
}
