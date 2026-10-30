/**
 * Copyright (c) 2015-present, Facebook, Inc.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

#import <XCTest/XCTest.h>
#import <UIKit/UIKit.h>
#import "FBIntegrationTestCase.h"
#import "XCUIDevice+FBRotation.h"
#import "XCUIDevice+FBHinge.h"
#import "FBConfiguration.h"
#import "FBScreen.h"
#import "FBSession.h"

@interface XCUIDeviceRotationTests : FBIntegrationTestCase
@property (nonatomic, strong) FBSession *session;

@end

@implementation XCUIDeviceRotationTests

- (void)setUp
{
  [super setUp];
  [self launchApplication];
  self.session = [FBSession initWithApplication:self.testedApplication];
}

- (void)tearDown
{
  [self resetOrientation];
  [self.session kill];
  [super tearDown];
}

- (void)testLandscapeRightOrientation
{
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceInterfaceOrientation:UIDeviceOrientationLandscapeRight];
  XCTAssertTrue(success, @"Device should support LandscapeRight");
  // Device rotation gives opposite interface rotation
  XCTAssertTrue(self.testedApplication.staticTexts[@"LandscapeLeft"].exists);
}

- (void)testLandscapeLeftOrientation
{
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceInterfaceOrientation:UIDeviceOrientationLandscapeLeft];
  XCTAssertTrue(success, @"Device should support LandscapeLeft");
  // Device rotation gives opposite interface rotation
  XCTAssertTrue(self.testedApplication.staticTexts[@"LandscapeRight"].exists);
}

- (void)testLandscapeRightRotation
{
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceRotation:@{
    @"x" : @(0),
    @"y" : @(0),
    @"z" : @(90)
  }];
  XCTAssertTrue(success, @"Device should support LandscapeRight");
  // Device rotation gives opposite interface rotation
  XCTAssertTrue(self.testedApplication.staticTexts[@"LandscapeLeft"].exists);
}

- (void)testLandscapeLeftRotation
{
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceRotation:@{
    @"x" : @(0),
    @"y" : @(0),
    @"z" : @(270)
  }];
  XCTAssertTrue(success, @"Device should support LandscapeLeft");
  // Device rotation gives opposite interface rotation
  XCTAssertTrue(self.testedApplication.staticTexts[@"LandscapeRight"].exists);
}

- (void)testGetDeviceOrientationInPortrait
{
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceInterfaceOrientation:UIDeviceOrientationPortrait];
  XCTAssertTrue(success, @"Device should support Portrait");
  XCTAssertEqualObjects([XCUIDevice sharedDevice].fb_deviceOrientation, @"UIDeviceOrientationPortrait");
}

- (void)testGetDeviceOrientationInLandscapeLeft
{
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceInterfaceOrientation:UIDeviceOrientationLandscapeLeft];
  XCTAssertTrue(success, @"Device should support LandscapeLeft");
  XCTAssertEqualObjects([XCUIDevice sharedDevice].fb_deviceOrientation, @"UIDeviceOrientationLandscapeLeft");
}

- (void)testGetDeviceOrientationInLandscapeRight
{
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceInterfaceOrientation:UIDeviceOrientationLandscapeRight];
  XCTAssertTrue(success, @"Device should support LandscapeRight");
  XCTAssertEqualObjects([XCUIDevice sharedDevice].fb_deviceOrientation, @"UIDeviceOrientationLandscapeRight");
}

- (void)testRotationTiltRotation
{
  UIDeviceOrientation currentRotation = [XCUIDevice sharedDevice].orientation;
  BOOL success = [[XCUIDevice sharedDevice] fb_setDeviceRotation:@{
    @"x" : @(15),
    @"y" : @(0),
    @"z" : @(0)}
  ];
  XCTAssertFalse(success, @"Device should not support tilt");
  XCTAssertEqual(currentRotation, [XCUIDevice sharedDevice].orientation, @"Device doesnt support tilt, should be at previous orientation");
}

- (void)testRotationOnSelectedDuoDisplay
{
#if TARGET_OS_SIMULATOR
  XCUIDevice *device = XCUIDevice.sharedDevice;
  XCTSkipIf(!device.fb_supportsHingeAngleReading || !device.fb_canAttemptSimulatedHingeAngleInjection,
            @"Requires hinge angle reading and injection support");
  NSNumber *originalDisplay = FBConfiguration.sharedInstance.currentDisplayId;
  NSNumber *originalAngle = [device fb_getSimulatedHingeAngle:nil];
  XCTAssertNotNil(originalAngle);
  @try {
    for (NSNumber *angle in @[@0, @90, @180]) {
      XCTAssertTrue([device fb_setSimulatedHingeAngle:angle.doubleValue error:nil]);
      [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:3]];
      for (NSDictionary *screen in [FBScreen screensWithError:nil]) {
        if ([screen[@"isMain"] boolValue] == (angle.intValue == 0)) {
          FBConfiguration.sharedInstance.currentDisplayId = screen[@"displayId"];
        }
      }
      [self.testedApplication activate];
      NSArray *orientations = @[@(UIDeviceOrientationLandscapeLeft),
                                @(UIDeviceOrientationLandscapeRight),
                                @(UIDeviceOrientationPortrait)];
      NSArray *labels = @[@"LandscapeRight", @"LandscapeLeft", @"Portrait"];
      for (NSUInteger i = 0; i < orientations.count; i++) {
        UIDeviceOrientation orientation = [orientations[i] integerValue];
        XCTAssertTrue([device fb_setDeviceInterfaceOrientation:orientation]);
        XCTAssertTrue([self.testedApplication.staticTexts[labels[i]] waitForExistenceWithTimeout:5]);
        XCTAssertEqual(device.orientation, orientation);
      }
      BOOL rotated = [device fb_setDeviceRotation:@{@"x": @0, @"y": @0, @"z": @270}];
      XCTAssertTrue(rotated);
      XCTAssertTrue(self.testedApplication.staticTexts[@"LandscapeRight"].exists);
    }
  } @finally {
    [device fb_setSimulatedHingeAngle:0 error:nil];
    [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:3]];
    FBConfiguration.sharedInstance.currentDisplayId = nil;
    [device fb_setDeviceInterfaceOrientation:UIDeviceOrientationPortrait];
    if (nil != originalAngle) {
      [device fb_setSimulatedHingeAngle:originalAngle.doubleValue error:nil];
    }
    FBConfiguration.sharedInstance.currentDisplayId = originalDisplay;
  }
#endif
}

@end
