package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSettingsCardsRiseInAndSettle(t *testing.T) {
	a := &app{tab: "Apps"}
	a.openSettings()
	tt := ui.NewTester(a.view, 760, 520)
	start, ok := tt.Find("Vault name")
	if !ok {
		t.Fatal("missing settings form")
	}
	tt.Advance(100 * time.Millisecond)
	mid, _ := tt.Find("Vault name")
	tt.Advance(time.Second)
	end, _ := tt.Find("Vault name")
	if !(start.Y > mid.Y && mid.Y > end.Y) {
		t.Fatalf("card did not rise into place: %v -> %v -> %v", start.Y, mid.Y, end.Y)
	}
	if tt.FramePending() {
		t.Fatal("entrance kept requesting frames after it settled")
	}
}

func TestReducedMotionSkipsEntrance(t *testing.T) {
	a := &app{tab: "Apps"}
	a.openSettings()
	tt := ui.NewTester(a.view, 760, 520)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	tt.Frame()
	first, _ := tt.Find("Vault name")
	tt.Advance(time.Second)
	settled, _ := tt.Find("Vault name")
	if first != settled {
		t.Fatalf("content moved with reduced motion: %+v -> %+v", first, settled)
	}
}

func TestLoadingBarWaitsBeforeShowing(t *testing.T) {
	a := &app{tab: "Clipboard", busy: true}
	tt := ui.NewTester(a.view, 760, 520)
	line, ok := tt.Find("Search")
	if !ok {
		t.Fatal("missing search")
	}
	// The hairline sits just below the search row.
	y := int(line.Y+line.H) + 4
	bright := func() bool {
		img := tt.Image()
		for x := 0; x < 760; x++ {
			for dy := 0; dy < 24; dy++ {
				r, _, _, _ := img.At(x, y+dy).RGBA()
				if r > 120*257 {
					return true
				}
			}
		}
		return false
	}
	tt.Advance(50 * time.Millisecond)
	if bright() {
		t.Fatal("loading bar flashed before the delay")
	}
	tt.Advance(400 * time.Millisecond)
	if !bright() {
		t.Fatal("loading bar did not appear for a slow search")
	}
	a.busy = false
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	if bright() {
		t.Fatal("loading bar stayed after the search finished")
	}
}

func TestStatusMessageSlidesIn(t *testing.T) {
	a := &app{tab: "Apps", rows: []result{{ID: "answer", Title: "100", Kind: "answer", Value: "100"}}}
	tt := ui.NewTester(a.view, 760, 520)
	a.status = "Copied result"
	tt.Frame()
	start, ok := tt.Find("Copied result")
	if !ok {
		t.Fatal("missing status")
	}
	tt.Advance(time.Second)
	end, _ := tt.Find("Copied result")
	if start.Y <= end.Y {
		t.Fatalf("status did not rise into place: %v -> %v", start.Y, end.Y)
	}
}
