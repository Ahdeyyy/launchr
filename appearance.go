package main

import (
	"embed"
	"time"

	"github.com/egoist/mygo/ui"
)

// Neutral grays keep every surface achromatic; only plugin icons and the
// Running state carry color.
var (
	inkBackground = ui.Hex("#101010")
	inkSurface    = ui.Hex("#171717")
	inkHover      = ui.Hex("#222222")
	inkSelected   = ui.Hex("#2a2a2a")
	inkPressed    = ui.Hex("#323232")
	inkLine       = ui.Hex("#ffffff").Alpha(.08)
	inkText       = ui.Hex("#ededed")
	inkMuted      = ui.Hex("#929292")
	inkFaint      = ui.Hex("#6b6b6b")
	inkRunning    = ui.Hex("#8de0bd")
)

// Motion tokens. Showing and hiding the launcher, moving the selection, and
// switching the preview stay instant: they run hundreds of times a day from
// the keyboard. Motion is kept to feedback (presses, hovers, status) and to
// screens that open occasionally.
var (
	easeOut     = ui.CubicBezier(0.23, 1, 0.32, 1)
	easeHover   = ui.CubicBezier(0.25, 0.1, 0.25, 1)
	pressMotion = ui.Animation{Duration: 120 * time.Millisecond, Ease: easeOut}
	hoverMotion = ui.Animation{Duration: 150 * time.Millisecond, Ease: easeHover}
	slideMotion = ui.Animation{Duration: 180 * time.Millisecond, Ease: easeOut}
)

// appear fades content in from slightly below as it is first built, after
// delay, so cards in a newly opened screen arrive one after another.
func appear(e ui.Element, delay time.Duration) ui.Element {
	p := e.Keyframes("appear", []ui.Keyframe{{At: 0}, {At: delay + 1}, {At: delay + 220*time.Millisecond, Value: 1, Ease: easeOut}})
	return e.Opacity(p).Translate(0, 8*(1-p))
}

// statusTransition brings a status message up into the action bar and
// lets the previous one fade down the same way.
var statusTransition = ui.ElementTransition{Animation: &slideMotion, Enter: &ui.Motion{Y: 6}, Exit: &ui.Motion{Y: 6}}

// tabIndicator identifies the pill that slides between view tabs.
type tabIndicator struct{}

func keycap(c *ui.Context, key string) ui.Element {
	return ui.Text(c, key).FontSize(11).TextColor(inkMuted).Padding(1, 6).Radius(5).Background(inkHover).Shrink(0).SingleLine()
}

// actionButton is a quiet, keyboard-labelled action in the style of a
// command palette's action bar: a hover fill that eases in and a slight
// press-down scale for feedback.
func actionButton(c *ui.Context, label string, keys ...string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Padding(4, 6, 4, 8).Radius(7).Shrink(0)
	bg := ui.Transparent
	if b.Pressed() {
		bg = inkPressed
	} else if b.Hovered() {
		bg = inkHover
	}
	scale := float32(1)
	if b.Pressed() {
		scale = .97
	}
	scale = b.AnimateTo("press", scale, pressMotion)
	return b.Background(b.AnimateColor("fill", bg, hoverMotion)).Scale(scale, scale).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(inkText).SingleLine()
			for _, key := range keys {
				keycap(c, key)
			}
		})
	})
}

// chipButton is a compact ghost button for toolbar actions and Back.
func chipButton(c *ui.Context, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Padding(4, 10).Radius(7).Shrink(0)
	bg, ink := ui.Transparent, inkMuted
	if b.Pressed() {
		bg, ink = inkPressed, inkText
	} else if b.Hovered() {
		bg, ink = inkHover, inkText
	}
	scale := float32(1)
	if b.Pressed() {
		scale = .97
	}
	scale = b.AnimateTo("press", scale, pressMotion)
	return b.Background(b.AnimateColor("fill", bg, hoverMotion)).Scale(scale, scale).Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(b.AnimateColor("ink", ink, hoverMotion)).SingleLine()
	})
}

// loadingBar replaces the hairline under the search field while a search
// is running. It waits 150 ms before showing so instant results never
// flicker, then sweeps like an indeterminate progress bar; with reduced
// motion it pulses in place instead.
func loadingBar(c *ui.Context, busy bool) {
	line := ui.Row(c.Key("search-line")).Height(1).FillWidth().Background(inkLine).Shrink(0)
	if !busy {
		return
	}
	line.Children(func() {
		bar := ui.Row(c.Key("loading")).Fill()
		show := bar.Keyframes("show", []ui.Keyframe{{At: 0}, {At: 150 * time.Millisecond}, {At: 300 * time.Millisecond, Value: 1}})
		if c.Preferences().ReduceMotion {
			pulse := bar.Loop("pulse", 1200*time.Millisecond, ui.Bounce(ui.EaseInOut))
			bar.Background(inkText.Alpha(.18 + .32*pulse)).Opacity(show)
			return
		}
		p := bar.Loop("sweep", 1100*time.Millisecond, ui.EaseInOut)
		bar.Opacity(show).Children(func() {
			ui.Spacer(c).Grow(p)
			ui.Row(c).WidthPercent(28).FillHeight().Background(inkText.Alpha(.7)).Shrink(0)
			ui.Spacer(c).Grow(1 - p)
		})
	})
}

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
	ui.Row(c).Size(size, size).Center().Shrink(0).Background(ink.Alpha(.18)).Radius(8).Children(func() {
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

// enabled dims an action that cannot run yet and stops it taking input.
func enabled(e ui.Element, on bool) ui.Element {
	if on {
		return e
	}
	return e.Disabled(true).Opacity(.4)
}
