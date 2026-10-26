/**
 * Copyright (c) 2015-present, Facebook, Inc.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

#import <XCTest/XCTest.h>

#import <mach/mach_time.h>
#import <math.h>

#import "FBIntegrationTestCase.h"
#import "FBElement.h"
#import "FBSession.h"
#import "FBSessionCommands.h"
#import "FBMacros.h"
#import "FBTestMacros.h"
#import "XCUIApplication.h"
#import "XCUIApplication+FBHelpers.h"
#import "XCUIElement+FBIsVisible.h"
#import "FBXCodeCompatibility.h"
#import "FBConfiguration.h"
#import "FBScreen.h"
#import "FBRunLoopSpinner.h"
#import "XCUIDevice+FBHinge.h"

// Exercise the session-start resume path without creating a second session.
@interface FBSessionCommands (FBActivationTests)
+ (id)activateBackgroundApplication:(XCUIApplication *)app
                          bundleID:(NSString *)bundleID
                        initialUrl:(nullable NSString *)initialUrl;
@end

void calculateMaxTreeDepth(NSDictionary *tree, NSNumber *currentDepth, NSNumber** maxDepth) {
  if (nil == maxDepth) {
    return;
  }

  NSArray *children = tree[@"children"];
  if (nil == children || 0 == children.count) {
    return;
  }
  for (NSDictionary *child in children) {
    if (currentDepth.integerValue > [*maxDepth integerValue]) {
      *maxDepth = currentDepth;
    }
    calculateMaxTreeDepth(child, @(currentDepth.integerValue + 1), maxDepth);
  }
}

@interface XCUIApplicationHelperTests : FBIntegrationTestCase
@end

@implementation XCUIApplicationHelperTests

- (void)setUp
{
  [super setUp];
  [self launchApplication];
}

- (void)testQueringSpringboard
{
  [self goToSpringBoardFirstPage];
  XCTAssertTrue(XCUIApplication.fb_systemApplication.icons[@"Safari"].exists);
  XCTAssertTrue(XCUIApplication.fb_systemApplication.icons[@"Calendar"].firstMatch.exists);
}

- (void)testApplicationTree
{
  NSDictionary *tree = self.testedApplication.fb_tree;
  XCTAssertNotNil(tree);
  NSNumber *maxDepth;
  calculateMaxTreeDepth(tree, @0, &maxDepth);
  XCTAssertGreaterThan(maxDepth.integerValue, 3);
  XCTAssertNotNil(self.testedApplication.fb_accessibilityTree);
}

- (void)testApplicationTreeAttributesFiltering
{
  NSDictionary *applicationTree = [self.testedApplication fb_tree:[NSSet setWithArray:@[@"visible"]]];
  XCTAssertNotNil(applicationTree);
  XCTAssertNil([applicationTree objectForKey:@"isVisible"], @"'isVisible' key should not be present in the application tree");
}

- (void)testDeactivateApplication
{
  NSError *error;
  uint64_t timeStarted = clock_gettime_nsec_np(CLOCK_MONOTONIC_RAW);
  NSTimeInterval backgroundDuration = 5.0;
  XCTAssertTrue([self.testedApplication fb_deactivateWithDuration:backgroundDuration error:&error]);
  NSTimeInterval timeElapsed = (clock_gettime_nsec_np(CLOCK_MONOTONIC_RAW) - timeStarted) / NSEC_PER_SEC;
  XCTAssertNil(error);
  XCTAssertEqualWithAccuracy(timeElapsed, backgroundDuration, 3.0);
  XCTAssertTrue(self.testedApplication.buttons[@"Alerts"].exists);
}

- (void)testInputAfterDeactivate
{
  [self verifyInputAfterBackgroundWithMode:0];
}

- (void)testInputAfterSessionActivate
{
  [self verifyInputAfterBackgroundWithMode:1];
}

- (void)testInputAfterSessionLaunch
{
  [self verifyInputAfterBackgroundWithMode:2];
}

- (void)testInputAfterSessionStartActivate
{
  [self verifyInputAfterBackgroundWithMode:3];
}

- (void)verifyInputAfterBackgroundWithMode:(NSUInteger)mode
{
  XCTSkipIf(self.class.isRunningInCI, @"Immediate input after activation is unreliable on slow CI hosts");
  XCUIDevice *device = XCUIDevice.sharedDevice;
  BOOL canFold = NO;
#if TARGET_OS_SIMULATOR
  canFold = device.fb_canAttemptSimulatedHingeAngleInjection;
#endif
  NSNumber *originalAngle = canFold ? [device fb_getSimulatedHingeAngle:nil] : nil;
  NSNumber *originalDisplay = FBConfiguration.sharedInstance.currentDisplayId;
  if (canFold) {
    XCTAssertNotNil(originalAngle);
  }
  BOOL (^setAngleAndWait)(NSNumber *) = ^BOOL(NSNumber *angle) {
    if (![device fb_setSimulatedHingeAngle:angle.doubleValue error:nil]) {
      return NO;
    }
    return [[[FBRunLoopSpinner new] timeout:5] spinUntilTrue:^BOOL{
      NSNumber *reading = [device fb_getSimulatedHingeAngle:nil];
      return nil != reading && fabs(reading.doubleValue - angle.doubleValue) < 1;
    }];
  };
  FBSession *session = mode != 0 ? [FBSession initWithApplication:self.testedApplication] : nil;
  @try {
    for (NSNumber *angle in (canFold ? @[@0, @90, @180] : @[@0])) {
      if (canFold) {
        XCTAssertTrue(setAngleAndWait(angle), @"Hinge did not reach %@ degrees", angle);
        for (NSDictionary *screen in [FBScreen screensWithError:nil]) {
          if ([screen[@"isMain"] boolValue] == (angle.intValue == 0)) {
            FBConfiguration.sharedInstance.currentDisplayId = screen[@"displayId"];
          }
        }
      }
      for (NSNumber *duration in @[@0, @2]) {
        [self launchApplication];
        NSError *error = nil;
        if (mode != 0) {
          XCTAssertTrue([device fb_goToHomescreenWithError:&error]);
          XCTAssertTrue([[[FBRunLoopSpinner new] timeout:5] spinUntilTrue:^BOOL{
            XCUIApplicationState state = self.testedApplication.state;
            return state == XCUIApplicationStateRunningBackground
              || state == XCUIApplicationStateRunningBackgroundSuspended;
          }]);
          [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:duration.doubleValue]];
          XCUIApplicationState state = self.testedApplication.state;
          XCTAssertTrue(state == XCUIApplicationStateRunningBackground
                        || state == XCUIApplicationStateRunningBackgroundSuspended,
                        @"Expected a background app before activation (angle %@, duration %@), got %lu",
                        angle, duration, (unsigned long)state);
          if (mode == 1) {
            [session activateApplicationWithBundleId:self.testedApplication.bundleID];
          } else if (mode == 2) {
            [session launchApplicationWithBundleId:self.testedApplication.bundleID
                          shouldWaitForQuiescence:nil arguments:nil environment:nil];
          } else {
            XCTAssertNil([FBSessionCommands activateBackgroundApplication:self.testedApplication
                                                                bundleID:self.testedApplication.bundleID
                                                              initialUrl:nil]);
          }
        } else {
          XCTAssertTrue([self.testedApplication fb_deactivateWithDuration:duration.doubleValue error:&error]);
        }
        XCTAssertNil(error);
        // Existence/foreground checks alone miss lost input. Do not add a
        // delay or retry: this first tap must actually open the alerts page.
        [self.testedApplication.buttons[@"Alerts"] tap];
        XCTAssertTrue([self.testedApplication.buttons[FBShowAlertButtonName] waitForExistenceWithTimeout:2]);
      }
    }
  } @finally {
    [session kill];
    BOOL restored = nil == originalAngle || setAngleAndWait(originalAngle);
    FBConfiguration.sharedInstance.currentDisplayId = originalDisplay;
    XCTAssertTrue(restored, @"Hinge did not return to %@ degrees", originalAngle);
  }
}

- (void)testActiveApplication
{
  XCUIApplication *systemApp = XCUIApplication.fb_systemApplication;
  XCTAssertTrue([XCUIApplication fb_activeApplication].buttons[@"Alerts"].fb_isVisible);
  [self goToSpringBoardFirstPage];
  XCTAssertTrue(systemApp.icons[@"Safari"].fb_isVisible);
}

- (void)testActiveElement
{
  [self goToAttributesPage];
  XCTAssertNil(self.testedApplication.fb_activeElement);
  XCUIElement *textField = self.testedApplication.textFields[@"aIdentifier"];
  [textField tap];
  FBAssertWaitTillBecomesTrue(nil != self.testedApplication.fb_activeElement);
  XCTAssertEqualObjects(((id<FBElement>)self.testedApplication.fb_activeElement).wdUID,
                        ((id<FBElement>)textField).wdUID);
}

- (void)testActiveApplicationsInfo
{
  NSArray *appsInfo = [XCUIApplication fb_activeAppsInfo];
  XCTAssertTrue(appsInfo.count > 0);
  BOOL isAppActive = NO;
  for (NSDictionary *appInfo in appsInfo) {
    if ([appInfo[@"bundleId"] isEqualToString:self.testedApplication.bundleID]) {
      isAppActive = YES;
      break;
    }
  }
  XCTAssertTrue(isAppActive);
}

- (void)testTestmanagerdVersion
{
  XCTAssertGreaterThan(FBTestmanagerdVersion(), 0);
}

- (void)testAccessbilityAudit
{
  if (SYSTEM_VERSION_LESS_THAN(@"17.0")) {
    XCTSkip(@"Requires iOS 17.0+");
  }

  NSError *error;
  NSArray *auditIssues1 = [XCUIApplication.fb_activeApplication fb_performAccessibilityAuditWithAuditTypes:~0UL
                                                                                                   error:&error];
  XCTAssertNotNil(auditIssues1);
  XCTAssertNil(error);

  NSMutableSet *set = [NSMutableSet new];
  [set addObject:@"XCUIAccessibilityAuditTypeAll"];
  NSArray *auditIssues2 = [XCUIApplication.fb_activeApplication fb_performAccessibilityAuditWithAuditTypesSet:set.copy
                                                                                                      error:&error];
  // 'elementDescription' is not in this list because it could have
  // different object id's debug description in XCTest.
  NSArray *checkKeys = @[
    @"auditType",
    @"compactDescription",
    @"detailedDescription",
    @"element",
    @"elementAttributes"
  ];

  XCTAssertEqual([auditIssues1 count], [auditIssues2 count]);
  for (int i = 1; i < [auditIssues1 count]; i++) {
    for (NSString *k in checkKeys) {
      XCTAssertEqualObjects(
                            [auditIssues1[i] objectForKey:k],
                            [auditIssues2[i] objectForKey:k]
                            );
    }
  }
  XCTAssertNil(error);
}

@end
