#import <Cocoa/Cocoa.h>

NSWindow *mainWindow;
NSTextView *logTextView;

@interface AppDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate>
@end

@implementation AppDelegate

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    NSRect frame = NSMakeRect(0, 0, 400, 300);

    mainWindow = [[NSWindow alloc] initWithContentRect:frame
                                             styleMask:(NSWindowStyleMaskTitled |
                                                       NSWindowStyleMaskClosable |
                                                       NSWindowStyleMaskMiniaturizable |
                                                       NSWindowStyleMaskResizable)
                                               backing:NSBackingStoreBuffered
                                                 defer:NO];
    [mainWindow setTitle:@"Onnx Tracker"];
    [mainWindow setDelegate:self];
    [mainWindow center];

    NSView *contentView = [mainWindow contentView];
    [contentView setAutoresizesSubviews:YES];

    // Scroll view — fills entire content area, resizes with window
    NSScrollView *scrollView = [[NSScrollView alloc] initWithFrame:NSMakeRect(10, 10, frame.size.width - 20, frame.size.height - 20)];
    [scrollView setHasVerticalScroller:YES];
    [scrollView setHasHorizontalScroller:NO];
    [scrollView setAutohidesScrollers:YES];
    [scrollView setBorderType:NSBezelBorder];
    [scrollView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];

    // Text view for logs
    logTextView = [[NSTextView alloc] initWithFrame:[scrollView bounds]];
    [logTextView setEditable:NO];
    [logTextView setFont:[NSFont fontWithName:@"Menlo" size:12]];
    [logTextView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
    [scrollView setDocumentView:logTextView];
    [contentView addSubview:scrollView];

    // Initial text
    [logTextView setString:@"Onnx Tracker\n\nStarting server...\n\n"];

    // Start minimized
    [mainWindow miniaturize:nil];

    extern void onCocoaAppReady();
    onCocoaAppReady();
}


- (void)windowWillClose:(NSNotification *)notification {
    [NSApp terminate:nil];
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
    return YES;
}

@end

void runCocoaApp(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        AppDelegate *delegate = [[AppDelegate alloc] init];
        [app setDelegate:delegate];
        [app setActivationPolicy:NSApplicationActivationPolicyRegular];
        [app activateIgnoringOtherApps:YES];
        [app run];
    }
}

void appendTextToLog(const char* text) {
    @autoreleasepool {
        NSString *newText = [NSString stringWithUTF8String:text];
        dispatch_async(dispatch_get_main_queue(), ^{
            NSString *currentText = [logTextView string];
            NSString *combined = [currentText stringByAppendingString:newText];
            [logTextView setString:combined];

            // Scroll to bottom
            NSRange range = NSMakeRange([[logTextView string] length], 0);
            [logTextView scrollRangeToVisible:range];
        });
    }
}
