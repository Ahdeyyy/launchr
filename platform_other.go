//go:build !windows

package main

import (
	"fmt"
	"image"
	"os/exec"
)

const maxCaptureBytes = 64 << 20

func nativeIconLoader() (func(string) (image.Image, error), func(), error) {
	return nil, nil, fmt.Errorf("native icons currently support Windows")
}
func readNativeClipboard() (capture, uint32, error) {
	return capture{}, 0, fmt.Errorf("clipboard capture currently supports Windows")
}
func nativeClipboardSequence() uint32             { return 0 }
func normalizeCapture(c capture) (capture, error) { return c, nil }

type appWindow struct {
	Handle      uintptr
	Title, Path string
}

func hideCommand(cmd *exec.Cmd) {}
func openWindows() []appWindow  { return nil }
func activateWindow(w appWindow) error {
	return fmt.Errorf("window switching currently supports Windows")
}
func watchClipboard(notify func()) (func(), error) {
	return nil, fmt.Errorf("clipboard capture currently supports Windows")
}
