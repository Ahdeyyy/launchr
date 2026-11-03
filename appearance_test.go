package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSelectionHasNoOverlappingHighlight(t *testing.T) {
	a := &app{tab: "Apps", rows: []result{
		{ID: "window", Title: "Window One", Detail: "Open window", Kind: "window"},
		{ID: "app", Title: "App One", Detail: "Application", Kind: "app"},
	}}
	tt := ui.NewTester(a.view, 760, 520)
	if tt.HasText("YOUR NEXT MOVE") {
		t.Fatal("tagline returned")
	}
	if !tt.HasText("Running") || !tt.HasText("Switch") {
		t.Fatal("running window needs a distinct state and action")
	}
	rect, ok := tt.Find("Window One")
	if !ok {
		t.Fatal("missing row")
	}
	img := tt.Image()
	// This strip held the bright outer selection slivers in the old design.
	// It must contain only the dark canvas and the neutral rounded highlight.
	for x := 10; x < 20; x++ {
		for y := int(rect.Y) - 8; y < int(rect.Y)+24; y++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 96*257 || g > 96*257 || b > 96*257 {
				t.Fatalf("bright selection rim at %d,%d", x, y)
			}
		}
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if !tt.HasText("Launch") {
		t.Fatal("installed app needs a Launch action")
	}
}

func TestAcrylicKeepsOpaqueFallback(t *testing.T) {
	a := &app{tab: "Apps"}
	tt := ui.NewTester(a.view, 760, 520)
	_, _, _, opaque := tt.Image().At(400, 400).RGBA()
	if opaque != 65535 {
		t.Fatal("unsupported desktops need an opaque fallback")
	}
	tt.SetVibrancy(true)
	_, _, _, glass := tt.Image().At(400, 400).RGBA()
	if glass == 0 || glass >= opaque {
		t.Fatalf("material must show through a readable tint: %d", glass)
	}
	tt.SetVibrancy(false)
	_, _, _, opaque = tt.Image().At(400, 400).RGBA()
	if opaque != 65535 {
		t.Fatal("fallback did not return after material disappeared")
	}
}

func TestSettingsActionsStayVisibleAtMinimumSize(t *testing.T) {
	a := &app{tab: "Apps"}
	a.openSettings()
	tt := ui.NewTester(a.view, 640, 420)
	for _, label := range []string{"Back", "Save", "Obsidian vault"} {
		r, ok := tt.Find(label)
		if !ok || r.Y < 0 || r.Y+r.H > 420 {
			t.Fatalf("%s is clipped: %+v", label, r)
		}
	}
	tt.Type("Vault")
	if a.vaultEdit != "Vault" {
		t.Fatal("vault did not receive focus")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.settingsOpen {
		t.Fatal("Escape did not leave settings")
	}
}

func TestResultsLeaveScrollbarGutter(t *testing.T) {
	a := &app{tab: "Apps"}
	for i := 0; i < 30; i++ {
		a.rows = append(a.rows, result{ID: fmt.Sprint(i), Title: fmt.Sprintf("Window %02d", i), Kind: "window"})
	}
	tt := ui.NewTester(a.view, 640, 420)
	for i := 0; i < 3; i++ {
		r, ok := tt.Find("Running")
		if !ok || r.X+r.W > 640-24 {
			t.Fatalf("result touches scrollbar gutter: %+v", r)
		}
		// All neutral surfaces, including selection, must stay achromatic.
		title, _ := tt.Find(a.rows[a.chosen].Title)
		red, green, blue, _ := tt.Image().At(350, int(title.Y)+6).RGBA()
		if red != green || green != blue {
			t.Fatal("selection has a color cast")
		}
		tt.Key(0, ui.KeyPageDown)
		tt.Frame()
	}
}
