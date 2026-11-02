package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	kind string
	data []byte
}
type app struct {
	w                                                       *mygo.Window
	db                                                      *store
	ctx                                                     context.Context
	cancel                                                  context.CancelFunc
	wg                                                      sync.WaitGroup
	stopClipboard                                           func()
	captures                                                chan capture
	prefs                                                   settings
	query, tab, status, shortcut                            string
	rows                                                    []result
	chosen                                                  int
	list                                                    ui.ListState
	input                                                   ui.Handle
	apps                                                    []mygo.InstalledApp
	windows                                                 []appWindow
	searchCancel                                            context.CancelFunc
	generation                                              uint64
	busy, more, paused, settingsOpen, composing, actionBusy bool
	noteTitle, noteBody, vaultEdit                          string
	previewID                                               uint64
	preview                                                 *ui.Bitmap
	previewText                                             string
}

func (a *app) background(fn func()) { a.wg.Add(1); go func() { defer a.wg.Done(); fn() }() }
func (a *app) update(fn func()) {
	if a.w != nil {
		a.w.Update(fn)
	}
}
func (a *app) setTab(tab string) {
	a.tab = tab
	a.query = ""
	a.composing = false
	a.preview = nil
	a.previewID = 0
	a.search(false)
	a.input.Focus()
}
func (a *app) search(appendPage bool) {
	if a.searchCancel != nil {
		a.searchCancel()
	}
	a.generation++
	gen := a.generation
	if !appendPage {
		a.rows = nil
		a.chosen = 0
		a.preview = nil
		a.previewText = ""
		a.previewID = 0
	}
	a.status = ""
	a.more = false
	q, tab := strings.TrimSpace(a.query), a.tab
	if tab == "Apps" && !looksLikeMath(q) {
		a.filterApps(q)
		a.busy = false
		return
	}
	if tab == "Obsidian" && q == "" {
		a.rows = []result{{ID: "daily", Title: "Open daily note", Detail: "Obsidian", Kind: "daily"}}
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
		timer := time.NewTimer(120 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
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
				value, detail, err = convertCurrency(job, a.db, &http.Client{Timeout: 8 * time.Second}, "https://api.frankfurter.dev/v2/rate", conv)
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
			a.chosen = min(a.chosen, max(0, len(a.rows)-1))
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
	match := func(s string) bool {
		for _, word := range strings.Fields(strings.ToLower(q)) {
			if !strings.Contains(strings.ToLower(s), word) {
				return false
			}
		}
		return true
	}
	for _, w := range a.windows {
		if match(w.Title) {
			a.rows = append(a.rows, result{ID: fmt.Sprint(w.Handle), Title: w.Title, Detail: "Open window", Kind: "window", Window: w})
		}
	}
	for _, pinned := range []bool{true, false} {
		for _, item := range a.apps {
			pin := slices.Contains(a.prefs.Pins, item.ID)
			if pin != pinned || !match(item.Name) {
				continue
			}
			detail := "Application"
			if pin {
				detail = "Pinned"
			}
			a.rows = append(a.rows, result{ID: item.ID, Title: item.Name, Detail: detail, Kind: "app", App: item})
		}
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
				a.composing = false
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
			a.prefs = next
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
func (a *app) loadPreview() {
	r, ok := a.selected()
	if !ok || r.Kind != "clip" || r.Clip.ID == a.previewID {
		return
	}
	a.previewID = r.Clip.ID
	a.preview = nil
	a.previewText = ""
	id := r.Clip.ID
	a.background(func() {
		data, err := a.db.data(r.Clip)
		var bitmap *ui.Bitmap
		text := ""
		if err == nil {
			if r.Clip.Kind == "image" {
				img, _, e := image.Decode(bytes.NewReader(data))
				err = e
				if err == nil {
					bounds := img.Bounds()
					w, h := bounds.Dx(), bounds.Dy()
					scale := min(1.0, 480.0/float64(max(w, h)))
					dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
					for y := 0; y < dst.Bounds().Dy(); y++ {
						for x := 0; x < dst.Bounds().Dx(); x++ {
							dst.Set(x, y, img.At(bounds.Min.X+int(float64(x)/scale), bounds.Min.Y+int(float64(y)/scale)))
						}
					}
					bitmap = ui.NewBitmap(dst)
				}
			} else {
				runes := []rune(string(data))
				if len(runes) > 4000 {
					runes = append(runes[:4000], []rune("\n… Preview truncated. Copy restores the full text.")...)
				}
				text = string(runes)
			}
		}
		a.update(func() {
			if a.previewID != id {
				return
			}
			if err != nil {
				a.status = err.Error()
				return
			}
			a.preview = bitmap
			a.previewText = text
		})
	})
}

var searchIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5"/></svg>`))

var rowIcons = func() map[string]*ui.SVG {
	paths := map[string]string{
		"app":    `<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>`,
		"window": `<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18m-13-5v5"/>`,
		"clip":   `<rect x="5" y="5" width="14" height="16" rx="2"/><rect x="9" y="3" width="6" height="4" rx="1"/><path d="M9 12h6m-6 4h6"/>`,
		"image":  `<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m3 18 5-5 4 4 4-6 5 7"/>`,
		"note":   `<path d="M14 3H5v18h14V8zM14 3v5h5M8 12h8m-8 4h6"/>`,
		"daily":  `<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M7 3v4m10-4v4M3 11h18m-14 4h3m4 0h3"/>`,
		"answer": `<path d="M5 9h14M5 15h14"/>`,
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
	theme := ui.DarkTheme()
	theme.Background = ui.RGB(24, 25, 27)
	theme.Surface = ui.RGB(32, 34, 37)
	theme.SurfaceHover = ui.RGB(43, 46, 49)
	theme.Border = ui.RGB(56, 59, 62)
	theme.Accent = ui.RGB(173, 203, 180)
	theme.AccentText = ui.RGB(22, 31, 25)
	theme.Focus = ui.Transparent
	theme.FontSize = 14
	theme.Radius = 6
	c.SetTheme(theme)
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	c.OnShortcut(0, ui.KeyEscape, func() {
		if a.settingsOpen || a.composing {
			a.settingsOpen = false
			a.composing = false
		} else if a.query != "" {
			a.query = ""
			a.search(false)
		} else {
			a.hide()
		}
	})
	for i, key := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4} {
		tab := []string{"Apps", "Clipboard", "Calculator", "Obsidian"}[i]
		c.OnShortcut(ui.Cmd, key, func() { a.setTab(tab) })
	}
	if !a.settingsOpen && !a.composing {
		c.OnShortcut(0, ui.KeyDown, func() { a.choose(1) })
		c.OnShortcut(0, ui.KeyUp, func() { a.choose(-1) })
		c.OnShortcut(0, ui.KeyEnter, a.activate)
		c.OnShortcut(ui.Cmd, ui.KeyP, a.pin)
	}
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(12, 18).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "Launchr").Bold().Grow(1).DragWindow()
			plainButton(c, "Settings").TextColor(theme.TextMuted).Padding(4, 8).OnClick(func() { a.settingsOpen = !a.settingsOpen; a.vaultEdit = a.prefs.Vault })
			plainButton(c, "Hide").TextColor(theme.TextMuted).Padding(4, 8).OnClick(a.hide)
		})
		if a.settingsOpen {
			a.settingsView(c)
			return
		}
		ui.Row(c).Padding(6, 18, 16, 18).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, searchIcon).FontSize(22).TextColor(theme.TextMuted)
			placeholder := "Search apps, calculate, or convert…"
			switch a.tab {
			case "Clipboard":
				placeholder = "Search clipboard history…"
			case "Calculator":
				placeholder = "24 * 18 or 100 USD to NGN"
			case "Obsidian":
				placeholder = "Search notes…"
			}
			ui.TextInputBase(c, &a.query).Bind(&a.input).AutoFocus().Grow(1).FontSize(20).Placeholder(placeholder).Label("Search").OnChange(func() { a.search(false) })
		})
		ui.Row(c).Padding(0, 12, 10, 12).Gap(4).Children(func() {
			for _, tab := range []string{"Apps", "Clipboard", "Calculator", "Obsidian"} {
				bg := ui.Transparent
				ink := theme.TextMuted
				if a.tab == tab {
					bg = theme.SurfaceHover
					ink = theme.Text
				}
				plainButton(c, tab).Padding(7, 12).Radius(6).Background(bg).TextColor(ink).OnClick(func() { a.setTab(tab) })
			}
		})
		if a.composing {
			a.composeView(c)
		} else {
			ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					if a.tab == "Clipboard" {
						ui.Row(c).Padding(4, 18, 8, 18).Gap(8).Children(func() {
							label := "Pause capture"
							if a.paused {
								label = "Resume capture"
							}
							plainButton(c, label).TextColor(theme.TextMuted).OnClick(func() { a.paused = !a.paused })
							ui.Text(c, "No automatic expiry").FontSize(12).TextColor(theme.TextMuted).Grow(1).TextAlign(ui.End)
							plainButton(c, "Delete selected").TextColor(theme.TextMuted).OnClick(a.deleteClip)
						})
					}
					if a.tab == "Obsidian" {
						ui.Row(c).Padding(4, 18, 8, 18).Gap(16).Children(func() {
							plainButton(c, "Quick capture").TextColor(theme.Accent).OnClick(func() { a.composing = true })
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
						ui.List(c, &a.list, len(a.rows)).Grow(1).FillWidth().Padding(6, 10).ItemKey(func(i int) any { return a.rows[i].Kind + ":" + a.rows[i].ID }).Rows(func(row ui.ListRow) {
							r := a.rows[row.Index]
							bg := ui.Transparent
							if row.Index == a.chosen {
								bg = theme.SurfaceHover
							}
							ui.Row(c).Padding(12).Gap(12).AlignItems(ui.Center).Radius(6).Background(bg).Children(func() {
								kind := r.Kind
								if kind == "clip" && r.Clip.Kind == "image" {
									kind = "image"
								}
								ui.Icon(c, rowIcons[kind]).Size(22, 22).TextColor(theme.Accent)
								ui.Column(c).Grow(1).Gap(4).Children(func() {
									size := float32(14)
									if r.Kind == "answer" {
										size = 24
									}
									ui.Text(c, r.Title).TextColor(theme.Text).FontSize(size).SingleLine()
									ui.Text(c, r.Detail).FontSize(12).TextColor(theme.TextMuted).SingleLine()
								})
								if r.Kind == "app" && row.Index == a.chosen {
									label := "Pin"
									if slices.Contains(a.prefs.Pins, r.App.ID) {
										label = "Unpin"
									}
									plainButton(c, label).FontSize(12).TextColor(theme.TextMuted).OnClick(a.pin)
								}
							})
						}).OnSubmit(a.activate)
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
							ui.Scroll(c).Grow(1).Children(func() { ui.Text(c, a.previewText).Wrap() })
						}
					})
				}
			})
		}
		ui.Row(c).Padding(12, 18).Gap(12).Children(func() {
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
				hint := "↑ ↓ Select    Enter Open"
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
func (a *app) settingsView(c *ui.Context) {
	ui.Column(c).Grow(1).FillWidth().Padding(24).Gap(18).Children(func() {
		ui.Text(c, "Settings").FontSize(20)
		ui.Text(c, "Obsidian vault")
		ui.TextInput(c, &a.vaultEdit).Label("Obsidian vault").Placeholder("Leave empty to use the active vault").FillWidth()
		ui.Button(c, "Save").OnClick(func() {
			next := a.prefs
			next.Vault = strings.TrimSpace(a.vaultEdit)
			a.savePrefs(next)
			a.settingsOpen = false
		})
		ui.Text(c, "Enable the official CLI in Obsidian → Settings → General.").TextColor(c.Theme().TextMuted).Wrap()
		ui.Text(c, "Ctrl+1–4 switches views. Ctrl+P pins an application.").TextColor(c.Theme().TextMuted).Wrap()
		ui.Text(c, "Clipboard history stays on this device, unencrypted, until you delete it. Deleted space is reused by new entries.").TextColor(c.Theme().TextMuted).Wrap()
	})
}
func (a *app) composeView(c *ui.Context) {
	ui.Column(c).Grow(1).FillWidth().Padding(18).Gap(12).Children(func() {
		ui.TextInput(c, &a.noteTitle).Label("Note title").Placeholder("Note title").FillWidth()
		ui.TextArea(c, &a.noteBody).Label("Note content").Placeholder("Write a note…").Grow(1).FillWidth()
		ui.Row(c).Gap(10).Children(func() {
			ui.Button(c, "Create note").Disabled(a.actionBusy || strings.TrimSpace(a.noteTitle) == "" || strings.TrimSpace(a.noteBody) == "").OnClick(func() {
				a.obsidianAction("create", []string{"name=" + a.noteTitle, "content=" + cliContent(a.noteBody)}, true)
			})
			ui.Button(c, "Append to daily note").Disabled(a.actionBusy || strings.TrimSpace(a.noteBody) == "").OnClick(func() { a.obsidianAction("daily:append", []string{"content=" + cliContent(a.noteBody)}, true) })
			plainButton(c, "Cancel").OnClick(func() { a.composing = false })
		})
	})
}
func cliContent(s string) string {
	return strings.NewReplacer("\n", `\n`, "\t", `\t`).Replace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func (a *app) show() {
	a.windows = openWindows()
	a.search(false)
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	w, h := a.w.Size()
	a.w.SetPosition(d.WorkArea.X+(d.WorkArea.Width-w)/2, d.WorkArea.Y+(d.WorkArea.Height-h)/3)
	a.input.Focus()
	a.w.Show()
	a.w.Focus()
}
func (a *app) captureClipboard() {
	a.update(func() {
		if a.paused {
			return
		}
		data := mygo.Clipboard.ReadImage()
		kind := "image"
		if len(data) == 0 {
			data = []byte(mygo.Clipboard.ReadText())
			kind = "text"
		}
		if len(data) == 0 {
			return
		}
		select {
		case a.captures <- capture{kind, data}:
		case <-a.ctx.Done():
		}
	})
}
func trayIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 6; y < 26; y++ {
		for x := 7; x < 25; x++ {
			if x < 12 || y > 20 {
				img.Set(x, y, color.RGBA{173, 203, 180, 255})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
func (a *app) ready() {
	a.w = mygo.NewWindow(mygo.WindowOptions{Title: "Launchr", Width: 760, Height: 520, MinWidth: 640, MinHeight: 420, Panel: true, Hidden: true, TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: true, BackgroundColor: "#18191b", Content: ui.View(a.view)})
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
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), ToolTip: "Launchr"})
	if err != nil {
		a.status = "Tray unavailable: " + err.Error()
	} else {
		tray.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Label: "Open Launchr", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }}, {Role: mygo.RoleQuit}}))
		tray.OnClick(a.show)
	}
	a.captures = make(chan capture, 32)
	a.background(func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case item := <-a.captures:
				err := a.db.add(item.kind, item.data)
				a.update(func() {
					if err != nil {
						a.status = "Clipboard: " + err.Error()
					} else if a.tab == "Clipboard" && a.w.IsVisible() {
						a.search(false)
					}
				})
			}
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
