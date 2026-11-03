package main

import (
	"sync"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestCtrlShiftEnterRunsAppAsAdministrator(t *testing.T) {
	var mu sync.Mutex
	var launched []string
	runElevated = func(path string) error {
		mu.Lock()
		defer mu.Unlock()
		launched = append(launched, path)
		return nil
	}
	defer func() { runElevated = launchElevated }()

	a := &app{tab: "Apps", rows: []result{
		{ID: "w", Title: "Report — Word", Detail: "Open window", Kind: "window"},
		{ID: "code", Title: "Code", Detail: "Application", Kind: "app", App: mygo.InstalledApp{ID: "Microsoft.VisualStudioCode", Name: "Code"}},
	}}
	tt := ui.NewTester(a.view, 760, 520)
	if !tt.HasText("Switch") || tt.HasText("Run as admin") {
		t.Fatal("open windows cannot be elevated")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyEnter)
	a.wg.Wait()
	if len(launched) != 0 {
		t.Fatalf("elevated a running window: %v", launched)
	}

	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if !tt.HasText("Run as admin") {
		t.Fatal("installed app needs a Run as admin action")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyEnter)
	a.wg.Wait()
	if len(launched) != 1 || launched[0] != `shell:AppsFolder\Microsoft.VisualStudioCode` {
		t.Fatalf("launched %v", launched)
	}
	if a.status != "" {
		t.Fatalf("Ctrl+Shift+Enter also submitted a normal launch: %q", a.status)
	}
}

func TestElevationIgnoredWhileComposing(t *testing.T) {
	called := false
	runElevated = func(string) error { called = true; return nil }
	defer func() { runElevated = launchElevated }()
	a := &app{tab: "Apps", rows: []result{{ID: "code", Title: "Code", Kind: "app", App: mygo.InstalledApp{Path: "x"}}}}
	a.openCommand("capture")
	tt := ui.NewTester(a.view, 760, 520)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyEnter)
	a.wg.Wait()
	if called {
		t.Fatal("Ctrl+Shift+Enter in Quick capture must append to the daily note, not elevate")
	}
}
