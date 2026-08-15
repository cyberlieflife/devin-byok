//go:build darwin

#import <Cocoa/Cocoa.h>

void devin_order_out(void *window) {
    if (window != NULL) {
        [(NSWindow *)window orderOut:nil];
    }
}

void devin_make_key_and_order_front(void *window) {
    if (window != NULL) {
        [(NSWindow *)window makeKeyAndOrderFront:nil];
    }
}

void devin_activate_and_focus(void *window) {
    NSApplication *app = [NSApplication sharedApplication];
    [app setActivationPolicy:NSApplicationActivationPolicyRegular];
    [app activateIgnoringOtherApps:YES];
    if (window != NULL) {
        NSWindow *nsWindow = (NSWindow *)window;
        [nsWindow makeKeyAndOrderFront:nil];
        [nsWindow makeFirstResponder:nsWindow.contentView];
    }
}

int devin_is_miniaturized(void *window) {
    if (window == NULL) {
        return 0;
    }
    return [(NSWindow *)window isMiniaturized] ? 1 : 0;
}
