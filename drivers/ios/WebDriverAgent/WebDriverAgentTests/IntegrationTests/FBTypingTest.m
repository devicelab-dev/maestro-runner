/**
 * Copyright (c) 2015-present, Facebook, Inc.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

#import <XCTest/XCTest.h>

#import "FBConfiguration.h"
#import "FBIntegrationTestCase.h"
#import "XCUIElement.h"
#import "XCUIElement+FBTyping.h"

@interface FBTypingTest : FBIntegrationTestCase
@end

@implementation FBTypingTest

- (void)setUp
{
  [super setUp];
  [self launchApplication];
  [self goToAttributesPage];
}

- (void)tearDown
{
  FBConfiguration.sharedInstance.keyboardWaitTimeout = 0;
  [super tearDown];
}

- (void)testTextTyping
{
  NSString *text = @"Happy typing";
  XCUIElement *textField = self.testedApplication.textFields[@"aIdentifier"];
  NSError *error;
  XCTAssertTrue([textField fb_typeText:text shouldClear:NO error:&error]);
  XCTAssertNil(error);
  XCTAssertEqualObjects(textField.value, text);
}

- (void)testTextTypingOnFocusedElement
{
  NSString *text = @"Happy typing";
  XCUIElement *textField = self.testedApplication.textFields[@"aIdentifier"];
  [textField tap];
  XCTAssertTrue(textField.hasKeyboardFocus);
  NSError *error;
  XCTAssertTrue([textField fb_typeText:text shouldClear:NO error:&error]);
  XCTAssertNil(error);
  XCTAssertTrue([textField fb_typeText:text shouldClear:NO error:&error]);
  XCTAssertNil(error);
  NSString *expectedText = [NSString stringWithFormat:@"%@%@", text, text];
  XCTAssertEqualObjects(textField.value, expectedText);
}

- (void)testTextTypingImmediatelyAfterFocus
{
  // The field is a storyboard UIKit text field. This passes with the pause off,
  // which is the default, and it does not reproduce a dropped character.
  NSString *text = @"TheQuickBrownFox123!";
  XCUIElement *textField = self.testedApplication.textFields[@"aIdentifier"];
  [textField tap];
  NSError *error;
  XCTAssertTrue([textField fb_typeText:text shouldClear:NO error:&error]);
  XCTAssertNil(error);
  XCTAssertEqualObjects(textField.value, text);
}

- (void)testTextTypingWithKeyboardWaitTimeout
{
  // The field is a storyboard UIKit text field. This does not reproduce a dropped character.
  FBConfiguration.sharedInstance.keyboardWaitTimeout = 0.5;
  NSString *text = @"TheQuickBrownFox123!";
  XCUIElement *textField = self.testedApplication.textFields[@"aIdentifier"];
  [textField tap];
  NSError *error;
  XCTAssertTrue([textField fb_typeText:text shouldClear:NO error:&error]);
  XCTAssertNil(error);
  XCTAssertEqualObjects(textField.value, text);
}

- (void)testTextClearing
{
  XCUIElement *textField = self.testedApplication.textFields[@"aIdentifier"];
  [textField tap];
  [textField typeText:@"Happy typing"];
  XCTAssertTrue([textField.value length] > 0);
  NSError *error;
  XCTAssertTrue([textField fb_clearTextWithError:&error]);
  XCTAssertNil(error);
  XCTAssertEqualObjects(textField.value, @"");
  XCTAssertTrue([textField fb_typeText:@"Happy typing" shouldClear:YES error:&error]);
  XCTAssertTrue([textField fb_typeText:@"Happy typing 2" shouldClear:YES error:&error]);
  XCTAssertEqualObjects(textField.value, @"Happy typing 2");
  XCTAssertNil(error);
}

@end
