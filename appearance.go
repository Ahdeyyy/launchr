package main

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed resources/fonts/*.ttf
var fonts embed.FS

func init() {
	for _, weight := range []string{"Regular", "Medium", "Bold"} {
		data, err := fonts.ReadFile("resources/fonts/MartianGrotesk-" + weight + ".ttf")
		if err != nil {
			panic(err)
		}
		if err := ui.RegisterFont(data, "Martian Grotesk"); err != nil {
			panic(err)
		}
	}
}

func (a *app) resultIcon(c *ui.Context, r result, kind string) {
	size := a.density().icon
	path := ""
	if r.Kind == "app" {
		path = r.App.Path
	}
	if r.Kind == "window" {
		path = r.Window.Path
	}
	if icon := a.icons[path]; icon != nil {
		ui.Image(c, icon).Size(size-2, size-2).Fit(ui.Contain).Shrink(0)
		return
	}
	if path != "" && a.w != nil && !a.iconPending[path] {
		if a.iconPending == nil {
			a.iconPending = map[string]bool{}
		}
		a.iconPending[path] = true
		select {
		case a.iconJobs <- path:
		default:
			delete(a.iconPending, path)
		}
	}
	if kind == "command" {
		kind = "settings"
		switch r.Value {
		case "Clipboard":
			kind = "clip"
		case "Calculator":
			kind = "calculator"
		case "Obsidian", "capture":
			kind = "note"
		}
	}
	if r.ID == "currency" && r.Kind == "command" {
		kind = "currency"
	}
	ink := pluginColor(kind)
	ui.Row(c).Size(size, size).Center().Shrink(0).Background(ink.Alpha(.20)).Radius(9).Children(func() {
		ui.Icon(c, rowIcons[kind]).Size(20, 20).TextColor(ink)
	})
}

func pluginColor(kind string) ui.Color {
	switch kind {
	case "clip", "image":
		return ui.Hex("#ffa781")
	case "calculator", "answer":
		return ui.Hex("#7acbff")
	case "currency":
		return ui.Hex("#f4cd71")
	case "note":
		return ui.Hex("#c3a0ff")
	case "daily":
		return ui.Hex("#f49bcd")
	case "window":
		return ui.Hex("#8de0bd")
	case "settings":
		return ui.Hex("#a6bbd7")
	default:
		return ui.Hex("#deded9")
	}
}

func resultAction(r result) string {
	switch r.Kind {
	case "window":
		return "Switch"
	case "app":
		return "Launch"
	case "daily":
		return "Run"
	case "clip", "answer":
		return "Copy"
	default:
		return "Open"
	}
}
