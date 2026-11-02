//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

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
