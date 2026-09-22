#import "RunnerXCTestTimeouts.h"

#import <XCTest/XCTest.h>
#import <dlfcn.h>
#import <objc/message.h>

// Exported by XCUIAutomation (Xcode 15 through 27); WebDriverAgent declares
// the same pair in CDStructures.h. Looked up rather than linked so the runner
// still builds and runs if a later Xcode drops them.
typedef double (*RunnerXPCTimeoutGetter)(void);
typedef void (*RunnerXPCTimeoutSetter)(double);

static RunnerXPCTimeoutGetter RunnerXPCGetter(void) {
  static RunnerXPCTimeoutGetter getter = NULL;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    getter = (RunnerXPCTimeoutGetter)dlsym(RTLD_DEFAULT, "_XCTXPCRequestTimeout");
  });
  return getter;
}

static RunnerXPCTimeoutSetter RunnerXPCSetter(void) {
  static RunnerXPCTimeoutSetter setter = NULL;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    setter = (RunnerXPCTimeoutSetter)dlsym(RTLD_DEFAULT, "_XCTSetXPCRequestTimeout");
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

typedef double (*RunnerAXTimeoutGetter)(id, SEL);
typedef BOOL (*RunnerAXTimeoutSetter)(id, SEL, double, NSError **);

@implementation RunnerXCTestTimeouts

+ (NSTimeInterval)defaultXPCRequestTimeout {
  static NSTimeInterval initial = 0;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    RunnerXPCTimeoutGetter getter = RunnerXPCGetter();
    initial = getter != NULL ? getter() : 0;
  });
  return initial;
}

+ (BOOL)withXPCRequestTimeout:(NSTimeInterval)timeout do:(NS_NOESCAPE void (^)(void))block {
  RunnerXPCTimeoutGetter getter = RunnerXPCGetter();
  RunnerXPCTimeoutSetter setter = RunnerXPCSetter();
  if (getter == NULL || setter == NULL || timeout <= 0) {
    block();
    return NO;
  }
  // Capture XCTest's own value before the first override.
  (void)[self defaultXPCRequestTimeout];
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
