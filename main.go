package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

type result struct {
	ID, Title, Detail, Kind, Value string
	Clip                           clip
	App                            mygo.InstalledApp
	Window                         appWindow
}
type capture struct {
	kind  string
	data  []byte
	epoch uint64
}
type app struct {
	w                                                       *mygo.Window
	db                                                      *store
	ctx                                                     context.Context
	cancel                                                  context.CancelFunc
	wg                                                      sync.WaitGroup
	stopClipboard                                           func()
	captureSignals                                          chan struct{}
	capturePaused                                           atomic.Bool
	captureEpoch                                            atomic.Uint64
	iconJobs                                                chan string
	previewJobs                                             chan previewJob
	previewCancel                                           context.CancelFunc
	refreshPending                                          bool
	historyDirty                                            bool
	appNames                                                []string
	unpinned                                                []result
	filterBuffer                                            []result
	previewGeneration                                       uint64
	rateMu                                                  sync.Mutex
	rateRetry                                               map[string]time.Time
	prefs                                                   settings
	query, tab, status, shortcut                            string
	rows                                                    []result
	chosen                                                  int
	list                                                    ui.ListState
	input                                                   ui.Handle
	apps                                                    []mygo.InstalledApp
	windows                                                 []appWindow
	windowsPending                                          bool
	searchCancel                                            context.CancelFunc
	generation                                              uint64
	busy, more, paused, settingsOpen, composing, actionBusy bool
	noteTitle, noteBody, vaultEdit                          string
	previewID                                               uint64
	preview                                                 *ui.Bitmap
	previewText                                             string
	history                                                 []navigation
	formInput                                               ui.Handle
	icons                                                   map[string]*ui.Bitmap
	iconPending                                             map[string]bool
	densityEdit                                             string
	windowSizeEdit                                          string
}

func (a *app) background(fn func()) { a.wg.Add(1); go func() { defer a.wg.Done(); fn() }() }
func (a *app) update(fn func()) {
	if a.w != nil {
		a.w.Update(fn)
	}
}
func (a *app) setTab(tab string) {
	a.history = nil
	a.settingsOpen = false
	a.tab = tab
	a.query = ""
	a.composing = false
	a.preview = nil
	a.previewID = 0
	a.search(false)
	a.input.Focus()
}
func (a *app) search(appendPage bool) {
	a.searchWithOptions(appendPage, false, false)
}
func (a *app) searchWithOptions(appendPage, typing, preserve bool) {
	if a.searchCancel != nil {
		a.searchCancel()
	}
	a.generation++
	gen := a.generation
	selectedID := ""
	selectedHash := ""
	if preserve {
		if r, ok := a.selected(); ok {
			selectedID = r.ID
			selectedHash = r.Clip.Hash
		}
	}
	if !appendPage && !preserve {
		// A new query chooses its first result rather than following the old
		// selected item's key into a different result set.
		a.list = ui.ListState{}
		a.filterBuffer = a.rows[:0]
		a.rows = nil
		a.chosen = 0
		a.preview = nil
		a.previewText = ""
		a.previewID = 0
		a.cancelPreview()
	}
	a.status = ""
	a.more = false
	q, tab := strings.TrimSpace(a.query), a.tab
	if tab == "Apps" && !looksLikeMath(q) {
		if q != "" {
			a.filterCommands(q)
		}
		a.filterApps(q)
		if q == "" {
			a.filterCommands(q)
		}
		a.busy = false
		return
	}
	if tab == "Obsidian" && q == "" {
		a.rows = []result{{ID: "daily", Title: "Open daily note", Detail: "Obsidian · Run action", Kind: "daily"}, {ID: "capture", Title: "Quick capture", Detail: "Obsidian · Write a note", Kind: "command", Value: "capture"}}
		a.busy = false
		return
	}
	if (tab == "Calculator" || tab == "Apps") && q == "" {
		a.busy = false
		return
	}
	if a.db == nil {
		a.busy = false
		return
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.searchCancel = cancel
	before := uint64(0)
	if appendPage && len(a.rows) > 0 {
		before = a.rows[len(a.rows)-1].Clip.ID
	}
	vault := a.prefs.Vault
	a.busy = true
	a.background(func() {
		if typing && ((tab == "Clipboard" && q != "") || tab == "Obsidian") && !waitJob(ctx, 120*time.Millisecond) {
			return
		}
		var rows []result
		var err error
		more := false
		switch tab {
		case "Clipboard":
			var clips []clip
			clips, more, err = a.db.page(ctx, q, before, 100)
			for _, c := range clips {
				rows = append(rows, result{ID: strconv.FormatUint(c.ID, 10), Title: c.Preview, Detail: c.Time.Format("Jan 2, 15:04") + "   " + formatSize(c.Size), Kind: "clip", Clip: c})
			}
		case "Obsidian":
			var output string
			job, done := context.WithTimeout(ctx, 10*time.Second)
			output, err = runObsidian(job, vault, "search", "query="+q, "limit=100", "format=json")
			done()
			if err == nil {
				var paths []string
				err = json.Unmarshal([]byte(output), &paths)
				if err != nil {
					err = fmt.Errorf("Obsidian returned an unexpected search response")
				}
				for _, path := range paths {
					rows = append(rows, result{ID: path, Title: strings.TrimSuffix(filepath.Base(path), ".md"), Detail: path, Kind: "note", Value: path})
				}
			}
		default:
			if conv, ok := parseConversion(q); ok {
				var value, detail string
				job, done := context.WithTimeout(ctx, 8*time.Second)
				value, detail, err = a.currencyResult(job, conv, &http.Client{Timeout: 8 * time.Second}, "https://api.frankfurter.dev/v2/rate")
				done()
				if err == nil {
					rows = []result{{ID: "answer", Title: value, Detail: detail, Kind: "answer", Value: strings.Fields(value)[0]}}
				}
			} else {
				var value string
				value, err = calculate(q)
				if err == nil {
					rows = []result{{ID: "answer", Title: value, Detail: "Copy result", Kind: "answer", Value: value}}
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		a.update(func() {
			if gen != a.generation {
				return
			}
			a.busy = false
			if a.historyDirty {
				a.historyDirty = false
				if a.tab == "Clipboard" && a.w.IsVisible() {
					a.refreshHistory()
				}
			}
			a.more = more
			if err != nil {
				a.status = err.Error()
				return
			}
			if appendPage {
				a.rows = append(a.rows, rows...)
			} else {
				a.rows = rows
			}
			if len(rows) > 0 && strings.HasPrefix(rows[0].Detail, "Cached rate from ") {
				if conv, ok := parseConversion(q); ok {
					a.refreshCurrency(conv, &http.Client{Timeout: 8 * time.Second}, "https://api.frankfurter.dev/v2/rate")
				}
			}
			a.chosen = min(a.chosen, max(0, len(a.rows)-1))
			if selectedID != "" {
				for i := range a.rows {
					if a.rows[i].ID == selectedID || (selectedHash != "" && a.rows[i].Clip.Hash == selectedHash) {
						a.chosen = i
						break
					}
				}
			}
		})
	})
}
func looksLikeMath(q string) bool {
	if q == "" {
		return false
	}
	if _, ok := parseConversion(q); ok {
		return true
	}
	return strings.ContainsAny(q[:1], "0123456789.(+-") || strings.HasPrefix(q, "sqrt(") || strings.HasPrefix(q, "abs(") || strings.HasPrefix(q, "sin(") || strings.HasPrefix(q, "cos(") || strings.HasPrefix(q, "log(") || q == "pi"
}
func (a *app) filterApps(q string) {
	if a.rows == nil && a.filterBuffer != nil {
		a.rows = a.filterBuffer[:0]
		a.filterBuffer = nil
	}
	words := strings.Fields(strings.ToLower(q))
	match := func(s string) bool {
		for _, word := range words {
			if !strings.Contains(s, word) {
				return false
			}
		}
		return true
	}
	for _, w := range a.windows {
		if match(strings.ToLower(w.Title)) {
			a.rows = append(a.rows, result{ID: fmt.Sprint(w.Handle), Title: w.Title, Detail: "Open window", Kind: "window", Window: w})
		}
	}
	if len(a.appNames) != len(a.apps) {
		a.cacheAppNames()
	}
	pins := make(map[string]bool, len(a.prefs.Pins))
	for _, id := range a.prefs.Pins {
		pins[id] = true
	}
	a.unpinned = a.unpinned[:0]
	for i, item := range a.apps {
		if !match(a.appNames[i]) {
			continue
		}
		r := result{ID: item.ID, Title: item.Name, Detail: "Application", Kind: "app", App: item}
		if pins[item.ID] {
			r.Detail = "Pinned"
			a.rows = append(a.rows, r)
		} else {
			a.unpinned = append(a.unpinned, r)
		}
	}
	a.rows = append(a.rows, a.unpinned...)
}
func (a *app) cacheAppNames() {
	a.appNames = make([]string, len(a.apps))
	for i, item := range a.apps {
		a.appNames[i] = strings.ToLower(item.Name)
	}
}
func formatSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}
func (a *app) selected() (result, bool) {
	if a.chosen < 0 || a.chosen >= len(a.rows) {
		return result{}, false
	}
	return a.rows[a.chosen], true
}
func (a *app) choose(delta int) {
	if len(a.rows) == 0 {
		return
	}
	a.chosen = max(0, min(len(a.rows)-1, a.chosen+delta))
	a.list.ScrollIntoView(a.chosen)
}
func (a *app) hide() {
	a.cancelPreview()
	if a.w != nil {
		a.w.Hide()
	}
}
func (a *app) activate() {
	r, ok := a.selected()
	if !ok {
		return
	}
	switch r.Kind {
	case "command":
		a.openCommand(r.Value)
	case "window":
		a.hide()
		if err := activateWindow(r.Window); err != nil {
			a.status = err.Error()
			a.w.Show()
		}
	case "app":
		if err := mygo.Shell.LaunchApp(r.App); err != nil {
			a.status = err.Error()
		} else {
			a.hide()
		}
	case "answer":
		mygo.Clipboard.WriteText(r.Value)
		a.status = "Copied result"
	case "clip":
		a.background(func() {
			data, err := a.db.data(r.Clip)
			a.update(func() {
				if err != nil {
					a.status = err.Error()
					return
				}
				format := transfer.Text
				if r.Clip.Kind == "image" {
					format = transfer.PNG
				}
				if err := mygo.Clipboard.Write(transfer.New(transfer.NewItem(transfer.Bytes(format, data)))); err != nil {
					a.status = err.Error()
					return
				}
				a.hide()
			})
		})
	case "daily":
		a.obsidianAction("daily", nil, false)
	case "note":
		a.obsidianAction("open", []string{"path=" + r.Value}, false)
	}
}
func (a *app) obsidianAction(command string, params []string, clear bool) {
	if a.actionBusy {
		return
	}
	a.actionBusy = true
	vault := a.prefs.Vault
	a.background(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 12*time.Second)
		defer cancel()
		_, err := runObsidian(ctx, vault, command, params...)
		a.update(func() {
			a.actionBusy = false
			if err != nil {
				a.status = err.Error()
				return
			}
			if clear {
				a.noteBody = ""
				a.noteTitle = ""
				a.back()
				a.status = "Saved to Obsidian"
			} else {
				a.hide()
			}
		})
	})
}

// errElevationCancelled reports that the UAC prompt was declined.
var errElevationCancelled = errors.New("administrator launch cancelled")

// runElevated is replaced in tests, which must not raise a UAC prompt.
var runElevated = launchElevated

// activateElevated runs the selected application as administrator. The UAC
// prompt blocks until it is answered, so the launch waits off the UI thread.
func (a *app) activateElevated() {
	r, ok := a.selected()
	if !ok || r.Kind != "app" || a.actionBusy {
		return
	}
	path := r.App.Path
	if path == "" {
		path = `shell:AppsFolder\` + r.App.ID
	}
	a.actionBusy = true
	a.background(func() {
		err := runElevated(path)
		a.update(func() {
			a.actionBusy = false
			switch {
			case err == nil:
				a.hide()
			case errors.Is(err, errElevationCancelled):
			default:
				// Packaged Store apps cannot be elevated.
				a.status = r.Title + " can't run as administrator: " + err.Error()
				a.w.Show()
			}
		})
	})
}

func (a *app) pin() {
	r, ok := a.selected()
	if !ok || r.Kind != "app" {
		return
	}
	pins := slices.Clone(a.prefs.Pins)
	if i := slices.Index(pins, r.App.ID); i >= 0 {
		pins = append(pins[:i], pins[i+1:]...)
	} else {
		pins = append(pins, r.App.ID)
	}
	next := a.prefs
	next.Pins = pins
	a.savePrefs(next)
}
func (a *app) savePrefs(next settings) {
	a.background(func() {
		err := a.db.save("settings", "main", next)
		a.update(func() {
			if err != nil {
				a.status = err.Error()
				return
			}
			resize := normalizedWindowSize(a.prefs.WindowSize) != normalizedWindowSize(next.WindowSize)
			a.prefs = next
			if resize {
				x, y := a.w.Position()
				w, h := a.w.Size()
				d := mygo.Screen.DisplayNearestPoint(mygo.Point{X: x + w/2, Y: y + h/2})
				a.applyWindowSize(d.WorkArea)
			}
			a.status = "Settings saved"
			a.search(false)
		})
	})
}
func (a *app) deleteClip() {
	r, ok := a.selected()
	if !ok || r.Kind != "clip" {
		return
	}
	a.background(func() {
		err := a.db.delete(r.Clip)
		a.update(func() {
			if err != nil {
				a.status = err.Error()
			} else {
				a.search(false)
			}
		})
	})
}

var searchIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5"/></svg>`))

var rowIcons = func() map[string]*ui.SVG {
	paths := map[string]string{
		"app":        `<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>`,
		"window":     `<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18m-13-5v5"/>`,
		"clip":       `<rect x="5" y="5" width="14" height="16" rx="2"/><rect x="9" y="3" width="6" height="4" rx="1"/><path d="M9 12h6m-6 4h6"/>`,
		"image":      `<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m3 18 5-5 4 4 4-6 5 7"/>`,
		"note":       `<path d="M14 3H5v18h14V8zM14 3v5h5M8 12h8m-8 4h6"/>`,
		"daily":      `<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M7 3v4m10-4v4M3 11h18m-14 4h3m4 0h3"/>`,
		"answer":     `<path d="M5 9h14M5 15h14"/>`,
		"calculator": `<rect x="5" y="2" width="14" height="20" rx="3"/><path d="M8 6h8M8 11h1m6 0h1m-8 4h1m6 0h1m-8 4h1m6 0h1"/>`,
		"currency":   `<path d="M3 7h17l-4-4M21 17H4l4 4M12 8v8m3-7h-4a2 2 0 0 0 0 4h2a2 2 0 0 1 0 4H9"/>`,
		"settings":   `<path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="15" cy="12" r="2"/><circle cx="8" cy="18" r="2"/>`,
	}
	icons := map[string]*ui.SVG{}
	for kind, path := range paths {
		icons[kind] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
	}
	return icons
}()

func cliContent(s string) string {
	return strings.NewReplacer("\n", `\n`, "\t", `\t`).Replace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func (a *app) show() {
	if !a.windowsPending {
		a.windowsPending = true
		a.background(func() {
			windows := openWindows()
			a.update(func() {
				a.windowsPending = false
				a.windows = windows
				if a.tab == "Apps" && !a.settingsOpen && !a.composing && a.w.IsVisible() {
					a.search(false)
				}
			})
		})
	}
	a.search(false)
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	a.applyWindowSize(d.WorkArea)
	if a.composing || a.settingsOpen {
		a.formInput.Focus()
	} else {
		a.input.Focus()
	}
	a.w.Show()
	a.w.Focus()
}
func (a *app) captureClipboard() {
	select {
	case a.captureSignals <- struct{}{}:
	default:
	}
}
func trayIcon() []byte {
	return sizedTrayIcon(trayPixelSize())
}
func (a *app) ready() {
	mygo.Theme.SetSource(mygo.ThemeDark)
	w, h := launcherWindowSize(a.prefs.WindowSize)
	a.w = mygo.NewWindow(mygo.WindowOptions{Title: "Launchr", Width: w, Height: h, MinWidth: 640, MinHeight: 420, Panel: true, Hidden: true, TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: true, Transparent: true, Vibrancy: mygo.VibrancyAcrylic, CornerRadius: 16, BackgroundColor: "#090909", Content: ui.View(a.view)})
	a.w.OnClose(func(e *mygo.CloseEvent) { e.PreventDefault(); a.hide() })
	a.w.OnBlur(a.hide)
	for _, key := range []string{"Alt+Space", "Alt+Shift+Space"} {
		if err := mygo.GlobalShortcut.Register(key, func() {
			if a.w.IsVisible() {
				a.hide()
			} else {
				a.show()
			}
		}); err == nil {
			a.shortcut = key
			break
		}
	}
	if a.shortcut == "" {
		a.status = "Global shortcut unavailable. Use the tray to open Launchr."
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "Launchr"})
	if err != nil {
		a.status = "Tray unavailable: " + err.Error()
	} else {
		tray.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Label: "Open Launchr", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }}, {Role: mygo.RoleQuit}}))
		tray.OnClick(a.show)
	}
	a.startCaptureWorkers()
	a.startIconWorker()
	a.startPreviewWorker()
	a.background(func() {
		if err := a.db.ensureIndex(a.ctx); err != nil && a.ctx.Err() == nil {
			a.clipboardError(err)
		}
	})
	a.stopClipboard, err = watchClipboard(a.captureClipboard)
	if err != nil {
		a.status = err.Error()
	}
	a.background(func() {
		apps, err := mygo.Shell.InstalledApps()
		a.update(func() {
			if err != nil {
				a.status = err.Error()
				return
			}
			a.apps = apps
			a.cacheAppNames()
			if a.tab == "Apps" {
				a.search(false)
			}
		})
	})
	a.show()
}
func main() {
	mygo.App.SetName("Launchr")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dir = local
	}
	dataDir := filepath.Join(dir, "Launchr")
	if override := os.Getenv("LAUNCHR_DATA_DIR"); override != "" {
		dataDir = override
	}
	db, err := openStore(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer db.db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{db: db, ctx: ctx, cancel: cancel, tab: "Apps"}
	if err = db.load("settings", "main", &a.prefs); err != nil && !os.IsNotExist(err) {
		log.Print(err)
	}
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.w != nil {
			a.show()
		}
	})
	mygo.App.WhenReady(a.ready)
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.stopClipboard != nil {
			a.stopClipboard()
		}
		a.cancel()
	})
	err = mygo.App.Run()
	cancel()
	a.wg.Wait()
	if err != nil {
		log.Print(err)
	}
}
