//go:build !linux

package main

import "unsafe"

// The desktop shell targets Linux (GTK/WebKitGTK). These stubs keep the module
// buildable elsewhere; they have no effect.

func setAppIdentity() {}

func setWindowIcon(unsafe.Pointer) {}
