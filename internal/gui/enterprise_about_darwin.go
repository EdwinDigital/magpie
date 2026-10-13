//go:build darwin && !nogui

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

static void mpEnterpriseAbout(const char *name, const void *bytes, int size) {
    NSMutableDictionary *options = [NSMutableDictionary dictionary];
    options[NSAboutPanelOptionApplicationName] = [NSString stringWithUTF8String:name];
    NSImage *icon = [[NSImage alloc] initWithData:[NSData dataWithBytes:bytes length:size]];
    if (icon != nil) options[NSAboutPanelOptionApplicationIcon] = icon;
    // Version and legal attribution continue to come from the signed bundle.
    [NSApp orderFrontStandardAboutPanelWithOptions:options];
    [icon release];
}
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func showEnterpriseAbout() {
	application.InvokeSync(func() {
		name := C.CString(brandName())
		defer C.free(unsafe.Pointer(name))
		logo := brandAppIcon()
		if len(logo) == 0 {
			logo = appIconLarge
		}
		C.mpEnterpriseAbout(name, unsafe.Pointer(&logo[0]), C.int(len(logo)))
	})
}
