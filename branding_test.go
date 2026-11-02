package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRenderBranding(t *testing.T) {
	dir := os.Getenv("LAUNCHR_RENDER")
	if dir == "" {
		t.Skip("set LAUNCHR_RENDER to export icon assets")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{16, 32, 256, 1024} {
		for _, tray := range []bool{false, true} {
			svg := ui.MustParseSVG(appIconSVG)
			name := fmt.Sprintf("icon-%d.png", size)
			if tray {
				svg = appMark
				name = fmt.Sprintf("tray-%d.png", size)
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.Root().Background(ui.Transparent)
				if tray {
					ui.Icon(c, svg).Size(float32(size), float32(size)).TextColor(ui.Hex("#f2f2f0"))
				} else {
					ui.Image(c, svg).Size(float32(size), float32(size))
				}
			}, size, size)
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
