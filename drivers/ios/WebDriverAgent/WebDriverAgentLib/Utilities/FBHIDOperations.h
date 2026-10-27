/**
 * Copyright (c) 2015-present, Facebook, Inc.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

#import <XCTest/XCTest.h>
#import <UIKit/UIKit.h>

/**
 HID paths for Home, lock/unlock, and rotation on hinged iOS devices.

 Remove each workaround once its XCTest-only path passes the corresponding regression
 test on supported runtimes and both panels: testHomeButtonBackgroundsApplication,
 testRepeatedLockUnlockAcrossHingePoses, and testRotationOnSelectedDuoDisplay.
 Physical-device Home, lock/unlock, and rotation behavior require verification.

 Once all paths are obsolete, remove the FBHIDOperations branches and imports from
 XCUIDevice+FBHelpers.m and XCUIDevice+FBRotation.m, then delete this module and its
 Xcode project entries. Hinge control in XCUIDevice+FBHinge is independent.

 Tracking: https://github.com/appium/appium/issues/22857
 Rotation: https://github.com/appium/appium-ios/pull/81
 */
@interface FBHIDOperations : NSObject

// All operations apply to hinged iOS devices, including physical devices.
+ (BOOL)appliesToDevice:(XCUIDevice *)device;

#if !TARGET_OS_TV && !TARGET_OS_WATCH
// Call only after the corresponding applicability check succeeds.
+ (BOOL)pressHomeWithError:(NSError **)error;
+ (BOOL)pressLockWithError:(NSError **)error;
// interfaceOrientation is the UIKit equivalent of the requested device orientation.
+ (BOOL)rotateToOrientation:(UIDeviceOrientation)orientation
     interfaceOrientation:(UIInterfaceOrientation)interfaceOrientation;
#endif

@end
