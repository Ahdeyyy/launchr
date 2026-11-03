package main

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var tabNames = []string{"Apps", "Clipboard", "Calculator", "Obsidian"}

var backIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5m6-6-6 6 6 6"/></svg>`))

func launcherTheme(c *ui.Context, density densityMetrics) *ui.Theme {
	theme := ui.DarkTheme()
	theme.Background = inkBackground
	theme.Font = "Martian Grotesk"
	theme.Surface = inkSurface
	theme.SurfaceHover = inkHover
	theme.SurfacePressed = inkPressed
	theme.Border = ui.Hex("#2c2c2c")
	theme.Accent = ui.Hex("#eeeeec")
	theme.AccentHover = ui.Hex("#ffffff")
	theme.AccentPressed = ui.Hex("#cececc")
	theme.AccentText = ui.Hex("#111111")
	theme.Text = inkText
	theme.TextMuted = inkMuted
	theme.Selection = ui.Hex("#5a5a5a")
	theme.Scrollbar = ui.Hex("#858583").Alpha(.4)
	theme.ScrollbarWidth = 5
	theme.Focus = theme.Accent.Alpha(.5)
	theme.FontSize = 14
	theme.Spacing = density.spacing
	theme.Radius = 9
	if c.Vibrancy() {
		// Acrylic supplies the blur. Only the surfaces are translucent;
		// text and controls retain their contrast and opacity.
		theme.Background = theme.Background.Alpha(.42)
		theme.Surface = theme.Surface.Alpha(.40)
		theme.SurfaceHover = theme.SurfaceHover.Alpha(.65)
		// Keep selection opaque so desktop colors cannot wash it out.
	}
	return theme
}

// groupDigits writes n with thousands separators.
func groupDigits(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func (a *app) sectionTitle() string {
	switch a.tab {
	case "Clipboard":
		if a.query != "" {
			return "Matches"
		}
		return "History"
	case "Calculator":
		return "Result"
	case "Obsidian":
		if a.query == "" {
			return "Actions"
		}
		return "Notes"
	}
	if a.query == "" {
		return "Suggestions"
	}
	return "Results"
}

// Messages longer than this get a strip of their own above the action bar,
// where they can wrap; shorter ones fit beside the actions.
const inlineStatusRunes = 40

func (a *app) longStatus() bool { return utf8.RuneCountInString(a.status) > inlineStatusRunes }

// statusLabel is what the action bar shows on its left: a short status,
// work in progress, or where you are.
func (a *app) statusLabel(place string) (string, ui.Color) {
	if a.status != "" && !a.longStatus() {
		return a.status, inkText
	}
	if a.actionBusy {
		return "Working…", inkMuted
	}
	return place, inkMuted
}

func (a *app) statusStrip(c *ui.Context, density densityMetrics) {
	if !a.longStatus() {
		return
	}
	strip := ui.Row(c.Key("status-strip:"+a.status)).Padding(8, density.inset-4, 8, density.inset).Gap(10).AlignItems(ui.Start).Background(inkSurface).BorderWidth(1, 0, 0, 0).BorderColor(inkLine).Shrink(0)
	strip.Transition(ui.ElementTransition{Animation: &slideMotion, Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}}).Children(func() {
		ui.Row(c).Size(6, 6).Radius(3).Background(inkMuted).Shrink(0).Margin(6, 0, 0, 0)
		ui.Text(c, a.status).FontSize(12).TextColor(inkText).Wrap().MaxLines(3).Grow(1).MinWidth(0).Tooltip(a.status)
		chipButton(c, "Dismiss").OnClick(func() { a.status = "" })
	})
}

// middleTruncate keeps the start and the end of s, where paths and titles
// usually differ, within n runes.
func middleTruncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	head := (n - 1) / 2
	return string(runes[:head]) + "…" + string(runes[len(runes)-(n-1-head):])
}

// rowText is what a result row shows, apart from the data it searches:
// a window's app name beside its document, a note's folder rather than its
// full path, and a placeholder for clipboard text that is only whitespace.
func rowText(r result) (title, detail string, placeholder bool) {
	title, detail = r.Title, r.Detail
	switch r.Kind {
	case "clip":
		if strings.TrimSpace(title) == "" {
			return "Whitespace only", detail, true
		}
	case "window":
		for _, sep := range []string{" — ", " – ", " - "} {
			if i := strings.LastIndex(title, sep); i > 0 {
				if app := title[i+len(sep):]; app != "" && utf8.RuneCountInString(app) <= 32 {
					return title[:i], app, false
				}
			}
		}
	case "note":
		// The deepest folders tell notes apart; the vault-level ones repeat.
		dir := filepath.ToSlash(filepath.Dir(detail))
		if dir == "." {
			dir = "Vault root"
		} else if parts := strings.Split(dir, "/"); len(parts) > 2 {
			dir = "…/" + strings.Join(parts[len(parts)-2:], "/")
		}
		detail = middleTruncate(dir, 28)
	}
	return title, detail, false
}

func resultAccessory(c *ui.Context, r result, detail string) {
	switch r.Kind {
	case "window":
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(3, 8).Radius(6).Background(ui.Hex("#254f43").Alpha(.55)).Shrink(0).Children(func() {
			ui.Row(c).Size(6, 6).Radius(3).Background(inkRunning).Shrink(0)
			ui.Text(c, "Running").FontSize(11).TextColor(inkRunning).SingleLine()
		})
	case "app":
		if detail == "Application" {
			return
		}
		ui.Text(c, "Application").FontSize(12).TextColor(inkFaint).Shrink(0).SingleLine()
	case "command", "daily":
		ui.Text(c, "Command").FontSize(12).TextColor(inkFaint).Shrink(0).SingleLine()
	}
}

func (a *app) view(c *ui.Context) {
	density := a.density()
	theme := launcherTheme(c, density)
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
		tab := tabNames[i]
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
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyEnter, a.activateElevated)
	}
	ui.Column(c).Fill().Children(func() {
		if a.settingsOpen {
			a.settingsView(c)
			return
		}
		if a.composing {
			a.composeView(c)
			return
		}
		a.searchBar(c, density)
		loadingBar(c, a.busy)
		a.tabStrip(c, density)
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				a.sectionHeader(c, density)
				if len(a.rows) == 0 {
					a.emptyState(c)
				} else {
					a.resultList(c, density)
				}
				if a.more {
					ui.Row(c).Padding(4, density.inset, 8, density.inset).Children(func() {
						actionButton(c, "Load older items", "Ctrl", "L").OnClick(func() { a.search(true) })
					})
				}
			})
			if a.tab == "Clipboard" && len(a.rows) > 0 {
				a.loadPreview()
				a.previewPane(c, density)
			}
		})
		a.statusStrip(c, density)
		a.actionBar(c, density)
	})
}

// backButton is the arrow that leads out of every screen below the root.
func backButton(c *ui.Context) ui.Element {
	back := ui.ButtonBase(c).Label("Back").Size(28, 28).Center().Radius(7).Shrink(0).Tooltip("Back  Esc")
	bg := inkHover
	if back.Hovered() || back.Pressed() {
		bg = inkPressed
	}
	scale := float32(1)
	if back.Pressed() {
		scale = .94
	}
	scale = back.AnimateTo("press", scale, pressMotion)
	return back.Background(back.AnimateColor("fill", bg, hoverMotion)).Scale(scale, scale).Children(func() {
		ui.Icon(c, backIcon).Size(16, 16).TextColor(inkText)
	})
}

func (a *app) searchBar(c *ui.Context, density densityMetrics) {
	ui.Row(c).Padding(density.searchY+2, density.inset).Gap(density.gap).AlignItems(ui.Center).Shrink(0).Children(func() {
		if a.tab == "Apps" {
			ui.Row(c).Size(28, 28).Center().Shrink(0).Children(func() { ui.Icon(c, searchIcon).Size(20, 20).TextColor(inkFaint) })
		} else {
			backButton(c).OnClick(a.back)
		}
		placeholder := "Search apps and commands…"
		switch a.tab {
		case "Clipboard":
			placeholder = "Search clipboard history…"
		case "Calculator":
			placeholder = "24 * 18 or 100 USD to NGN"
		case "Obsidian":
			placeholder = "Search notes…"
		}
		ui.TextInputBase(c.Key("search:"+a.tab), &a.query).Bind(&a.input).AutoFocus().Grow(1).MinWidth(0).FontSize(density.searchFont).Placeholder(placeholder).Label("Search").OnChange(func() { a.searchWithOptions(false, true, false) }).OnSubmit(a.activate)
	})
}

func (a *app) tabStrip(c *ui.Context, density densityMetrics) {
	ui.Row(c).Padding(density.tabsY, density.inset-6, 2, density.inset-6).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		for i, tab := range tabNames {
			active := a.tab == tab
			chip := ui.ButtonBase(c.Key("tab:"+tab)).Label(tab).Padding(5, 10).Radius(7).Shrink(0)
			ink := inkMuted
			if active {
				ink = inkText
			} else if chip.Hovered() {
				ink = ui.Hex("#c8c8c8")
			}
			chip.OnClick(func() { a.setTab(tab) }).Children(func() {
				if active {
					// The pill is matched across tabs, so it slides from the
					// previous tab to this one instead of jumping.
					ui.Row(c.Key("pill")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(7).Background(ui.Hex("#ffffff").Alpha(.1)).Transition(ui.ElementTransition{Animation: &slideMotion, Match: tabIndicator{}})
				}
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Text(c, tab).FontSize(12).FontWeight(500).TextColor(chip.AnimateColor("ink", ink, hoverMotion)).SingleLine()
					ui.Text(c, strconv.Itoa(i+1)).FontSize(10).TextColor(inkFaint).SingleLine()
				})
			})
		}
		ui.Row(c).Grow(1).Height(28).DragWindow()
		chipButton(c, "Settings").Tooltip("Settings  Ctrl+,").OnClick(a.openSettings)
	})
}

func (a *app) sectionHeader(c *ui.Context, density densityMetrics) {
	ui.Row(c).Padding(8, density.inset+28, 4, density.inset+4).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, a.sectionTitle()).FontSize(11).FontWeight(500).TextColor(inkFaint).SingleLine().Shrink(0)
		if n := len(a.rows); n > 0 && a.tab != "Calculator" {
			count := groupDigits(n)
			if a.more {
				count += "+"
			}
			ui.Text(c, count).FontSize(11).TextColor(inkFaint).FontFeatures("tnum").SingleLine().Shrink(0)
		}
		ui.Row(c).Grow(1)
		switch a.tab {
		case "Clipboard":
			label := "Pause capture"
			if a.paused {
				label = "Resume capture"
				ui.Text(c, "Paused").FontSize(11).TextColor(ui.Hex("#f4cd71")).SingleLine().Shrink(0)
			}
			chipButton(c, label).OnClick(func() { a.paused = !a.paused; a.capturePaused.Store(a.paused); a.captureEpoch.Add(1) })
		case "Obsidian":
			chipButton(c, "Quick capture").OnClick(func() { a.openCommand("capture") })
			chipButton(c, "Daily note").OnClick(func() { a.obsidianAction("daily", nil, false) })
		}
	})
}

func (a *app) emptyState(c *ui.Context) {
	title, detail := "No results", "Try a different search."
	if a.busy {
		title, detail = "Searching…", ""
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
	// Keyed by its message so a new state fades in rather than swapping.
	ui.Column(c.Key("empty:" + title)).Grow(1).FillWidth().Center().Gap(8).Padding(24).Transition(ui.ElementTransition{Animation: &slideMotion, Enter: &ui.Motion{Y: 4}}).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(500).TextAlign(ui.Center).Wrap()
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(inkMuted).TextAlign(ui.Center).Wrap()
		}
	})
}

func (a *app) resultList(c *ui.Context, density densityMetrics) {
	theme := c.Theme()
	a.list.Selected = &a.chosen
	a.list.Label = func(i int) string { return a.rows[i].Title }
	// MyGo uses Accent for the native selected row. Scope a quiet neutral
	// fill to the list so primary controls stay vivid.
	listTheme := *theme
	listTheme.Accent, listTheme.AccentText = inkSelected, inkText
	listTheme.Radius = 8
	c.SetTheme(&listTheme)
	ui.List(c, &a.list, len(a.rows)).Grow(1).FillWidth().Padding(2, 24, 6, density.inset-8).ItemKey(func(i int) any { return a.rows[i].Kind + ":" + a.rows[i].ID }).Rows(func(row ui.ListRow) {
		r := a.rows[row.Index]
		// The list owns the selection and hover background. A second
		// rounded fill here exposes the outer row at its corners.
		ui.Row(c).Padding(density.rowY, density.rowX).Gap(density.gap).AlignItems(ui.Center).Children(func() {
			kind := r.Kind
			if kind == "clip" && r.Clip.Kind == "image" {
				kind = "image"
			}
			a.resultIcon(c, r, kind)
			if r.Kind == "answer" {
				ui.Column(c).Grow(1).MinWidth(0).Gap(density.textGap).Children(func() {
					ui.Text(c, r.Title).FontSize(24).FontWeight(500).TextColor(inkText).FontFeatures("tnum").SingleLine().Tooltip(r.Title)
					ui.Text(c, r.Detail).FontSize(12).TextColor(inkMuted).SingleLine()
				})
				return
			}
			title, detail, placeholder := rowText(r)
			ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Baseline).Children(func() {
				ink := inkText
				if placeholder {
					ink = inkMuted
				}
				t := ui.Text(c, title).FontWeight(500).TextColor(ink).SingleLine().Tooltip(r.Title)
				muted := inkMuted
				if row.Selected() {
					muted = ui.Hex("#b4b4b4")
				}
				d := ui.Text(c, detail).FontSize(12).TextColor(muted).SingleLine().MinWidth(0).Tooltip(r.Detail)
				// Dates, sizes and app names stay whole and the title yields;
				// elsewhere the title wins and the description yields.
				if r.Kind == "clip" || r.Kind == "note" || (r.Kind == "window" && detail != r.Detail) {
					t.Shrink(1).MinWidth(0)
					d.Shrink(0)
				} else {
					t.Shrink(0).MaxWidthPercent(72)
					d.Shrink(1000)
				}
			})
			resultAccessory(c, r, detail)
		})
	}).OnSubmit(a.activate)
	c.SetTheme(theme)
}

func (a *app) previewPane(c *ui.Context, density densityMetrics) {
	r, _ := a.selected()
	ui.Column(c).Width(260).Shrink(0).BorderWidth(0, 0, 0, 1).BorderColor(inkLine).Children(func() {
		ui.Column(c).Grow(1).MinHeight(0).Padding(12, 16).Children(func() {
			if a.preview != nil {
				ui.Image(c, a.preview).Width(228).Height(200).Fit(ui.Contain)
			} else if a.previewText != "" && strings.TrimSpace(a.previewText) == "" {
				ui.Text(c, "Whitespace only").FontSize(13).TextColor(inkMuted)
			} else if a.previewText != "" {
				ui.Scroll(c).Grow(1).Padding(0, 14, 0, 0).Children(func() { ui.Text(c, a.previewText).FontSize(13).Wrap() })
			}
		})
		if r.Kind != "clip" {
			return
		}
		ui.Column(c).Padding(10, 16, 12, 16).Gap(6).BorderWidth(1, 0, 0, 0).BorderColor(inkLine).Shrink(0).Children(func() {
			ui.Text(c, "Information").FontSize(11).FontWeight(500).TextColor(inkFaint)
			kind := "Text"
			if r.Clip.Kind == "image" {
				kind = "Image"
			}
			copied := "Unknown"
			if !r.Clip.Time.IsZero() {
				copied = r.Clip.Time.Format("Jan 2, 15:04")
			}
			for _, field := range [][2]string{{"Content type", kind}, {"Size", formatSize(r.Clip.Size)}, {"Copied", copied}} {
				ui.Row(c).Gap(12).Children(func() {
					ui.Text(c, field[0]).FontSize(12).TextColor(inkMuted).Grow(1).SingleLine()
					ui.Text(c, field[1]).FontSize(12).TextColor(inkText).FontFeatures("tnum").SingleLine().Shrink(0)
				})
			}
		})
	})
}

// actionBar mirrors a command palette's footer: where you are (or the
// latest status) on the left, the primary and secondary actions with their
// keys on the right.
func (a *app) actionBar(c *ui.Context, density densityMetrics) {
	ui.Row(c).Padding(7, density.inset-4, 7, density.inset).Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(inkLine).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Icon(c, appMark).Size(16, 16).TextColor(inkMuted).Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Height(24).AlignItems(ui.Center).DragWindow().Children(func() {
			label := "Launchr"
			if a.tab != "Apps" {
				label = "Launchr / " + a.tab
			}
			label, ink := a.statusLabel(label)
			ui.Text(c.Key("status:"+label), label).FontSize(12).TextColor(ink).SingleLine().MinWidth(0).Tooltip(label).Transition(statusTransition)
		})
		r, ok := a.selected()
		if !ok {
			return
		}
		actionButton(c, resultAction(r), "Enter").OnClick(a.activate)
		secondary, key := "", []string{}
		switch r.Kind {
		case "app":
			secondary, key = "Pin", []string{"Ctrl", "P"}
			if slices.Contains(a.prefs.Pins, r.App.ID) {
				secondary = "Unpin"
			}
		case "clip":
			secondary, key = "Delete", []string{"Ctrl", "Del"}
		}
		if secondary != "" {
			ui.Row(c).Size(1, 14).Background(inkLine).Shrink(0)
			b := actionButton(c, secondary, key...)
			if secondary == "Delete" {
				b.OnClick(a.deleteClip)
			} else {
				b.OnClick(a.pin)
			}
		}
		if r.Kind == "app" {
			ui.Row(c).Size(1, 14).Background(inkLine).Shrink(0)
			enabled(actionButton(c, "Run as admin", "Ctrl", "Shift", "Enter"), !a.actionBusy).OnClick(a.activateElevated)
		}
	})
}

func (a *app) composeView(c *ui.Context) {
	density := a.density()
	ui.Row(c).Padding(density.searchY, density.inset).Gap(density.gap).AlignItems(ui.Center).Shrink(0).Children(func() {
		backButton(c).OnClick(a.back)
		ui.Text(c, "Quick capture").FontSize(16).FontWeight(500).Grow(1).SingleLine().DragWindow()
	})
	ui.Row(c).Height(1).FillWidth().Background(inkLine).Shrink(0)
	content := ui.Column(c.Key("compose")).Grow(1).FillWidth().MinHeight(0).Padding(density.inset).Gap(density.gap)
	appear(content, 0).Children(func() {
		c.OnShortcut(ui.Cmd, ui.KeyEnter, a.createNote)
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyEnter, a.appendDaily)
		ui.TextInput(c.Key("note-title"), &a.noteTitle).Bind(&a.formInput).Label("Note title").Placeholder("Note title").FillWidth()
		ui.TextArea(c, &a.noteBody).Label("Note content").Placeholder("Write a note…").Padding(density.scale(12), density.scale(24), density.scale(12), density.scale(12)).Grow(1).FillWidth()
	})
	a.statusStrip(c, density)
	ui.Row(c).Padding(7, density.inset-4, 7, density.inset).Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(inkLine).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Icon(c, appMark).Size(16, 16).TextColor(inkMuted).Shrink(0)
		label, ink := a.statusLabel("Launchr / Obsidian")
		ui.Text(c.Key("status:"+label), label).FontSize(12).TextColor(ink).Grow(1).MinWidth(0).SingleLine().Tooltip(label).Transition(statusTransition)
		canCreate := !a.actionBusy && strings.TrimSpace(a.noteTitle) != "" && strings.TrimSpace(a.noteBody) != ""
		canAppend := !a.actionBusy && strings.TrimSpace(a.noteBody) != ""
		enabled(actionButton(c, "Create note", "Ctrl", "Enter"), canCreate).OnClick(a.createNote)
		ui.Row(c).Size(1, 14).Background(inkLine).Shrink(0)
		enabled(actionButton(c, "Append to daily note", "Ctrl", "Shift", "Enter"), canAppend).OnClick(a.appendDaily)
	})
}
