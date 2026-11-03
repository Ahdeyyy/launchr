package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Worst-case fixtures: realistic values (or the stored limit) for every
// field the launcher renders. They enter through a.rows, a.status and the
// preview fields, the same boundary the demo data uses.
var copiedAt = time.Date(2026, 11, 2, 16, 45, 0, 0, time.UTC)

func worstApps() []result {
	long := mygo.InstalledApp{ID: "vscode", Name: "Microsoft Visual Studio Code Insiders (User Setup, ARM64)"}
	return []result{
		{ID: "w1", Title: "Q3 Forecast — Aleksandra Wiśniewska-Kowalczyk — Final (v12) — Shared with bartholomew.fitzgerald@northwind-industries-holdings.example.com — Excel", Detail: "Open window", Kind: "window"},
		{ID: "w2", Title: "📌 会議メモ – Северо-западный регион – Notion", Detail: "Open window", Kind: "window"},
		{ID: "vscode", Title: long.Name, Detail: "Pinned", Kind: "app", App: long},
		{ID: "jo", Title: "Jo", Detail: "Application", Kind: "app"},
		{ID: "wa", Title: "واتساب للأعمال", Detail: "Application", Kind: "app"},
		{ID: "capture", Title: "Quick Capture", Detail: "Obsidian · Create a note or append to daily note", Kind: "command", Value: "capture"},
	}
}

func worstClips() []result {
	url := "https://northwind-industries-holdings.example.com/reports/2026/q3/aleksandra.wisniewska-kowalczyk/forecast?version=12&share=eyJhbGciOiJIUzI1NiJ9…"
	return []result{
		{ID: "1", Title: url, Detail: "Nov 2, 16:45   159 B", Kind: "clip", Clip: clip{ID: 1, Kind: "text", Size: 159, Time: copiedAt}},
		{ID: "2", Title: "", Detail: "Nov 2, 16:44   3 B", Kind: "clip", Clip: clip{ID: 2, Kind: "text", Size: 3, Time: copiedAt}},
		{ID: "3", Title: "Image", Detail: "Nov 2, 16:40   48.0 MB", Kind: "clip", Clip: clip{ID: 3, Kind: "image", Size: 48 << 20, Time: copiedAt}},
		{ID: "4", Title: "Ok", Detail: "Nov 2, 16:39   2 B", Kind: "clip", Clip: clip{ID: 4, Kind: "text", Size: 2, Time: copiedAt}},
	}
}

func worstStatus() string {
	return `exec: "obsidian": executable file not found in %PATH%. Install the Obsidian CLI, register it in PATH, and enable it in Obsidian → Settings → General.`
}

type worstScreen struct {
	name  string
	setup func(a *app)
}

var worstScreens = []worstScreen{
	{"apps-demo", func(a *app) {
		a.rows = []result{{ID: "window", Title: "Launchr — Visual Studio Code", Detail: "Open window", Kind: "window"}, {ID: "obsidian", Title: "Obsidian", Detail: "Pinned", Kind: "app"}, {ID: "terminal", Title: "Terminal", Detail: "Application", Kind: "app"}}
	}},
	{"apps-worst", func(a *app) { a.rows = worstApps(); a.status = worstStatus() }},
	{"apps-worst-pinned-selected", func(a *app) {
		a.rows = worstApps()
		a.chosen = 2
		a.prefs.Pins = []string{"vscode"}
	}},
	{"apps-empty", func(a *app) { a.query = "zzqx" }},
	{"apps-one", func(a *app) { a.query = "jo"; a.rows = worstApps()[3:4] }},
	{"apps-1000", func(a *app) {
		for i := range 1284 {
			a.rows = append(a.rows, result{ID: fmt.Sprint(i), Title: fmt.Sprintf("Application %04d", i), Detail: "Application", Kind: "app"})
		}
	}},
	{"clipboard-worst", func(a *app) {
		a.tab = "Clipboard"
		a.rows = worstClips()
		a.more = true
		a.previewID = 1
		a.previewText = a.rows[0].Title
	}},
	{"clipboard-worst-empty-title", func(a *app) {
		a.tab = "Clipboard"
		a.rows = worstClips()
		a.chosen = 1
		a.previewID = 2
		a.previewText = "\n\n\n"
		a.paused = true
	}},
	{"calculator-worst", func(a *app) {
		a.tab = "Calculator"
		a.query = "123456789 * 987654321 * 1e300"
		a.rows = []result{{ID: "answer", Title: "1.2193263111263526e+317", Detail: "Copy result", Kind: "answer", Value: "1.2193263111263526e+317"}}
	}},
	{"currency-worst", func(a *app) {
		a.tab = "Calculator"
		a.query = "1250000000 USD to NGN"
		a.rows = []result{{ID: "answer", Title: "1,935,412,500,000.00 NGN", Detail: "Cached rate from Oct 28, 2026 · 1 USD = 1,548.33 NGN · refreshing", Kind: "answer", Value: "1935412500000.00"}}
	}},
	{"obsidian-worst", func(a *app) {
		a.tab = "Obsidian"
		a.query = "regional"
		a.rows = []result{{ID: "n1", Title: "Meeting notes with Bartholomew Fitzgerald and the regional leads (follow-up, revised)", Detail: "Projects/Northwind Industries Holdings/2026/Q3/Regional reviews/Meeting notes with Bartholomew Fitzgerald and the regional leads (follow-up, revised).md", Kind: "note"}}
	}},
	{"capture-worst", func(a *app) {
		a.openCommand("capture")
		a.noteTitle = "Meeting notes with Bartholomew Fitzgerald and the regional leads (follow-up, revised)"
		a.noteBody = "x"
		a.status = worstStatus()
	}},
	{"settings-worst", func(a *app) {
		a.shortcut = "Alt+Shift+Space"
		a.openSettings()
		a.vaultEdit = "Aleksandra Wiśniewska-Kowalczyk — Northwind Industries Holdings (Work)"
	}},
}

func TestRenderWorstCase(t *testing.T) {
	dir := os.Getenv("LAUNCHR_RENDER")
	if dir == "" {
		t.Skip("set LAUNCHR_RENDER to save worst-case renders")
	}
	dir = filepath.Join(dir, "worst")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, size := range []string{"small", "large"} {
		for _, screen := range worstScreens {
			a := &app{tab: "Apps", shortcut: "Alt+Space", prefs: settings{WindowSize: size}}
			screen.setup(a)
			w, h := launcherWindowSize(size)
			tt := ui.NewTester(a.view, w, h)
			tt.Frame()
			tt.Advance(time.Second)
			f, err := os.Create(filepath.Join(dir, size+"-"+screen.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			// Report anything drawn outside the window instead of failing, so
			// every screen renders for review.
			for _, text := range tt.Texts() {
				if r, ok := tt.Find(text); ok && text != "" && (r.X+r.W > float32(w)+0.5 || r.Y+r.H > float32(h)+0.5) && !strings.HasPrefix(screen.name, "apps-1000") {
					t.Logf("%s/%s: %q extends outside the window: %+v", size, screen.name, text, r)
				}
			}
		}
	}
}

func TestRowTextKeepsDistinguishingParts(t *testing.T) {
	for _, tc := range []struct {
		r             result
		title, detail string
		placeholder   bool
	}{
		{result{Kind: "window", Title: "Q3 Forecast — Final (v12) — Excel", Detail: "Open window"}, "Q3 Forecast — Final (v12)", "Excel", false},
		{result{Kind: "window", Title: "Untitled", Detail: "Open window"}, "Untitled", "Open window", false},
		{result{Kind: "clip", Title: " ", Detail: "Nov 2, 16:44   3 B"}, "Whitespace only", "Nov 2, 16:44   3 B", true},
		{result{Kind: "note", Title: "Plan", Detail: "Projects/Northwind/2026/Q3/Plan.md"}, "Plan", "…/2026/Q3", false},
		{result{Kind: "note", Title: "Inbox", Detail: "Inbox.md"}, "Inbox", "Vault root", false},
	} {
		title, detail, placeholder := rowText(tc.r)
		if title != tc.title || detail != tc.detail || placeholder != tc.placeholder {
			t.Errorf("rowText(%q) = %q, %q, %v", tc.r.Title, title, detail, placeholder)
		}
	}
	a := &app{status: worstStatus()}
	if !a.longStatus() {
		t.Fatal("long errors need the wrapping strip")
	}
	if label, _ := a.statusLabel("Launchr"); label != "Launchr" {
		t.Fatalf("long status also crowded the action bar: %q", label)
	}
}
