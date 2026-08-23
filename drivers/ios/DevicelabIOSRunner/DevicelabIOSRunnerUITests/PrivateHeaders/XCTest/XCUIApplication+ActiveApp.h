// Private XCTest surface for frontmost-app detection, class-dump style
// like the neighboring headers. The accessibility interface reports the
// active applications' accessibility elements (with PIDs), and
// XCUIApplication can be constructed from a PID — together they let the
// runner target whatever is on screen when the caller names no bundle,
// instead of activating its own placeholder host app. Same mechanism
// WebDriverAgent has used across Xcode releases.

#import <XCTest/XCTest.h>

@interface XCAccessibilityElement : NSObject
@property (readonly) pid_t processIdentifier;
@end

// Implemented by -[XCUIDevice accessibilityInterface] (XCUIAccessibilityInterface).
@protocol DLAccessibilityActiveApps <NSObject>
- (NSArray<XCAccessibilityElement *> *)activeApplications;
@end

// libproc: resolves a (simulator) process id to its executable path —
// simulator processes are host processes, so the path leads to the .app
// bundle and its Info.plist names the bundle identifier.
int proc_pidpath(int pid, void *buffer, uint32_t buffersize);
