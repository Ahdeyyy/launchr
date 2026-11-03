package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestWindowSizeFitsDisplay(t *testing.T) {
	for _, tc := range []struct {
		value string
		w, h  int
	}{{"small", 640, 420}, {"medium", 760, 520}, {"large", 960, 680}, {"", 760, 520}, {"unknown", 760, 520}} {
		w, h := launcherWindowSize(tc.value)
		if w != tc.w || h != tc.h {
			t.Fatalf("%q: got %dx%d, want %dx%d", tc.value, w, h, tc.w, tc.h)
		}
		for _, area := range []mygo.Rectangle{
			{X: -1920, Y: 100, Width: 1920, Height: 1080},
			{Width: 800, Height: 600},
			{Width: 600, Height: 400},
		} {
			b := launcherWindowBounds(tc.value, area)
			if b.Width != min(w, area.Width) || b.Height != min(h, area.Height) ||
				b.X < area.X || b.Y < area.Y || b.X+b.Width > area.X+area.Width || b.Y+b.Height > area.Y+area.Height {
				t.Fatalf("%q: bounds %+v outside display %+v", tc.value, b, area)
			}
		}
	}
}

func TestWindowSizeSelectionPersistenceAndCancel(t *testing.T) {
	a := &app{tab: "Apps", prefs: settings{Vault: "Work", Density: "compact", Pins: []string{"pinned"}}}
	a.openSettings()
	tt := ui.NewTester(a.view, 640, 420)
	if a.windowSizeEdit != "medium" {
		t.Fatal("old preferences must default to Medium")
	}
	tt.Key(0, ui.KeyTab) // Density group.
	tt.Key(0, ui.KeyTab) // Window size group.
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if a.windowSizeEdit != "large" || a.prefs.WindowSize != "" {
		t.Fatalf("selection did not stay in draft: %q / %q", a.windowSizeEdit, a.prefs.WindowSize)
	}
	s := testStore(t)
	if err := s.save("settings", "main", a.settingsDraft()); err != nil {
		t.Fatal(err)
	}
	var loaded settings
	if err := s.load("settings", "main", &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.WindowSize != "large" || loaded.Density != "compact" || loaded.Vault != "Work" || len(loaded.Pins) != 1 {
		t.Fatalf("preferences lost on save: %+v", loaded)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	a.openSettings()
	tt.Frame()
	if a.windowSizeEdit != "medium" {
		t.Fatal("cancel did not discard window size")
	}
	a.back()
	a.prefs = loaded
	a.openSettings()
	tt.Frame()
	if a.windowSizeEdit != "large" {
		t.Fatal("saved size was not restored on reopening settings")
	}
}

func TestWindowSizeSettingsLayout(t *testing.T) {
	for _, size := range []string{"small", "medium", "large"} {
		for _, density := range []string{"compact", "comfortable", "spacious"} {
			t.Run(size+"/"+density, func(t *testing.T) {
				a := &app{tab: "Apps", prefs: settings{WindowSize: size, Density: density}}
				a.openSettings()
				w, h := launcherWindowSize(size)
				tt := ui.NewTester(a.view, w, h)
				tt.Key(0, ui.KeyTab)
				tt.Key(0, ui.KeyTab)
				tt.Frame()
				tt.Advance(time.Second)
				for _, label := range []string{"Small", "Medium", "Large", "Save"} {
					r, ok := tt.Find(label)
					if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > float32(w) || r.Y+r.H > float32(h) {
						t.Fatalf("%s clipped: %+v", label, r)
					}
				}
				if dir := os.Getenv("LAUNCHR_RENDER"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("window-%s-%s.png", size, density)))
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()
					if err := png.Encode(f, tt.Image()); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}
