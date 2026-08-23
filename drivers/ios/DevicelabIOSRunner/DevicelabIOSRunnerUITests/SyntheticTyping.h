#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

// Local extension (not in upstream agent-device). Synthesizes typing events
// via the private XCSynthesizedEventRecord + XCPointerEventPath +
// XCUIDevice.eventSynthesizer pathway — the same mechanism WebDriverAgent
// uses for FBTypeText. Unlike XCUIElement.typeText / XCUIApplication.typeText,
// this does NOT require XCUIElement.hasKeyboardFocus to be true. That is
// what makes it work for RN SecureTextField on iOS Simulator, where the
// keyboard appears visually but hasKeyboardFocus stays false and the public
// typeText APIs silently drop characters.
//
// Returns YES on success. Writes a NSError into *outError on failure.
BOOL DLSendSyntheticTyping(NSString *text, NSUInteger typingSpeed, NSError * _Nullable * _Nullable outError);

NS_ASSUME_NONNULL_END
