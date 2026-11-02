//go:build windows

package main

// SM_CXSMICON tracks the system notification area's small-icon pixel size.
// Choose the matching optical asset at startup instead of resampling a 32px PNG.
func trayPixelSize() int {
	return int(winCall("GetSystemMetrics", 49))
}
