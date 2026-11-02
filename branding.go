package main

import (
	"embed"
	"fmt"

	"github.com/egoist/mygo/ui"
)

// An open capsule frames a floating input bar. The tray uses an optically
// tuned version of the same silhouette for small Windows notification icons.
//
//go:embed resources/mark.svg
var appMarkSVG []byte

var appMark = ui.MustParseSVG(appMarkSVG)

//go:embed resources/icon.svg
var appIconSVG []byte

//go:embed resources/tray.png
var appTrayPNG []byte

//go:embed resources/tray-*.png
var appTraySizes embed.FS

func sizedTrayIcon(size int) []byte {
	data, err := appTraySizes.ReadFile(fmt.Sprintf("resources/tray-%d.png", size))
	if err != nil {
		return appTrayPNG
	}
	return data
}
