package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDensitySettingsKeyboardAndPersistence(t *testing.T) {
	a := &app{tab: "Apps", prefs: settings{Vault: "Work", Pins: []string{"pinned"}}}
	a.openSettings()
	tt := ui.NewTester(a.view, 640, 420)
	if a.densityEdit != "comfortable" {
		t.Fatal("missing preference must default to comfortable")
	}
	// Vault is focused on entry; the selected density radio follows it.
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if a.densityEdit != "spacious" {
		t.Fatalf("keyboard did not select density: %q", a.densityEdit)
	}
	if a.prefs.Density != "" {
		t.Fatal("draft changed saved preferences")
	}
	draft := a.settingsDraft()
	s := testStore(t)
	if err := s.save("settings", "main", draft); err != nil {
		t.Fatal(err)
	}
	var loaded settings
	if err := s.load("settings", "main", &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Density != "spacious" || loaded.Vault != "Work" || len(loaded.Pins) != 1 {
		t.Fatalf("preferences did not persist: %+v", loaded)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	a.openSettings()
	tt.Frame()
	if a.densityEdit != "comfortable" {
		t.Fatal("cancelled density was not discarded")
	}
	if launcherDensity("unknown") != launcherDensity("") {
		t.Fatal("unknown density needs a compatible default")
	}
}

func TestDensityLayoutAndNavigation(t *testing.T) {
	previousHeight := float32(0)
	for _, density := range []string{"compact", "comfortable", "spacious"} {
		t.Run(density, func(t *testing.T) {
			a := &app{tab: "Apps", prefs: settings{Density: density}}
			for i := 0; i < 40; i++ {
				a.rows = append(a.rows, result{ID: fmt.Sprint(i), Title: fmt.Sprintf("Result %02d", i), Kind: "window"})
			}
			tt := ui.NewTester(a.view, 640, 420)
			first, ok := tt.Find("Result 00")
			if !ok {
				t.Fatal("missing first row")
			}
			second, ok := tt.Find("Result 01")
			if !ok {
				t.Fatal("missing second row")
			}
			height := second.Y - first.Y
			if height <= previousHeight {
				t.Fatalf("density did not increase row spacing: %v <= %v", height, previousHeight)
			}
			previousHeight = height
			if dir := os.Getenv("LAUNCHR_RENDER"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, "density-"+density+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 7; i++ {
				tt.Key(0, ui.KeyPageDown)
			}
			tt.Frame()
			if a.chosen != 39 {
				t.Fatal("could not reach last result")
			}
			r, ok := tt.Find("Switch  ↵")
			if !ok || r.Y < 0 || r.Y+r.H > 385 || r.X+r.W > 640-24 {
				t.Fatalf("selected result clipped or touching scrollbar: %+v", r)
			}
		})
	}
}

func TestDensityPreviewsSettingsAndCancelRestoresLayout(t *testing.T) {
	a := &app{tab: "Apps", prefs: settings{Density: "comfortable"}}
	tt := ui.NewTester(a.view, 760, 520)
	header, _ := tt.Find("Launchr")
	a.openSettings()
	tt.Frame()
	comfortable, ok := tt.Find("Vault name")
	if !ok {
		t.Fatal("missing settings form")
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	spacious, ok := tt.Find("Vault name")
	if !ok || spacious.Y <= comfortable.Y {
		t.Fatalf("spacious selection did not preview settings spacing: %+v -> %+v", comfortable, spacious)
	}
	tt.Key(0, ui.KeyLeft)
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	compact, ok := tt.Find("Vault name")
	if !ok || compact.Y >= comfortable.Y {
		t.Fatalf("compact selection did not preview settings spacing: %+v -> %+v", comfortable, compact)
	}
	if a.prefs.Density != "comfortable" {
		t.Fatal("preview changed saved preference")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	restored, ok := tt.Find("Launchr")
	if !ok || restored != header {
		t.Fatalf("cancel did not restore saved layout: %+v -> %+v", header, restored)
	}
}
