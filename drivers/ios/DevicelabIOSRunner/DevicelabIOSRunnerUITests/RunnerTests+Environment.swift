import Foundation

// MARK: - Environment

enum RunnerEnv {
  // Accepts either AGENT_DEVICE_RUNNER_PORT (agent-device's name, kept for
  // verbatim-copy compatibility) or DEVICELAB_IOS_RUNNER_PORT (set by the
  // maestro-runner Go driver during xctestrun injection).
  static func resolvePort() -> UInt16 {
    for key in ["DEVICELAB_IOS_RUNNER_PORT", "AGENT_DEVICE_RUNNER_PORT"] {
      if let env = ProcessInfo.processInfo.environment[key], let port = UInt16(env) {
        return port
      }
    }
    for arg in CommandLine.arguments {
      for prefix in ["DEVICELAB_IOS_RUNNER_PORT=", "AGENT_DEVICE_RUNNER_PORT="] {
        if arg.hasPrefix(prefix) {
          let value = arg.replacingOccurrences(of: prefix, with: "")
          if let port = UInt16(value) { return port }
        }
      }
    }
    return 0
  }

  static func isTruthy(_ name: String) -> Bool {
    guard let raw = ProcessInfo.processInfo.environment[name] else {
      return false
    }
    switch raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "1", "true", "yes", "on":
      return true
    default:
      return false
    }
  }
}
