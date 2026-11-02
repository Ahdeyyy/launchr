package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

type navigation struct {
	tab, query string
	chosen     int
	composing  bool
}

var commands = []result{
	{ID: "clipboard", Title: "Clipboard History", Detail: "Search and restore copied text and images", Kind: "command", Value: "Clipboard"},
	{ID: "calculator", Title: "Calculator", Detail: "Calculate expressions and convert currencies", Kind: "command", Value: "Calculator"},
	{ID: "currency", Title: "Currency Converter", Detail: "Enter an amount, for example 100 USD to NGN", Kind: "command", Value: "Calculator"},
	{ID: "obsidian", Title: "Obsidian", Detail: "Search notes and browse actions", Kind: "command", Value: "Obsidian"},
	{ID: "capture", Title: "Quick Capture", Detail: "Obsidian · Create a note or append to daily note", Kind: "command", Value: "capture"},
	{ID: "daily", Title: "Open Daily Note", Detail: "Obsidian · Run action", Kind: "daily"},
	{ID: "settings", Title: "Settings", Detail: "Configure Launchr", Kind: "command", Value: "settings"},
}

func (a *app) filterCommands(q string) {
	if a.rows == nil && a.filterBuffer != nil {
		a.rows = a.filterBuffer[:0]
		a.filterBuffer = nil
	}
	words := strings.Fields(strings.ToLower(q))
	// Names rank ahead of descriptions so "daily" runs Daily Note rather
	// than opening another command that merely mentions it.
	for _, titleMatch := range []bool{true, false} {
		for _, command := range commands {
			title := strings.ToLower(command.Title)
			searchable := title + " " + strings.ToLower(command.Detail)
			match := true
			inTitle := true
			for _, word := range words {
				if !strings.Contains(title, word) {
					inTitle = false
				}
				if !strings.Contains(searchable, word) {
					match = false
					break
				}
			}
			if match && inTitle == titleMatch {
				a.rows = append(a.rows, command)
			}
		}
	}
}

func (a *app) openCommand(name string) {
	a.history = append(a.history, navigation{tab: a.tab, query: a.query, chosen: a.chosen, composing: a.composing})
	a.settingsOpen, a.composing = false, false
	switch name {
	case "capture":
		a.composing = true
		a.formInput.Focus()
	case "settings":
		a.settingsOpen = true
		a.vaultEdit = a.prefs.Vault
		a.densityEdit = normalizedDensity(a.prefs.Density)
		a.windowSizeEdit = normalizedWindowSize(a.prefs.WindowSize)
		a.formInput.Focus()
	default:
		a.tab, a.query = name, ""
		a.search(false)
		a.input.Focus()
	}
}

func (a *app) openSettings() {
	if !a.settingsOpen {
		a.openCommand("settings")
	}
}

func (a *app) back() {
	if a.settingsOpen || a.composing {
		a.settingsOpen, a.composing = false, false
	} else if a.tab == "Apps" && a.query != "" {
		a.query = ""
		a.search(false)
		a.input.Focus()
		return
	} else if len(a.history) == 0 && a.tab == "Apps" {
		a.hide()
		return
	}
	if n := len(a.history); n > 0 {
		prev := a.history[n-1]
		a.history = a.history[:n-1]
		a.tab, a.query = prev.tab, prev.query
		a.search(false)
		a.chosen = max(0, prev.chosen)
		if !a.busy {
			a.chosen = min(a.chosen, max(0, len(a.rows)-1))
		}
		a.composing = prev.composing
		a.list.ScrollIntoView(a.chosen)
	} else {
		a.setTab("Apps")
	}
	if a.composing {
		a.formInput.Focus()
	} else {
		a.input.Focus()
	}
}

func (a *app) saveSettings() {
	a.savePrefs(a.settingsDraft())
	a.back()
}

func (a *app) settingsDraft() settings {
	next := a.prefs
	next.Vault = strings.TrimSpace(a.vaultEdit)
	next.Density = normalizedDensity(a.densityEdit)
	next.WindowSize = normalizedWindowSize(a.windowSizeEdit)
	return next
}

func (a *app) createNote() {
	if strings.TrimSpace(a.noteTitle) != "" && strings.TrimSpace(a.noteBody) != "" {
		a.obsidianAction("create", []string{"name=" + a.noteTitle, "content=" + cliContent(a.noteBody)}, true)
	}
}
func (a *app) appendDaily() {
	if strings.TrimSpace(a.noteBody) != "" {
		a.obsidianAction("daily:append", []string{"content=" + cliContent(a.noteBody)}, true)
	}
}

// Only the search field owns these commands; a focused list uses its native
// navigation. This keeps each keypress from moving selection twice.
func (a *app) searchShortcuts(c *ui.Context) {
	a.input.OnShortcut(c, 0, ui.KeyDown, func() { a.choose(1) })
	a.input.OnShortcut(c, 0, ui.KeyUp, func() { a.choose(-1) })
	a.input.OnShortcut(c, 0, ui.KeyPageDown, func() { a.choose(6) })
	a.input.OnShortcut(c, 0, ui.KeyPageUp, func() { a.choose(-6) })
}
