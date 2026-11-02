package main

import (
	"context"
	"encoding/json"
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

func plainButton(c *ui.Context, label string) ui.Element {
	return ui.ButtonBase(c).Label(label).Children(func() { ui.Text(c, label) })
}

func (a *app) view(c *ui.Context) {
	density := a.density()
	theme := ui.DarkTheme()
	theme.Background = ui.Hex("#090909")
	theme.Font = "Martian Grotesk"
	theme.Surface = ui.Hex("#141414")
	theme.SurfaceHover = ui.Hex("#242424")
	theme.SurfacePressed = ui.Hex("#303030")
	theme.Border = ui.Hex("#303030")
	theme.Accent = ui.Hex("#eeeeec")
	theme.AccentHover = ui.Hex("#ffffff")
	theme.AccentPressed = ui.Hex("#cececc")
	theme.AccentText = ui.Hex("#111111")
	theme.Text = ui.Hex("#f2f2f0")
	theme.TextMuted = ui.Hex("#a7a7a5")
	theme.Selection = ui.Hex("#606060")
	theme.Scrollbar = ui.Hex("#858583").Alpha(.45)
	theme.ScrollbarWidth = 5
	theme.Focus = theme.Accent.Alpha(.6)
	theme.FontSize = 14
	theme.Spacing = density.spacing
	theme.Radius = 10
	if c.Vibrancy() {
		// Acrylic supplies the blur. Only the surfaces are translucent;
		// text and controls retain their contrast and opacity.
		theme.Background = theme.Background.Alpha(.42)
		theme.Surface = theme.Surface.Alpha(.40)
		theme.SurfaceHover = theme.SurfaceHover.Alpha(.65)
		// Keep selection opaque so desktop colors cannot wash it out.
	}
	c.SetTheme(theme)
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	c.Root().Background(theme.Background)
	c.OnShortcut(0, ui.KeyEscape, a.back)
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.openSettings)
	c.OnShortcut(ui.Cmd, ui.KeyB, a.back)
	c.OnShortcut(ui.Cmd, ui.KeyF, func() {
		if !a.settingsOpen && !a.composing {
			a.input.Focus(c)
		}
	})

	for i, key := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4} {
		tab := []string{"Apps", "Clipboard", "Calculator", "Obsidian"}[i]
		c.OnShortcut(ui.Cmd, key, func() { a.setTab(tab) })
	}
	if !a.settingsOpen && !a.composing {
		a.searchShortcuts(c)
		c.OnShortcut(ui.Cmd, ui.KeyN, func() { a.choose(1) })
		c.OnShortcut(ui.Cmd, ui.KeyDelete, a.deleteClip)
		c.OnShortcut(ui.Cmd, ui.KeyL, func() {
			if a.more {
				a.search(true)
			}
		})
		c.OnShortcut(ui.Cmd, ui.KeyP, a.pin)
	}
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(density.headerY, density.inset).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, appMark).Size(28, 28).Shrink(0).TextColor(theme.Text)
			ui.Text(c, "Launchr").Bold().Grow(1).DragWindow()
			plainButton(c, "Settings").TextColor(theme.TextMuted).Padding(4, 8).OnClick(func() { a.openSettings() })
			plainButton(c, "Hide").TextColor(theme.TextMuted).Padding(4, 8).OnClick(a.hide)
		})
		if a.settingsOpen {
			a.settingsView(c)
			return
		}
		if a.composing {
			ui.Row(c).Padding(0, 18).Children(func() { plainButton(c, "Back").TextColor(theme.Accent).OnClick(a.back) })
			a.composeView(c)
			return
		}
		ui.Row(c).Padding(density.searchY, density.inset).Background(theme.Surface).Gap(density.gap).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, searchIcon).FontSize(22).TextColor(theme.TextMuted)
			placeholder := "Search apps and commands…"
			switch a.tab {
			case "Clipboard":
				placeholder = "Search clipboard history…"
			case "Calculator":
				placeholder = "24 * 18 or 100 USD to NGN"
			case "Obsidian":
				placeholder = "Search notes…"
			}
			ui.TextInputBase(c.Key("search:"+a.tab), &a.query).Bind(&a.input).AutoFocus().Grow(1).FontSize(density.searchFont).Placeholder(placeholder).Label("Search").OnChange(func() { a.searchWithOptions(false, true, false) }).OnSubmit(a.activate)
		})
		ui.Row(c).Padding(density.tabsY, 12).Gap(4).Children(func() {
			for i, tab := range []string{"Apps", "Clipboard", "Calculator", "Obsidian"} {
				bg := ui.Transparent
				ink := theme.TextMuted
				if a.tab == tab {
					bg = theme.Selection
					ink = theme.Text
				}
				plainButton(c, fmt.Sprintf("%s  %d", tab, i+1)).Padding(7, 12).Radius(6).Background(bg).TextColor(ink).OnClick(func() { a.setTab(tab) })
			}
		})
		if a.tab != "Apps" {
			ui.Row(c).Padding(0, 18, 8, 18).Gap(8).Children(func() {
				plainButton(c, "Back").TextColor(theme.Accent).OnClick(a.back)
				ui.Text(c, "Launchr / "+a.tab).FontSize(12).TextColor(theme.TextMuted)
			})
		}
		{
			ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					if a.tab == "Clipboard" {
						ui.Row(c).Padding(4, 18, 8, 18).Gap(8).Children(func() {
							label := "Pause capture"
							if a.paused {
								label = "Resume capture"
							}
							plainButton(c, label).TextColor(theme.TextMuted).OnClick(func() { a.paused = !a.paused; a.capturePaused.Store(a.paused); a.captureEpoch.Add(1) })
							ui.Text(c, "No automatic expiry").FontSize(12).TextColor(theme.TextMuted).Grow(1).TextAlign(ui.End)
							plainButton(c, "Delete selected").TextColor(theme.TextMuted).OnClick(a.deleteClip)
						})
					}
					if a.tab == "Obsidian" {
						ui.Row(c).Padding(4, 18, 8, 18).Gap(16).Children(func() {
							plainButton(c, "Quick capture").TextColor(theme.Accent).OnClick(func() { a.openCommand("capture") })
							plainButton(c, "Daily note").TextColor(theme.TextMuted).OnClick(func() { a.obsidianAction("daily", nil, false) })
						})
					}
					if len(a.rows) == 0 {
						ui.Column(c).Grow(1).FillWidth().Center().Gap(10).Padding(24).Children(func() {
							title, detail := "No results", "Try a different search."
							if a.busy {
								title = "Searching…"
								detail = ""
							} else if a.query == "" {
								switch a.tab {
								case "Apps":
									title = "Applications"
									detail = "Open apps and installed applications appear here."
								case "Clipboard":
									title = "Your clipboard starts here"
									detail = "Copy text or an image to keep it here."
								case "Calculator":
									title = "Calculator"
									detail = "Try (240 + 60) / 3 or 100 USD to NGN"
								}
							}
							ui.Text(c, title).FontSize(14)
							if detail != "" {
								ui.Text(c, detail).TextColor(theme.TextMuted).TextAlign(ui.Center)
							}
						})
					} else {
						a.list.Selected = &a.chosen
						a.list.Label = func(i int) string { return a.rows[i].Title }
						// MyGo uses Accent for the native selected row. Scope its
						// muted fill to the list so primary controls stay vivid.
						listTheme := *theme
						listTheme.Accent, listTheme.AccentText = theme.Selection, theme.Text
						c.SetTheme(&listTheme)
						ui.List(c, &a.list, len(a.rows)).Grow(1).FillWidth().Padding(6, 24, 6, 10).ItemKey(func(i int) any { return a.rows[i].Kind + ":" + a.rows[i].ID }).Rows(func(row ui.ListRow) {
							r := a.rows[row.Index]
							// The list owns the selection and hover background. A second
							// rounded fill here exposes the outer row at its corners.
							ui.Row(c).Padding(density.rowY, density.rowX).Gap(density.gap).AlignItems(ui.Center).Children(func() {
								kind := r.Kind
								if kind == "clip" && r.Clip.Kind == "image" {
									kind = "image"
								}
								a.resultIcon(c, r, kind)
								ui.Column(c).Grow(1).Gap(density.textGap).Children(func() {
									size := float32(14)
									if r.Kind == "answer" {
										size = 24
									}
									ui.Text(c, r.Title).Bold().TextColor(theme.Text).FontSize(size).SingleLine()
									muted := theme.TextMuted
									if row.Selected() {
										muted = ui.Hex("#dededc")
									}
									ui.Text(c, r.Detail).FontSize(12).TextColor(muted).SingleLine()
								})
								if r.Kind == "window" {
									ui.Text(c, "Running").FontSize(11).TextColor(ui.Hex("#8de0bd")).Padding(4, 8).Radius(6).Background(ui.Hex("#254f43").Alpha(.65)).Shrink(0)
								} else if r.Kind == "app" {
									ui.Text(c, "Application").FontSize(11).TextColor(theme.TextMuted).Shrink(0)
								}
								if row.Index == a.chosen {
									ui.Text(c, resultAction(r)+"  ↵").FontSize(11).TextColor(theme.Accent).Padding(4, 8).Background(theme.Surface).Radius(5).Shrink(0)
								}
								if r.Kind == "app" && row.Index == a.chosen {
									label := "Pin"
									if slices.Contains(a.prefs.Pins, r.App.ID) {
										label = "Unpin"
									}
									plainButton(c, label).FontSize(12).TextColor(theme.TextMuted).OnClick(a.pin)
								}
							})
						}).OnSubmit(a.activate)
						c.SetTheme(theme)
					}
					if a.more {
						plainButton(c, "Load older items").Padding(10).OnClick(func() { a.search(true) })
					}
				})
				if a.tab == "Clipboard" && len(a.rows) > 0 {
					a.loadPreview()
					ui.Column(c).Width(240).Padding(16).Gap(12).Children(func() {
						ui.Text(c, "Preview").TextColor(theme.TextMuted).FontSize(12)
						if a.preview != nil {
							ui.Image(c, a.preview).Width(208).Height(240).Fit(ui.Contain)
						} else if a.previewText != "" {
							ui.Scroll(c).Grow(1).Padding(0, 20, 0, 0).Children(func() { ui.Text(c, a.previewText).Wrap() })
						}
					})
				}
			})
		}
		ui.Row(c).Padding(density.headerY, density.inset).Background(theme.Surface).Gap(density.gap).Children(func() {
			label := a.shortcut
			if a.status != "" {
				label = a.status
			} else if a.actionBusy {
				label = "Working…"
			} else if label == "" {
				label = "Launchr"
			}
			ui.Text(c, label).FontSize(12).TextColor(theme.TextMuted).Grow(1).MaxLines(2)
			if !a.composing {
				hint := "↑ ↓ Select    Enter Open    Esc Back"
				if r, ok := a.selected(); ok {
					hint = "↑ ↓ Select    Enter " + resultAction(r) + "    Esc Back"
				}
				if a.tab == "Clipboard" {
					hint = "Enter Copy"
				} else if a.tab == "Calculator" {
					hint = "Enter Copy result"
				}
				ui.Text(c, hint).FontSize(12).TextColor(theme.TextMuted)
			}
		})
	})
}
func (a *app) composeView(c *ui.Context) {
	density := a.density()
	ui.Column(c).Grow(1).FillWidth().Padding(density.inset).Gap(density.gap).Children(func() {
		c.OnShortcut(ui.Cmd, ui.KeyEnter, a.createNote)
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyEnter, a.appendDaily)
		ui.Text(c, "Quick capture").FontSize(22).Bold()
		ui.Text(c, "Ctrl+Enter creates a note · Ctrl+Shift+Enter appends to today").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.TextInput(c.Key("note-title"), &a.noteTitle).Bind(&a.formInput).Label("Note title").Placeholder("Note title").FillWidth()
		ui.TextArea(c, &a.noteBody).Label("Note content").Placeholder("Write a note…").Padding(density.scale(12), density.scale(24), density.scale(12), density.scale(12)).Grow(1).FillWidth()
		ui.Row(c).Gap(density.scale(10)).Children(func() {
			ui.Button(c, "Create note").Disabled(a.actionBusy || strings.TrimSpace(a.noteTitle) == "" || strings.TrimSpace(a.noteBody) == "").OnClick(a.createNote)
			ui.Button(c, "Append to daily note").Disabled(a.actionBusy || strings.TrimSpace(a.noteBody) == "").OnClick(a.appendDaily)
			plainButton(c, "Cancel").OnClick(a.back)
		})
	})
}
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
