//go:build windows

package main

import "testing"

func TestClipboardListenerLifecycle(t *testing.T) {
	stop, err := watchClipboard(func() {})
	if err != nil {
		t.Fatal(err)
	}
	stop()
	stop()
}
