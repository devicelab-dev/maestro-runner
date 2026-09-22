#ifndef RunnerXCTestTimeouts_h
#define RunnerXCTestTimeouts_h

#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

@class XCUIApplication;

/// The outcome of +waitForQuiescenceOfApplication:timeout:.
typedef NS_ENUM(NSInteger, RunnerQuiescence) {
  /// XCTest saw the app's event loop go idle and its animations finish.
  RunnerQuiescenceIdle = 0,
  /// The wait ended (the cap passed) without both signals.
  RunnerQuiescenceBusy = 1,
  /// A private symbol is missing, so nothing was waited for and nothing is
  /// known. Never to be read as idle.
  RunnerQuiescenceUnavailable = 2,
};

/**
 * Scoped overrides of XCTest's process-wide request timeouts.
 *
 * XCTest resolves element queries through the target app's automation
 * session over XPC, and every such request waits up to the XPC request
 * timeout (30s by default). When the target app leaves the screen while a
 * query is in flight (Home, the app switcher), iOS suspends it and the reply
 * never comes, so the query waits the full 30s. The runner executes commands
 * one at a time on the main thread, so every later command waits too.
 *
 * These helpers set a timeout for the duration of a block and restore the
 * previous value afterwards, even if the block raises. The pattern follows
 * WebDriverAgent's FBXCAXClientProxy (+withXPCRequestTimeout:do:,
 * -withAXTimeout:do:).
 *
 * A shorter timeout only makes this process stop waiting. The request that
 * was abandoned keeps running on its channel until the app answers or dies.
 *
 * Every private symbol is looked up at runtime. When one is missing the block
 * runs with XCTest's own timeout, so an XCTest change can make a hang long
 * again but can never break a snapshot.
 */
@interface RunnerXCTestTimeouts : NSObject

/// XCTest's XPC request timeout as it stood before this class first changed
/// it, or 0 when the private getter is unavailable. The runner uses it to go
/// back to XCTest's own patience inside a shorter scope.
+ (NSTimeInterval)defaultXPCRequestTimeout;

/// Runs @c block once with XCTest's XPC request timeout (the per-request
/// limit on element queries answered by an app's automation session) set to
/// @c timeout seconds. Returns YES if the timeout was applied, NO if the
/// private setter is unavailable and the block ran with XCTest's own value.
+ (BOOL)withXPCRequestTimeout:(NSTimeInterval)timeout do:(NS_NOESCAPE void (^)(void))block;

/// Runs @c block once with the accessibility client's AXTimeout (the
/// per-request limit on direct accessibility requests, such as the private
/// AX snapshot) set to @c timeout seconds. Returns YES if the timeout was
/// applied, NO if the block ran with the existing value.
+ (BOOL)withAXTimeout:(NSTimeInterval)timeout do:(NS_NOESCAPE void (^)(void))block;

/// Waits up to @c timeout seconds for @c app to go quiescent, animations
/// included: the wait XCTest runs before and after each synthesized event,
/// run on demand. Follows WebDriverAgent's -fb_waitUntilStableWithTimeout:,
/// which bounds XCUIApplicationProcess
/// -waitForQuiescenceIncludingAnimationsIdle:(isPreEvent:) by setting
/// XCTest's application-state timeout for the duration of the call.
///
/// The answer comes from the process's own eventLoopHasIdled and
/// animationsHaveFinished flags after the wait, not from how long it took.
/// XCTest skips the whole check for an app that is not in the foreground
/// and leaves the flags stale, so callers must only ask about a foreground
/// app. Must be called on the main thread (XCTest asserts it). Does not
/// activate the app.
///
/// When the event loop fails to idle within the cap, XCTest may take a
/// spindump of the app before returning, so the call can outlast
/// @c timeout; callers report the time they actually waited.
+ (RunnerQuiescence)waitForQuiescenceOfApplication:(XCUIApplication *)app
                                           timeout:(NSTimeInterval)timeout
    NS_SWIFT_NAME(waitForQuiescence(of:timeout:));

@end

NS_ASSUME_NONNULL_END

#endif /* RunnerXCTestTimeouts_h */
