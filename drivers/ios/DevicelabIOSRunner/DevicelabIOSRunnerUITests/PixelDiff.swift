// Local file (not in upstream agent-device). Computes the fraction of
// differing pixels between two UIImages by drawing each into a raw RGBA
// byte buffer via CGContext and comparing those buffers directly. Used
// by the idleCheck command to power waitForAnimationToEnd without
// shipping two full PNGs back to the host.

import Foundation
#if canImport(UIKit)
import UIKit
#endif

extension RunnerTests {
  func computePixelDiffFraction(_ a: RunnerImage, _ b: RunnerImage) -> Double {
    guard let cgA = runnerCGImage(from: a), let cgB = runnerCGImage(from: b) else {
      return 1.0
    }
    let width = cgA.width
    let height = cgA.height
    guard width > 0, height > 0,
          width == cgB.width, height == cgB.height else {
      return 1.0
    }

    let bytesPerRow = width * 4
    let totalBytes = bytesPerRow * height
    var bufA = [UInt8](repeating: 0, count: totalBytes)
    var bufB = [UInt8](repeating: 0, count: totalBytes)

    let colorSpace = CGColorSpaceCreateDeviceRGB()
    let bitmapInfo: UInt32 =
      CGImageAlphaInfo.premultipliedLast.rawValue |
      CGBitmapInfo.byteOrder32Big.rawValue

    guard let ctxA = CGContext(
      data: &bufA,
      width: width,
      height: height,
      bitsPerComponent: 8,
      bytesPerRow: bytesPerRow,
      space: colorSpace,
      bitmapInfo: bitmapInfo
    ) else {
      return 1.0
    }
    guard let ctxB = CGContext(
      data: &bufB,
      width: width,
      height: height,
      bitsPerComponent: 8,
      bytesPerRow: bytesPerRow,
      space: colorSpace,
      bitmapInfo: bitmapInfo
    ) else {
      return 1.0
    }
    let rect = CGRect(x: 0, y: 0, width: width, height: height)
    ctxA.draw(cgA, in: rect)
    ctxB.draw(cgB, in: rect)

    var differing = 0
    let pixelCount = width * height
    var i = 0
    while i < totalBytes {
      // Compare R, G, B (skip alpha).
      if bufA[i] != bufB[i] || bufA[i + 1] != bufB[i + 1] || bufA[i + 2] != bufB[i + 2] {
        differing += 1
      }
      i += 4
    }
    return Double(differing) / Double(pixelCount)
  }
}
