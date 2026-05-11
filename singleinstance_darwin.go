//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <stdlib.h>

// Returns 1 if another instance is already running, 0 otherwise.
// When another instance is found its window is raised before returning.
int checkAndRaiseExistingInstance(void) {
    NSString *bundleID = [[NSBundle mainBundle] bundleIdentifier];

    // When running as a plain binary (no bundle) bundleIdentifier is nil.
    // Fall back to the process name so we can still match instances.
    if (!bundleID || bundleID.length == 0) {
        bundleID = [[NSProcessInfo processInfo] processName];
    }

    NSArray<NSRunningApplication *> *running =
        [NSRunningApplication runningApplicationsWithBundleIdentifier:bundleID];

    NSRunningApplication *self_ = [NSRunningApplication currentApplication];
    for (NSRunningApplication *app in running) {
        if (![app isEqual:self_]) {
            // Raise the other instance's windows so the user sees it.
            [app activateWithOptions:NSApplicationActivateAllWindows];
            return 1;
        }
    }
    return 0;
}
*/
import "C"
import (
	"fmt"
	"os"
)

func enforceSingleInstance() {
	if C.checkAndRaiseExistingInstance() != 0 {
		fmt.Fprintln(os.Stderr, "OnnxTracker is already running.")
		os.Exit(0)
	}
}
