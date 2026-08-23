#ifndef SnapshotParams_h
#define SnapshotParams_h

#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

/**
 * Overrides XCTest's accessibility-snapshot request parameters so
 * `XCUIElement.snapshot()` returns a complete tree instead of XCTest's
 * silently-clipped default.
 *
 * XCTest's `-[XCAXClient_iOS defaultParameters]` nominally defaults maxDepth /
 * maxChildren / maxArrayCount to INT_MAX, but in practice the tree is clipped
 * and elements behind modal views are dropped. This swizzles defaultParameters
 * to re-assert generous limits and set `snapshotKeyHonorModalViews=0`, which is
 * exactly what WebDriverAgent and mobile-next/devicekit-ios do. It is the fix
 * for deep React Native trees where native shadow views and modal-obscured
 * content were missing from our snapshot.
 *
 * Idempotent and safe: a no-op if XCAXClient_iOS is unavailable. Call once at
 * runner startup, before the first snapshot.
 */
void DLApplyCompleteSnapshotParams(void);

NS_ASSUME_NONNULL_END

#endif /* SnapshotParams_h */
