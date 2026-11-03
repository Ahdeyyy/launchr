package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func testStore(t *testing.T) *store {
	t.Helper()
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}
func TestHistoryPersistenceAndPaging(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"first résumé", "second", "third", "first résumé"} {
		if err := s.add("text", []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	s.db.Close()
	s, err = openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	page, more, err := s.page(context.Background(), "", 0, 2)
	if err != nil || len(page) != 2 || !more || page[0].Preview != "first résumé" {
		t.Fatalf("page=%+v more=%v err=%v", page, more, err)
	}
	next, more, err := s.page(context.Background(), "", page[1].ID, 2)
	if err != nil || len(next) != 1 || more || next[0].Preview != "second" {
		t.Fatalf("next=%+v more=%v err=%v", next, more, err)
	}
	found, _, err := s.page(context.Background(), "RÉSUMÉ", 0, 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("search=%v, %v", found, err)
	}
	if err = s.delete(found[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.data(found[0]); err == nil {
		t.Fatal("deleted payload survives")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = s.page(ctx, "", 0, 10); err == nil {
		t.Fatal("cancelled search ran")
	}
}
func TestHistoryImagesAndFullTextSearch(t *testing.T) {
	s := testStore(t)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.White)
	var b bytes.Buffer
	png.Encode(&b, img)
	if err := s.add("image", b.Bytes()); err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("long text ", 100) + "needle"
	s.add("text", []byte(text))
	found, _, _ := s.page(context.Background(), "needle", 0, 20)
	if len(found) != 1 {
		t.Fatal("search only checked preview")
	}
	all, _, _ := s.page(context.Background(), "", 0, 20)
	for _, item := range all {
		if item.Kind == "image" {
			data, err := s.data(item)
			if err != nil || !bytes.Equal(data, b.Bytes()) {
				t.Fatal("image did not round trip")
			}
		}
	}
}

func TestDeleteStaleClipboardItem(t *testing.T) {
	s := testStore(t)
	s.add("text", []byte("keep me"))
	old, _, _ := s.page(context.Background(), "", 0, 1)
	s.add("text", []byte("keep me"))
	if err := s.delete(old[0]); err == nil {
		t.Fatal("stale deletion succeeded")
	}
	latest, _, _ := s.page(context.Background(), "", 0, 1)
	data, err := s.data(latest[0])
	if err != nil || string(data) != "keep me" {
		t.Fatal("stale deletion removed the current item")
	}
}
func TestCalculator(t *testing.T) {
	for input, want := range map[string]string{"2+3*4": "14", "(240 + 60) / 3": "100", "2^3^2": "512", "-2^2": "-4", "2^-2": "0.25", "50% * 200": "100", "sqrt(81)": "9", "abs(-8)": "8", "1e3 + .5": "1000.5", "12 × 3": "36"} {
		got, err := calculate(input)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v; want %s", input, got, err, want)
		}
	}
	for _, input := range []string{"1/0", "sqrt(-1)", "(1+2", "1 2", "1..2", "exec(foo)", strings.Repeat("(", 140) + "1" + strings.Repeat(")", 140)} {
		if _, err := calculate(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
func TestCurrencyCachingAndFallback(t *testing.T) {
	s := testStore(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/USD/NGN" {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"date":"2026-10-30","base":"USD","quote":"NGN","rate":1500}`))
	}))
	q, ok := parseConversion("100 usd to ngn")
	if !ok {
		t.Fatal("conversion not parsed")
	}
	value, _, err := convertCurrency(context.Background(), s, server.Client(), server.URL, q)
	if err != nil || value != "150000.00 NGN" {
		t.Fatal(value, err)
	}
	convertCurrency(context.Background(), s, server.Client(), server.URL, q)
	if calls != 1 {
		t.Fatal("fresh cache was not used")
	}
	server.Close()
	var r rate
	s.load("rates", "USD/NGN", &r)
	r.Fetched = time.Now().Add(-24 * time.Hour)
	s.save("rates", "USD/NGN", r)
	value, detail, err := convertCurrency(context.Background(), s, &http.Client{Timeout: time.Second}, server.URL, q)
	if err != nil || value != "150000.00 NGN" || !strings.Contains(detail, "Offline") {
		t.Fatal(value, detail, err)
	}
	_, _, err = convertCurrency(context.Background(), s, &http.Client{Timeout: time.Second}, server.URL, conversion{1, "GBP", "NGN"})
	if err == nil {
		t.Fatal("uncached offline request succeeded")
	}
}
func TestObsidianArguments(t *testing.T) {
	got := obsidianArgs("My Vault", "create", "name=Test & notes", "content=hello; $(test)")
	want := []string{"vault=My Vault", "create", "name=Test & notes", "content=hello; $(test)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if got := cliContent("a\r\nb\nc\td"); got != `a\nb\nc\td` {
		t.Fatalf("content=%q", got)
	}
}
func TestLauncherKeyboardAndViews(t *testing.T) {
	a := &app{tab: "Apps", apps: []mygo.InstalledApp{{ID: "notes", Name: "Obsidian", Path: "notes"}, {ID: "code", Name: "Visual Studio Code", Path: "code"}}}
	a.search(false)
	tt := ui.NewTester(a.view, 760, 520)
	if !tt.HasText("Obsidian") {
		t.Fatal(tt.Texts())
	}
	tt.Type("visual")
	if len(a.rows) != 1 || a.rows[0].App.ID != "code" {
		t.Fatalf("search rows=%+v", a.rows)
	}
	tt.Key(0, ui.KeyEscape)
	if a.query != "" || len(a.rows) != len(commands)+2 {
		t.Fatal("Escape did not clear query")
	}
	tt.Key(ui.Cmd, ui.Key2)
	if a.tab != "Clipboard" || !tt.HasText("Your clipboard starts here") {
		t.Fatal(tt.Texts())
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.Key3)
	if a.tab != "Calculator" {
		t.Fatal(a.tab)
	}
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Key(ui.Cmd, ui.Key4)
	if err := tt.Click("Quick capture"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.composing || !tt.HasText("Append to daily note") {
		t.Fatal(tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.composing {
		t.Fatal("composer remained open")
	}
}
func TestRender(t *testing.T) {
	dir := os.Getenv("LAUNCHR_RENDER")
	if dir == "" {
		t.Skip("set LAUNCHR_RENDER to save UI renders")
	}
	os.MkdirAll(dir, 0700)
	for _, tab := range []string{"Apps", "Clipboard", "Calculator", "Obsidian"} {
		a := &app{tab: tab, shortcut: "Alt+Space"}
		switch tab {
		case "Apps":
			a.rows = []result{{ID: "window", Title: "Launchr — Visual Studio Code", Detail: "Open window", Kind: "window"}, {ID: "obsidian", Title: "Obsidian", Detail: "Pinned", Kind: "app"}, {ID: "terminal", Title: "Terminal", Detail: "Application", Kind: "app"}}
		case "Calculator":
			a.query = "(240 + 60) / 3"
			a.rows = []result{{ID: "answer", Title: "100", Detail: "Copy result", Kind: "answer"}}
		case "Clipboard":
			a.rows = []result{{ID: "1", Title: "A useful thought, saved for later.", Detail: "Nov 2, 16:45   36 B", Kind: "clip", Clip: clip{ID: 1}}}
			a.previewID = 1
			a.previewText = "A useful thought, saved for later."
		case "Obsidian":
			a.rows = []result{{ID: "daily", Title: "Open daily note", Detail: "Obsidian", Kind: "daily"}}
		}
		tt := ui.NewTester(a.view, 760, 520)
		if tab == "Apps" {
			rect, ok := tt.Find("Terminal")
			if !ok || rect.Y+rect.H > 450 {
				t.Fatalf("results are clipped: %+v", rect)
			}
			rect, ok = tt.Find("Visual Studio Code")
			if !ok || rect.Y > 200 {
				t.Fatalf("results begin too far down: %+v", rect)
			}
		}
		f, err := os.Create(filepath.Join(dir, strings.ToLower(tab)+".png"))
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

func TestInstalledObsidianCLI(t *testing.T) {
	if os.Getenv("LAUNCHR_LIVE_OBSIDIAN") == "" {
		t.Skip("opt-in read-only integration check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := runObsidian(ctx, "", "search", "query=launchr_smoke_no_match_6219f42", "limit=1", "format=json")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err := json.Unmarshal([]byte(output), &paths); err != nil {
		t.Fatalf("CLI search did not return JSON: %v", err)
	}
}
