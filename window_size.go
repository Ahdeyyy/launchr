package main

import "github.com/egoist/mygo"

func normalizedWindowSize(value string) string {
	switch value {
	case "small", "large":
		return value
	default:
		return "medium"
	}
}

func launcherWindowSize(value string) (width, height int) {
	switch normalizedWindowSize(value) {
	case "small":
		return 640, 420
	case "large":
		return 960, 680
	default:
		return 760, 520
	}
}

func launcherWindowBounds(value string, area mygo.Rectangle) mygo.Rectangle {
	w, h := launcherWindowSize(value)
	if area.Width > 0 {
		w = min(w, area.Width)
	}
	if area.Height > 0 {
		h = min(h, area.Height)
	}
	return mygo.Rectangle{
		X:     area.X + max(0, area.Width-w)/2,
		Y:     area.Y + max(0, area.Height-h)/3,
		Width: w, Height: h,
	}
}

// Called on the UI thread after saving, and when opening on another display.
func (a *app) applyWindowSize(area mygo.Rectangle) {
	bounds := launcherWindowBounds(a.prefs.WindowSize, area)
	a.w.SetMinimumSize(min(640, bounds.Width), min(420, bounds.Height))
	a.w.SetSize(bounds.Width, bounds.Height)
	a.w.SetPosition(bounds.X, bounds.Y)
}
