#import "RunnerXCTestTimeouts.h"

#import <XCTest/XCTest.h>
#import <dlfcn.h>
#import <objc/message.h>

// Exported by XCUIAutomation (Xcode 15 through 27); WebDriverAgent declares
// the same pairs in CDStructures.h. Looked up rather than linked so the
// runner still builds and runs if a later Xcode drops them.
typedef double (*RunnerTimeoutGetter)(void);
typedef void (*RunnerTimeoutSetter)(double);

static RunnerTimeoutGetter RunnerXPCGetter(void) {
  static RunnerTimeoutGetter getter = NULL;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    getter = (RunnerTimeoutGetter)dlsym(RTLD_DEFAULT, "_XCTXPCRequestTimeout");
  });
  return getter;
}

static RunnerTimeoutSetter RunnerXPCSetter(void) {
  static RunnerTimeoutSetter setter = NULL;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    setter = (RunnerTimeoutSetter)dlsym(RTLD_DEFAULT, "_XCTSetXPCRequestTimeout");
  });
  return setter;
}

// The limit on XCTest's app-state waits, quiescence among them.
static RunnerTimeoutGetter RunnerAppStateGetter(void) {
  static RunnerTimeoutGetter getter = NULL;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    getter = (RunnerTimeoutGetter)dlsym(RTLD_DEFAULT, "_XCTApplicationStateTimeout");
  });
  return getter;
}

static RunnerTimeoutSetter RunnerAppStateSetter(void) {
  static RunnerTimeoutSetter setter = NULL;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    setter = (RunnerTimeoutSetter)dlsym(RTLD_DEFAULT, "_XCTSetApplicationStateTimeout");
  });
  return setter;
}

// One lock per global, recursive so scopes can nest (a default-timeout retry
// inside a short scope). Commands run on the main thread, so the lock only
// guards against a future caller on another thread.
static NSRecursiveLock *RunnerTimeoutLock(void) {
  static NSRecursiveLock *lock = nil;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    lock = [NSRecursiveLock new];
  });
  return lock;
}

// Sets a process-wide XCTest timeout for one run of block and restores it
// afterwards, even if the block raises. Runs block unchanged and returns NO
// when either symbol is missing.
static BOOL RunnerWithGlobalTimeout(RunnerTimeoutGetter getter, RunnerTimeoutSetter setter,
                                    NSTimeInterval timeout, NS_NOESCAPE void (^block)(void)) {
  if (getter == NULL || setter == NULL || timeout <= 0) {
    block();
    return NO;
  }
  NSRecursiveLock *lock = RunnerTimeoutLock();
  [lock lock];
  double previous = getter();
  setter(timeout);
  @try {
    block();
  } @finally {
    setter(previous);
    [lock unlock];
  }
  return YES;
}

typedef double (*RunnerAXTimeoutGetter)(id, SEL);
typedef BOOL (*RunnerAXTimeoutSetter)(id, SEL, double, NSError **);
typedef BOOL (*RunnerBoolGetter)(id, SEL);
typedef void (*RunnerQuiescenceWait)(id, SEL, BOOL, BOOL);
typedef void (*RunnerLegacyQuiescenceWait)(id, SEL, BOOL);

// XCUIApplication -> XCUIApplicationImpl -> XCUIApplicationProcess, or nil
// when any link is missing.
static id RunnerApplicationProcess(XCUIApplication *app) {
  SEL implSel = NSSelectorFromString(@"applicationImpl");
  SEL processSel = NSSelectorFromString(@"currentProcess");
  if (![app respondsToSelector:implSel]) {
    return nil;
  }
  id impl = ((id (*)(id, SEL))objc_msgSend)(app, implSel);
  if (![impl respondsToSelector:processSel]) {
    return nil;
  }
  return ((id (*)(id, SEL))objc_msgSend)(impl, processSel);
}

// Runs XCTest's quiescence wait on process. Returns NO when neither the
// current nor the pre-Xcode 15 selector exists.
static BOOL RunnerRunQuiescenceWait(id process) {
  SEL current = NSSelectorFromString(@"waitForQuiescenceIncludingAnimationsIdle:isPreEvent:");
  SEL legacy = NSSelectorFromString(@"waitForQuiescenceIncludingAnimationsIdle:");
  if ([process respondsToSelector:current]) {
    // Post-event: the pre-event variant is skipped outright under the
    // skip-pre-event interaction option, which would read as instant idle.
    ((RunnerQuiescenceWait)objc_msgSend)(process, current, YES, NO);
    return YES;
  }
  if ([process respondsToSelector:legacy]) {
    ((RunnerLegacyQuiescenceWait)objc_msgSend)(process, legacy, YES);
    return YES;
  }
  return NO;
}

@implementation RunnerXCTestTimeouts

+ (NSTimeInterval)defaultXPCRequestTimeout {
  static NSTimeInterval initial = 0;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    RunnerTimeoutGetter getter = RunnerXPCGetter();
    initial = getter != NULL ? getter() : 0;
  });
  return initial;
}

+ (BOOL)withXPCRequestTimeout:(NSTimeInterval)timeout do:(NS_NOESCAPE void (^)(void))block {
  // Capture XCTest's own value before the first override.
  (void)[self defaultXPCRequestTimeout];
  return RunnerWithGlobalTimeout(RunnerXPCGetter(), RunnerXPCSetter(), timeout, block);
}

+ (RunnerQuiescence)waitForQuiescenceOfApplication:(XCUIApplication *)app
                                           timeout:(NSTimeInterval)timeout {
  id process = RunnerApplicationProcess(app);
  SEL idledSel = NSSelectorFromString(@"eventLoopHasIdled");
  SEL finishedSel = NSSelectorFromString(@"animationsHaveFinished");
  // Without the state-timeout setter the wait would run to XCTest's own
  // limit, far past any cap, so it is not attempted at all.
  if (process == nil || ![process respondsToSelector:idledSel] ||
      ![process respondsToSelector:finishedSel] || RunnerAppStateGetter() == NULL ||
      RunnerAppStateSetter() == NULL || timeout <= 0) {
    return RunnerQuiescenceUnavailable;
  }
  __block BOOL waited = NO;
  RunnerWithGlobalTimeout(RunnerAppStateGetter(), RunnerAppStateSetter(), timeout, ^{
    waited = RunnerRunQuiescenceWait(process);
  });
  if (!waited) {
    return RunnerQuiescenceUnavailable;
  }
  RunnerBoolGetter get = (RunnerBoolGetter)objc_msgSend;
  BOOL idle = get(process, idledSel) && get(process, finishedSel);
  return idle ? RunnerQuiescenceIdle : RunnerQuiescenceBusy;
}

+ (BOOL)withAXTimeout:(NSTimeInterval)timeout do:(NS_NOESCAPE void (^)(void))block {
  id client = [self accessibilityClient];
  SEL getSel = NSSelectorFromString(@"AXTimeout");
  SEL setSel = NSSelectorFromString(@"_setAXTimeout:error:");
  if (client == nil || timeout <= 0 || ![client respondsToSelector:getSel] ||
      ![client respondsToSelector:setSel]) {
    block();
    return NO;
  }
  RunnerAXTimeoutGetter get = (RunnerAXTimeoutGetter)objc_msgSend;
  RunnerAXTimeoutSetter set = (RunnerAXTimeoutSetter)objc_msgSend;
  NSRecursiveLock *lock = RunnerTimeoutLock();
  [lock lock];
  double previous = get(client, getSel);
  NSError *error = nil;
  if (!set(client, setSel, timeout, &error)) {
    NSLog(@"DL_AX_TIMEOUT: could not set %.1fs (%@); running unbounded", timeout, error);
    [lock unlock];
    block();
    return NO;
  }
  @try {
    block();
  } @finally {
    NSError *restoreError = nil;
    if (!set(client, setSel, previous, &restoreError)) {
      NSLog(@"DL_AX_TIMEOUT: could not restore %.1fs (%@)", previous, restoreError);
    }
    [lock unlock];
  }
  return YES;
}

+ (nullable id)accessibilityClient {
  SEL selector = NSSelectorFromString(@"accessibilityInterface");
  XCUIDevice *device = XCUIDevice.sharedDevice;
  if (![device respondsToSelector:selector]) {
    return nil;
  }
  return ((id (*)(id, SEL))objc_msgSend)(device, selector);
}

@end
