/**
 * Copyright (c) 2015-present, Facebook, Inc.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

#import "FBHIDOperations.h"

#import <dlfcn.h>
#import <mach/mach_time.h>

#import "FBConfiguration.h"
#import "FBErrorBuilder.h"
#import "FBLogger.h"
#import "FBRunLoopSpinner.h"
#import "FBScreen.h"
#import "FBSession.h"
#import "XCUIScreen.h"
#import "XCUIDevice+FBHinge.h"
#import "XCUIApplication.h"
#import "XCUIApplication+FBHelpers.h"
#import "XCUIElement+FBUtilities.h"

#if !TARGET_OS_TV && !TARGET_OS_WATCH

static const uint32_t FBHIDConsumerUsagePage = 0x0C;
static const uint32_t FBHIDHomeUsage = 0x40;
static const uint32_t FBHIDPowerUsage = 0x30;
static const uint32_t FBHIDOrientationUsagePage = 0xff61;
static const uint32_t FBHIDOrientationUsage = 0x5b;
static const CFOptionFlags FBHIDBinarySerialization = 1;
static const NSTimeInterval FBHIDButtonPressDuration = 0.03;
static const NSTimeInterval FBHIDRotationTimeout = 5.0;

typedef struct {
  CFTypeRef (*createKeyboardEvent)(CFAllocatorRef, uint64_t, uint32_t, uint32_t, Boolean, uint32_t);
  CFTypeRef (*createVendorEvent)(CFAllocatorRef, uint64_t, uint32_t, uint32_t,
                               uint32_t, uint8_t *, CFIndex, uint32_t);
  CFTypeRef (*createClient)(CFAllocatorRef);
  void (*dispatchEvent)(CFTypeRef, CFTypeRef);
  void (*setTimestamp)(CFTypeRef, uint64_t);
  CFDataRef (*serialize)(CFTypeRef, CFOptionFlags);
} FBHIDAPI;

static const FBHIDAPI *FBGetHIDAPI(void)
{
  static FBHIDAPI api;
  static dispatch_once_t onceToken;
  dispatch_once(&onceToken, ^{
    // Keep the framework loaded for the lifetime of the cached function pointers.
    void *handle = dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", RTLD_LAZY);
    if (NULL == handle) {
      return;
    }
    api.createKeyboardEvent = (CFTypeRef (*)(CFAllocatorRef, uint64_t, uint32_t, uint32_t, Boolean, uint32_t))
      dlsym(handle, "IOHIDEventCreateKeyboardEvent");
    api.createVendorEvent = (CFTypeRef (*)(CFAllocatorRef, uint64_t, uint32_t, uint32_t,
                                         uint32_t, uint8_t *, CFIndex, uint32_t))
      dlsym(handle, "IOHIDEventCreateVendorDefinedEvent");
    api.createClient = (CFTypeRef (*)(CFAllocatorRef))dlsym(handle, "IOHIDEventSystemClientCreate");
    api.dispatchEvent = (void (*)(CFTypeRef, CFTypeRef))dlsym(handle, "IOHIDEventSystemClientDispatchEvent");
    api.setTimestamp = (void (*)(CFTypeRef, uint64_t))dlsym(handle, "IOHIDEventSetTimeStamp");
    api.serialize = (CFDataRef (*)(CFTypeRef, CFOptionFlags))dlsym(handle, "IOCFSerialize");
  });
  // Each operation checks only the symbols it needs.
  return &api;
}

static void FBLogUnverifiedPhysicalHIDOperation(NSString *operation)
{
#if !TARGET_OS_SIMULATOR
  [FBLogger logFmt:@"HID %@ on physical hinged devices: not verified yet.", operation];
#endif
}

static BOOL FBWaitForStableInterfaceOrientation(XCUIApplication *application,
                                               UIInterfaceOrientation orientation)
{
  [application fb_waitUntilStableWithTimeout:FBConfiguration.sharedInstance.animationCoolOffTimeout];
  return application.interfaceOrientation == orientation;
}

// Workaround tracked in https://github.com/appium/appium/issues/22857.
// Reproduced on the iOS 27.1 Duo simulator with Xcode 27.1: XCTest's Home
// request returns without backgrounding the app. This direct HID path is also
// enabled on physical hinged devices, but Home behavior there is unverified.
// The affected Xcode/runtime version range has not been established. Remove
// the Home workaround when XCTest-only presses pass testHomeButtonBackgroundsApplication
// on supported Duo environments, including physical devices and both panels.
static BOOL FBPressHIDButton(uint32_t usage, NSError **error)
{
  const FBHIDAPI *api = FBGetHIDAPI();
  if (NULL == api->createKeyboardEvent || NULL == api->createClient
      || NULL == api->dispatchEvent || NULL == api->setTimestamp) {
    return [[FBErrorBuilder.builder withDescription:@"The button HID APIs are unavailable"] buildError:error];
  }
  CFTypeRef client = api->createClient(kCFAllocatorDefault);
  CFTypeRef down = api->createKeyboardEvent(kCFAllocatorDefault, mach_absolute_time(),
                                           FBHIDConsumerUsagePage, usage, YES, 0);
  CFTypeRef up = api->createKeyboardEvent(kCFAllocatorDefault, mach_absolute_time(),
                                           FBHIDConsumerUsagePage, usage, NO, 0);
  BOOL ready = NULL != client && NULL != down && NULL != up;
  if (ready) {
    // Dispatch a short paired press. Holding Power for 0.5s invokes Siri on Duo.
    api->dispatchEvent(client, down);
    [NSThread sleepForTimeInterval:FBHIDButtonPressDuration];
    api->setTimestamp(up, mach_absolute_time());
    api->dispatchEvent(client, up);
  }
  if (NULL != down) {
    CFRelease(down);
  }
  if (NULL != up) {
    CFRelease(up);
  }
  if (NULL != client) {
    CFRelease(client);
  }
  return ready || [[FBErrorBuilder.builder
                    withDescription:@"Cannot create the button HID event or client"] buildError:error];
}

// Work around XCTest updating Duo device orientation without rotating the app.
// Reproduction details and checks required before removing this workaround:
// https://github.com/appium/appium-ios/pull/81
static BOOL FBDispatchHIDOrientation(UIDeviceOrientation orientation)
{
  XCUIScreen *screen = [FBScreen currentScreenWithError:nil];
  if (nil == screen) {
    return NO;
  }
  // The inner panel's native orientation is a quarter turn from the cover.
  // Values match DeviceKit's orientation-picker-control (Xcode 27.1).
  NSString *value;
  switch (orientation) {
    case UIDeviceOrientationPortrait:
      value = screen.isMainScreen ? @"portrait" : @"landscape-left";
      break;
    case UIDeviceOrientationLandscapeLeft:
      value = screen.isMainScreen ? @"landscape-left" : @"pud";
      break;
    case UIDeviceOrientationLandscapeRight:
      value = screen.isMainScreen ? @"landscape-right" : @"portrait";
      break;
    case UIDeviceOrientationPortraitUpsideDown:
      value = screen.isMainScreen ? @"pud" : @"landscape-right";
      break;
    case UIDeviceOrientationUnknown:
    case UIDeviceOrientationFaceUp:
    case UIDeviceOrientationFaceDown:
    default:
      return NO;
  }

  const FBHIDAPI *api = FBGetHIDAPI();
  if (NULL == api->serialize || NULL == api->createVendorEvent
      || NULL == api->createClient || NULL == api->dispatchEvent) {
    return NO;
  }
  NSDictionary *payload = @{
    @"provider": @"com.apple.Virtualization.VirtualMachines",
    @"source": @"orientation-picker-control",
    @"type": @"enum",
    @"value": value,
  };
  // The receiver expects IOCF binary serialization, not a binary plist.
  CFDataRef data = api->serialize((__bridge CFTypeRef)payload, FBHIDBinarySerialization);
  if (NULL == data) {
    return NO;
  }
  CFTypeRef event = api->createVendorEvent(kCFAllocatorDefault, mach_absolute_time(),
                                         FBHIDOrientationUsagePage, FBHIDOrientationUsage, 0,
                                         (uint8_t *)CFDataGetBytePtr(data), CFDataGetLength(data), 0);
  CFRelease(data);
  CFTypeRef client = api->createClient(kCFAllocatorDefault);
  BOOL canDispatch = NULL != event && NULL != client;
  if (canDispatch) {
    api->dispatchEvent(client, event);
  }
  if (NULL != event) {
    CFRelease(event);
  }
  if (NULL != client) {
    CFRelease(client);
  }
  return canDispatch;
}

#endif

@implementation FBHIDOperations

+ (BOOL)appliesToDevice:(XCUIDevice *)device
{
#if !TARGET_OS_TV && !TARGET_OS_WATCH
  return device.fb_supportsHingeAngleReading;
#else
  return NO;
#endif
}

#if !TARGET_OS_TV && !TARGET_OS_WATCH
+ (BOOL)pressHomeWithError:(NSError **)error
{
  FBLogUnverifiedPhysicalHIDOperation(@"Home");
  return FBPressHIDButton(FBHIDHomeUsage, error);
}

+ (BOOL)pressLockWithError:(NSError **)error
{
  FBLogUnverifiedPhysicalHIDOperation(@"lock");
  // XCTest locking times out on Duo / iOS 27.1 (Xcode 27.1). Remove this
  // workaround once XCTest alone passes testRepeatedLockUnlockAcrossHingePoses.
  return FBPressHIDButton(FBHIDPowerUsage, error);
}

+ (BOOL)rotateToOrientation:(UIDeviceOrientation)orientation
     interfaceOrientation:(UIInterfaceOrientation)interfaceOrientation
{
  FBLogUnverifiedPhysicalHIDOperation(@"rotation");
  XCUIApplication *application = XCUIApplication.fb_activeApplication;
  // Point-based detection can select the cover SpringBoard while the app
  // is on the inner panel. Prefer the session's active application.
  application = FBSession.activeSession.activeApplication ?: application;
  // Keep XCTest's reported device orientation in sync on the simulator too.
  [XCUIDevice sharedDevice].orientation = orientation;
  // On Duo simulators XCTest can update its value without rotating the app.
  // Use the same direct HID path on simulators and physical hinged devices.
  if (!FBDispatchHIDOrientation(orientation)) {
    return NO;
  }
  // Event dispatch is asynchronous. Check the app, not just XCTest's value.
  if (![[[FBRunLoopSpinner new] timeout:FBHIDRotationTimeout] spinUntilTrue:^BOOL{
    return application.interfaceOrientation == interfaceOrientation;
  }]) {
    return NO;
  }
  return FBWaitForStableInterfaceOrientation(application, interfaceOrientation);
}
#endif

@end
