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

func TestDirectActionRanksAheadOfDescriptions(t *testing.T) {
	a := &app{tab: "Apps", query: "daily"}
	a.search(false)
	if len(a.rows) == 0 || a.rows[0].Kind != "daily" {
		t.Fatalf("daily action was not first: %+v", a.rows)
	}
}

func TestRenderCommandScreens(t *testing.T) {
	dir := os.Getenv("LAUNCHR_RENDER")
	if dir == "" {
		t.Skip("set LAUNCHR_RENDER to save UI renders")
	}
	for _, screen := range []string{"commands", "capture", "settings"} {
		a := &app{tab: "Apps"}
		a.search(false)
		if screen != "commands" {
			a.openCommand(screen)
		}
		tt := ui.NewTester(a.view, 640, 420)
		tt.Frame()
		// Let entrance motion settle before capturing.
		tt.Advance(time.Second)
		if screen == "capture" {
			if rect, ok := tt.Find("Append to daily note"); !ok || rect.Y+rect.H > 420 {
				t.Fatalf("capture actions clipped: %+v", rect)
			}
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, screen+".png"))
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

func TestCommandNavigationRestoresSearch(t *testing.T) {
	a := &app{tab: "Apps"}
	a.search(false)
	tt := ui.NewTester(a.view, 760, 520)
	tt.Type("clipboard")
	if len(a.rows) != 1 {
		t.Fatalf("rows: %+v", a.rows)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.tab != "Clipboard" || a.query != "" {
		t.Fatalf("did not enter clipboard: %s %q", a.tab, a.query)
	}
	tt.Type("needle")
	if a.query != "needle" {
		t.Fatalf("search lost focus: %q", a.query)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.tab != "Apps" || a.query != "clipboard" || a.chosen != 0 {
		t.Fatalf("did not restore search: %+v", a.history)
	}
	tt.Type(" history")
	if a.query != "clipboard history" {
		t.Fatalf("restored search lost focus: %q", a.query)
	}
}

func TestCaptureAndSettingsReceiveKeyboardFocus(t *testing.T) {
	a := &app{tab: "Apps"}
	a.search(false)
	tt := ui.NewTester(a.view, 760, 520)
	tt.Type("quick capture")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !a.composing {
		t.Fatalf("Enter did not open capture: tab=%s query=%q chosen=%d history=%+v rows=%+v", a.tab, a.query, a.chosen, a.history, a.rows)
	}
	tt.Type("New thought")
	if a.noteTitle != "New thought" || a.query != "quick capture" {
		t.Fatalf("focus: title=%q search=%q", a.noteTitle, a.query)
	}
	tt.Key(0, ui.KeyTab)
	tt.Type("First line")
	tt.Key(0, ui.KeyEnter)
	tt.Type("Second line")
	if a.noteBody != "First line\nSecond line" {
		t.Fatalf("multiline input: %q", a.noteBody)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.composing || a.query != "quick capture" {
		t.Fatal("capture did not return to search")
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	tt.Type("Personal")
	if !a.settingsOpen || a.vaultEdit != "Personal" {
		t.Fatalf("settings focus: %q", a.vaultEdit)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Type(" note")
	if a.query != "quick capture note" {
		t.Fatalf("search focus after settings: %q", a.query)
	}
}

func TestKeyboardSelectionAndScroll(t *testing.T) {
	a := &app{tab: "Apps"}
	for i := 0; i < 60; i++ {
		a.apps = append(a.apps, mygo.InstalledApp{ID: fmt.Sprint(i), Name: fmt.Sprintf("Example App %02d", i)})
	}
	a.search(false)
	tt := ui.NewTester(a.view, 760, 520)
	tt.Key(0, ui.KeyDown)
	if a.chosen != 1 {
		t.Fatalf("Down moved to %d", a.chosen)
	}
	tt.Key(0, ui.KeyPageDown)
	if a.chosen != 7 {
		t.Fatalf("PageDown moved to %d", a.chosen)
	}
	for i := 0; i < 12; i++ {
		tt.Key(0, ui.KeyPageDown)
	}
	tt.Frame()
	if a.chosen != len(a.rows)-1 {
		t.Fatal("selection did not reach last row")
	}
	if rect, ok := tt.Find("Example App 59"); !ok || rect.Y+rect.H > 480 {
		t.Fatalf("selected row not visible: %+v %v", rect, ok)
	}
	tt.Type("Example App 00")
	if a.chosen != 0 || len(a.rows) != 1 {
		t.Fatalf("filter did not reset selection: %d %+v", a.chosen, a.rows)
	}
}

func TestObsidianNestedCommands(t *testing.T) {
	a := &app{tab: "Apps", query: "obsidian"}
	a.search(false)
	tt := ui.NewTester(a.view, 760, 520)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.tab != "Obsidian" || len(a.rows) != 2 || a.rows[0].Kind != "daily" {
		t.Fatalf("Obsidian actions: %+v", a.rows)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !a.composing {
		t.Fatal("nested capture did not open")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.composing || a.tab != "Obsidian" || a.chosen != 1 {
		t.Fatal("did not restore Obsidian selection")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.tab != "Apps" || a.query != "obsidian" {
		t.Fatal("did not restore root search")
	}
}
