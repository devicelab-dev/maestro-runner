/**
 * Copyright (c) 2015-present, Facebook, Inc.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

#import <XCTest/XCTest.h>
#import <math.h>

#import "FBIntegrationTestCase.h"
#import "FBImageUtils.h"
#import "FBMacros.h"
#import "FBTestMacros.h"
#import "XCUIApplication.h"
#import "XCUIApplication+FBHelpers.h"
#import "XCUIDevice+FBHelpers.h"
#import "XCUIDevice+FBHinge.h"
#import "XCUIDevice+FBRotation.h"
#import "XCUIScreen.h"

@interface XCUIDeviceHelperTests : FBIntegrationTestCase
@end

@implementation XCUIDeviceHelperTests

- (void)restorePortraitOrientation
{
  if ([XCUIDevice sharedDevice].orientation != UIDeviceOrientationPortrait) {
    [[XCUIDevice sharedDevice] fb_setDeviceInterfaceOrientation:UIDeviceOrientationPortrait];
  }
}

- (void)setUp
{
  [super setUp];
  [self launchApplication];
  [self restorePortraitOrientation];
}

- (void)tearDown
{
  [self restorePortraitOrientation];
  [super tearDown];
}

- (void)testScreenshot
{
  NSError *error = nil;
  NSData *screenshotData = [[XCUIDevice sharedDevice] fb_screenshotWithError:&error];
  XCTAssertNotNil(screenshotData);
  XCTAssertNil(error);
  XCTAssertTrue(FBIsPngImage(screenshotData));

  UIImage *screenshot = [UIImage imageWithData:screenshotData];
  XCTAssertNotNil(screenshot);

  XCUIScreen *mainScreen = XCUIScreen.mainScreen;
  UIImage *screenshotExact = ((XCUIScreenshot *)mainScreen.screenshot).image;
  XCTAssertEqualWithAccuracy(screenshotExact.size.height * mainScreen.scale,
                             screenshot.size.height,
                             FLT_EPSILON);
  XCTAssertEqualWithAccuracy(screenshotExact.size.width * mainScreen.scale,
                             screenshot.size.width,
                             FLT_EPSILON);
}

- (void)testLandscapeScreenshot
{
  XCTAssertTrue([[XCUIDevice sharedDevice] fb_setDeviceInterfaceOrientation:UIDeviceOrientationLandscapeLeft]);
  NSError *error = nil;
  NSData *screenshotData = [[XCUIDevice sharedDevice] fb_screenshotWithError:&error];
  XCTAssertNotNil(screenshotData);
  XCTAssertTrue(FBIsPngImage(screenshotData));
  XCTAssertNil(error);

  UIImage *screenshot = [UIImage imageWithData:screenshotData];
  XCTAssertNotNil(screenshot);
  XCTAssertTrue(screenshot.size.width > screenshot.size.height);

  XCUIScreen *mainScreen = XCUIScreen.mainScreen;
  UIImage *screenshotExact = ((XCUIScreenshot *)mainScreen.screenshot).image;
  CGSize realMainScreenSize = screenshotExact.size.height > screenshot.size.width
    ? CGSizeMake(screenshotExact.size.height * mainScreen.scale, screenshotExact.size.width * mainScreen.scale)
    : CGSizeMake(screenshotExact.size.width * mainScreen.scale, screenshotExact.size.height * mainScreen.scale);
  XCTAssertEqualWithAccuracy(realMainScreenSize.height, screenshot.size.height, FLT_EPSILON);
  XCTAssertEqualWithAccuracy(realMainScreenSize.width, screenshot.size.width, FLT_EPSILON);
}

- (void)testWifiAddress
{
  NSString *adderss = [XCUIDevice sharedDevice].fb_wifiIPAddress;
  if (!adderss) {
    XCTSkip(@"No WiFi IP address available on this device");
  }
  NSRange range = [adderss rangeOfString:@"^([0-9]{1,3})\\.([0-9]{1,3})\\.([0-9]{1,3})\\.([0-9]{1,3})" options:NSRegularExpressionSearch];
  XCTAssertTrue(range.location != NSNotFound);
}

- (void)testGoToHomeScreen
{
  NSError *error;
  XCTAssertTrue([[XCUIDevice sharedDevice] fb_goToHomescreenWithError:&error]);
  XCTAssertNil(error);
  FBAssertWaitTillBecomesTrue([XCUIApplication fb_activeApplication].icons[@"Safari"].exists);
}

- (void)testHomeButtonBackgroundsApplication
{
  XCUIDevice *device = XCUIDevice.sharedDevice;
  BOOL hasHinge = NO;
#if TARGET_OS_SIMULATOR
  hasHinge = device.fb_canAttemptSimulatedHingeAngleInjection;
#endif
  NSNumber *originalAngle = hasHinge ? [device fb_getSimulatedHingeAngle:nil] : nil;
  if (hasHinge && nil == originalAngle) {
    XCTFail(@"Cannot read the original hinge angle");
    return;
  }
  BOOL (^setAngleAndWait)(NSNumber *) = ^BOOL(NSNumber *angle) {
    return [device fb_setSimulatedHingeAngle:angle.doubleValue error:nil]
      && [[[FBRunLoopSpinner new] timeout:5] spinUntilTrue:^BOOL{
        NSNumber *reading = [device fb_getSimulatedHingeAngle:nil];
        return nil != reading && fabs(reading.doubleValue - angle.doubleValue) < 1;
      }];
  };
  @try {
    for (NSNumber *angle in (hasHinge ? @[@0, @180] : @[@0])) {
      if (hasHinge) {
        if (!setAngleAndWait(angle)) {
          XCTFail(@"Hinge angle did not reach %@", angle);
          return;
        }
      }
      for (NSUInteger attempt = 0; attempt < 2; attempt++) {
        [self.testedApplication activate];
        XCTAssertEqual(self.testedApplication.state, XCUIApplicationStateRunningForeground);
        NSError *error = nil;
        XCTAssertTrue([device fb_pressButton:@"home" forDuration:nil error:&error]);
        XCTAssertNil(error);
        XCTAssertTrue([[[FBRunLoopSpinner new] timeout:5] spinUntilTrue:^BOOL{
          return self.testedApplication.state == XCUIApplicationStateRunningBackground
            || self.testedApplication.state == XCUIApplicationStateRunningBackgroundSuspended;
        }]);
        XCTAssertTrue([XCUIApplication.fb_activeApplication.bundleID isEqualToString:@"com.apple.springboard"]);
      }
    }
  } @finally {
    if (nil != originalAngle) {
      XCTAssertTrue(setAngleAndWait(originalAngle));
    }
  }
}

- (void)testLockUnlockScreen
{
  XCTAssertFalse([[XCUIDevice sharedDevice] fb_isScreenLocked]);
  NSError *error;
  XCTAssertTrue([[XCUIDevice sharedDevice] fb_lockScreen:&error]);
  XCTAssertTrue([[XCUIDevice sharedDevice] fb_isScreenLocked]);
  XCTAssertNil(error);
  XCTAssertTrue([[XCUIDevice sharedDevice] fb_unlockScreen:&error]);
  XCTAssertFalse([[XCUIDevice sharedDevice] fb_isScreenLocked]);
  XCTAssertNil(error);
}

- (void)testRepeatedLockUnlockAcrossHingePoses
{
  XCTSkipIf(self.class.isRunningInCI, @"Repeated lock/unlock is unreliable on slow CI hosts");
  XCUIDevice *device = XCUIDevice.sharedDevice;
  BOOL hasHinge = NO;
#if TARGET_OS_SIMULATOR
  hasHinge = device.fb_canAttemptSimulatedHingeAngleInjection;
#endif
  NSNumber *originalAngle = hasHinge ? [device fb_getSimulatedHingeAngle:nil] : nil;
  if (hasHinge) {
    XCTAssertNotNil(originalAngle);
  }
  BOOL (^setAngleAndWait)(NSNumber *) = ^BOOL(NSNumber *angle) {
    return [device fb_setSimulatedHingeAngle:angle.doubleValue error:nil]
      && [[[FBRunLoopSpinner new] timeout:5] spinUntilTrue:^BOOL{
        NSNumber *reading = [device fb_getSimulatedHingeAngle:nil];
        return nil != reading && fabs(reading.doubleValue - angle.doubleValue) < 1;
      }];
  };
  @try {
    for (NSNumber *angle in (hasHinge ? @[@0, @90, @180] : @[@0])) {
      if (hasHinge) {
        XCTAssertTrue(setAngleAndWait(angle));
      }
      for (NSUInteger attempt = 0; attempt < 3; attempt++) {
        XCTAssertFalse(device.fb_isScreenLocked);
        NSError *error = nil;
        XCTAssertTrue([device fb_lockScreen:&error]);
        XCTAssertNil(error);
        XCTAssertTrue(device.fb_isScreenLocked);
        // Repeating lock must not toggle the screen back to unlocked.
        XCTAssertTrue([device fb_lockScreen:&error]);
        XCTAssertTrue(device.fb_isScreenLocked);
        XCTAssertTrue([device fb_unlockScreen:&error]);
        XCTAssertNil(error);
        XCTAssertFalse(device.fb_isScreenLocked);
        XCTAssertTrue([device fb_unlockScreen:&error]);
        XCTAssertFalse(device.fb_isScreenLocked);
      }
    }
  } @finally {
    [device fb_unlockScreen:nil];
    if (nil != originalAngle) {
      XCTAssertTrue(setAngleAndWait(originalAngle));
    }
  }
}

- (void)testUrlSchemeActivation
{
  if (SYSTEM_VERSION_LESS_THAN(@"16.4")) {
    XCTSkip(@"Requires iOS 16.4+");
  }

  NSError *error;
  XCTAssertTrue([XCUIDevice.sharedDevice fb_openUrl:@"https://apple.com" error:&error]);
  FBAssertWaitTillBecomesTrue([XCUIApplication.fb_activeApplication.bundleID isEqualToString:@"com.apple.mobilesafari"]);
  XCTAssertNil(error);
}

- (void)testUrlSchemeActivationWithApp
{
  if (SYSTEM_VERSION_LESS_THAN(@"16.4")) {
    XCTSkip(@"Requires iOS 16.4+");
  }

  NSError *error;
  XCTAssertTrue([XCUIDevice.sharedDevice fb_openUrl:@"https://apple.com"
                                    withApplication:@"com.apple.mobilesafari"
                                              error:&error]);
  FBAssertWaitTillBecomesTrue([XCUIApplication.fb_activeApplication.bundleID isEqualToString:@"com.apple.mobilesafari"]);
  XCTAssertNil(error);
}

#if !TARGET_OS_TV
- (void)testSimulatedLocationSetup
{
  if (SYSTEM_VERSION_LESS_THAN(@"16.4")) {
    XCTSkip(@"Requires iOS 16.4+");
  }

  CLLocation *simulatedLocation = [[CLLocation alloc] initWithLatitude:50 longitude:50];
  NSError *error;
  XCTAssertTrue([XCUIDevice.sharedDevice fb_setSimulatedLocation:simulatedLocation error:&error]);
  XCTAssertNil(error);
  CLLocation *currentLocation = [XCUIDevice.sharedDevice fb_getSimulatedLocation:&error];
  XCTAssertNil(error);
  XCTAssertNotNil(currentLocation);
  XCTAssertEqualWithAccuracy(simulatedLocation.coordinate.latitude, currentLocation.coordinate.latitude, 0.1);
  XCTAssertEqualWithAccuracy(simulatedLocation.coordinate.longitude, currentLocation.coordinate.longitude, 0.1);
  XCTAssertTrue([XCUIDevice.sharedDevice fb_clearSimulatedLocation:&error]);
  XCTAssertNil(error);
  currentLocation = [XCUIDevice.sharedDevice fb_getSimulatedLocation:&error];
  XCTAssertNil(error);
  XCTAssertNotEqualWithAccuracy(simulatedLocation.coordinate.latitude, currentLocation.coordinate.latitude, 0.1);
  XCTAssertNotEqualWithAccuracy(simulatedLocation.coordinate.longitude, currentLocation.coordinate.longitude, 0.1);
}
#endif

- (void)testPressingUnsupportedButton
{
  NSError *error;
  NSNumber *duration = nil;
  XCTAssertFalse([XCUIDevice.sharedDevice fb_pressButton:@"volumeUpp"
                                             forDuration:duration
                                                   error:&error]);
  XCTAssertNotNil(error);
}

- (void)testPressingSupportedButton
{
  NSError *error;
  XCTAssertTrue([XCUIDevice.sharedDevice fb_pressButton:@"home"
                                            forDuration:nil
                                                  error:&error]);
  XCTAssertNil(error);
}

- (void)testPressingDeviceSpecificButton
{
  NSError *error;
  BOOL hasActionButton = [XCUIDevice.sharedDevice fb_hasButton:@"action"];
  BOOL didPressButton = [XCUIDevice.sharedDevice fb_pressButton:@"action"
                                                     forDuration:nil
                                                           error:&error];
  if (hasActionButton) {
    XCTAssertTrue(didPressButton);
    XCTAssertNil(error);
  } else {
    XCTAssertFalse(didPressButton);
    XCTAssertNotNil(error);
  }
}

- (void)testPressingSupportedButtonNumber
{
  NSError *error;
  XCTAssertTrue([XCUIDevice.sharedDevice fb_pressButton:@"home"
                                            forDuration:[NSNumber numberWithDouble:1.0]
                                                  error:&error]);
  XCTAssertNil(error);
}

- (void)testLongPressHomeButton
{
  NSError *error;
  // kHIDPage_Consumer = 0x0C
  // kHIDUsage_Csmr_Menu = 0x40
  XCTAssertTrue([XCUIDevice.sharedDevice fb_performIOHIDEventWithPage:0x0C
                                                                usage:0x40
                                                             duration:1.0
                                                                error:&error]);
  XCTAssertNil(error);
}

- (void)testAppearance
{
  if (SYSTEM_VERSION_LESS_THAN(@"15.0")) {
    XCTSkip(@"Requires iOS 15.0+");
  }
  NSError *error;
  XCTAssertTrue([XCUIDevice.sharedDevice fb_setAppearance:FBUIInterfaceAppearanceDark error:&error]);
  XCTAssertNil(error);
}

@end
