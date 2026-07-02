#import "SyntheticTyping.h"
#import <UIKit/UIKit.h>
#import "PrivateHeaders/XCTest/XCSynthesizedEventRecord.h"
#import "PrivateHeaders/XCTest/XCPointerEventPath.h"
#import "PrivateHeaders/XCTest/XCUIDevice.h"

BOOL DLSendSyntheticTyping(NSString *text, NSUInteger typingSpeed, NSError * _Nullable * _Nullable outError) {
  if (text.length == 0) {
    return YES;
  }
  if (typingSpeed == 0) {
    typingSpeed = 60; // matches WDA default
  }

  XCSynthesizedEventRecord *record =
    [[XCSynthesizedEventRecord alloc] initWithName:@"DLSyntheticTyping"];
  XCPointerEventPath *path = [[XCPointerEventPath alloc] initForTextInput];
  [path typeText:text atOffset:0.0 typingSpeed:typingSpeed shouldRedact:NO];
  [record addPointerEventPath:path];

  id device = [XCUIDevice sharedDevice];
  id synthesizer = [device valueForKey:@"eventSynthesizer"];
  if (synthesizer == nil) {
    if (outError) {
      *outError = [NSError errorWithDomain:@"DevicelabIOSRunner"
                                       code:1
                                   userInfo:@{NSLocalizedDescriptionKey:
                                              @"XCUIDevice.eventSynthesizer unavailable"}];
    }
    return NO;
  }

  __block BOOL done = NO;
  __block NSError *innerError = nil;

  SEL sel = NSSelectorFromString(@"synthesizeEvent:completion:");
  if (![synthesizer respondsToSelector:sel]) {
    if (outError) {
      *outError = [NSError errorWithDomain:@"DevicelabIOSRunner"
                                       code:2
                                   userInfo:@{NSLocalizedDescriptionKey:
                                              @"eventSynthesizer does not support synthesizeEvent:completion:"}];
    }
    return NO;
  }

  NSMethodSignature *sig = [synthesizer methodSignatureForSelector:sel];
  NSInvocation *inv = [NSInvocation invocationWithMethodSignature:sig];
  inv.target = synthesizer;
  inv.selector = sel;
  XCSynthesizedEventRecord *recordArg = record;
  [inv setArgument:&recordArg atIndex:2];
  void (^completion)(BOOL, NSError *) = ^(BOOL success, NSError *invokeError) {
    if (invokeError != nil) {
      innerError = invokeError;
    }
    done = YES;
  };
  [inv setArgument:&completion atIndex:3];
  [inv invoke];

  // Spin the run loop until the completion handler fires.
  NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:30.0];
  while (!done && [deadline timeIntervalSinceNow] > 0) {
    [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
                             beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
  }

  if (!done) {
    if (outError) {
      *outError = [NSError errorWithDomain:@"DevicelabIOSRunner"
                                       code:3
                                   userInfo:@{NSLocalizedDescriptionKey:
                                              @"synthesizeEvent timed out"}];
    }
    return NO;
  }

  if (innerError != nil) {
    if (outError) {
      *outError = innerError;
    }
    return NO;
  }
  return YES;
}
