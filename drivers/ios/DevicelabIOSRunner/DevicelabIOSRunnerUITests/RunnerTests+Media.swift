import Photos
import XCTest

extension RunnerTests {
  /// Adds a photo/video to the device Photos library via PhotoKit.
  ///
  /// This runs on-device because writing the Photos database is only possible
  /// from a signed, add-permitted process — there is no host-side path on real
  /// devices. On the simulator the host uses `simctl addmedia` instead and
  /// never reaches this; this path is what gives real iOS devices addMedia
  /// support (which upstream Maestro lacks).
  func executeAddMedia(command: Command) throws -> Response {
    guard let b64 = command.mediaData, let name = command.mediaName else {
      return Response(ok: false, error: ErrorPayload(message: "addMedia: mediaData and mediaName are required"))
    }
    guard let data = Data(base64Encoded: b64) else {
      return Response(ok: false, error: ErrorPayload(message: "addMedia: invalid base64 data"))
    }

    let isVideo = (command.mimeType ?? "").hasPrefix("video/")
    let tmpURL = URL(fileURLWithPath: NSTemporaryDirectory()).appendingPathComponent(name)
    do {
      try data.write(to: tmpURL)
    } catch {
      return Response(ok: false, error: ErrorPayload(message: "addMedia: failed to write temp file: \(error.localizedDescription)"))
    }
    defer { try? FileManager.default.removeItem(at: tmpURL) }

    // Ensure add-only authorization. On a real device the first request shows a
    // SpringBoard alert, which we auto-accept on a background queue while the
    // request blocks. On the simulator the permission is pre-granted host-side.
    if #available(iOS 14, *) {
      if PHPhotoLibrary.authorizationStatus(for: .addOnly) == .notDetermined {
        acceptPhotoPermissionAlert()
        let authSem = DispatchSemaphore(value: 0)
        PHPhotoLibrary.requestAuthorization(for: .addOnly) { _ in authSem.signal() }
        _ = authSem.wait(timeout: .now() + 15)
      }
    }

    var creationError: Error?
    let sem = DispatchSemaphore(value: 0)
    PHPhotoLibrary.shared().performChanges({
      if isVideo {
        _ = PHAssetCreationRequest.creationRequestForAssetFromVideo(atFileURL: tmpURL)
      } else {
        _ = PHAssetCreationRequest.creationRequestForAssetFromImage(atFileURL: tmpURL)
      }
    }, completionHandler: { success, error in
      if !success { creationError = error }
      sem.signal()
    })
    if sem.wait(timeout: .now() + 30) == .timedOut {
      return Response(ok: false, error: ErrorPayload(message: "addMedia: PhotoKit timed out"))
    }
    if let e = creationError {
      return Response(ok: false, error: ErrorPayload(message: "addMedia: PhotoKit failed: \(e.localizedDescription)"))
    }
    return Response(ok: true, data: DataPayload(message: "added media: \(name)"))
  }

  /// Best-effort auto-accept of the one-time add-only Photos permission alert on
  /// a real device (no-op on the simulator where permission is pre-granted).
  private func acceptPhotoPermissionAlert() {
    DispatchQueue.global(qos: .userInitiated).async {
      let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
      let labels = ["Allow Access to All Photos", "Allow", "Add", "OK"]
      let deadline = Date().addingTimeInterval(12)
      while Date() < deadline {
        for label in labels {
          let button = springboard.buttons[label]
          if button.exists && button.isHittable {
            button.tap()
            return
          }
        }
        Thread.sleep(forTimeInterval: 0.3)
      }
    }
  }
}
