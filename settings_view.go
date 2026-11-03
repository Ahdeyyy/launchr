package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// settingsCard groups related preferences. Cards rise in one after another
// when Settings opens: an occasional screen, so a short stagger is welcome.
func settingsCard(c *ui.Context, index int, icon, title, detail string, body func()) {
	t := c.Theme()
	card := ui.Column(c.Key(title)).FillWidth().Padding(4*t.Spacing).Gap(3*t.Spacing).Radius(12).Background(t.Surface).Border(1, inkLine)
	appear(card, time.Duration(index)*40*time.Millisecond).Children(func() {
		ui.Row(c).Gap(3 * t.Spacing).AlignItems(ui.Center).Children(func() {
			ink := pluginColor(icon)
			ui.Row(c).Size(32, 32).Center().Radius(8).Background(ink.Alpha(.18)).Shrink(0).Children(func() { ui.Icon(c, rowIcons[icon]).Size(18, 18).TextColor(ink) })
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Spacing).Children(func() {
				ui.Text(c, title).FontWeight(500)
				ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
		})
		if body != nil {
			body()
		}
	})
}

func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	density := a.density()
	c.OnShortcut(ui.Cmd, ui.KeyEnter, a.saveSettings)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Row(c).Padding(density.searchY, density.inset).Gap(density.scale(12)).AlignItems(ui.Center).Shrink(0).Children(func() {
			backButton(c).OnClick(a.back)
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).DragWindow().Children(func() {
				ui.Text(c, "Settings").FontSize(16).FontWeight(500).SingleLine()
				ui.Text(c, "Preferences and integrations").FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
		})
		ui.Row(c).Height(1).FillWidth().Background(inkLine).Shrink(0)
		ui.Scroll(c.Key("settings-scroll")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(density.scale(16), density.scale(20)+8, density.scale(16), density.scale(20)).Gap(density.gap).Children(func() {
				settingsCard(c, 0, "note", "Obsidian", "Choose where notes and quick captures are saved.", func() {
					ui.Text(c, "Vault name").FontSize(12).TextColor(t.TextMuted)
					ui.TextInput(c.Key("vault"), &a.vaultEdit).Bind(&a.formInput).Label("Obsidian vault").Placeholder("Active vault (default)").FillWidth()
					ui.Text(c, "Leave empty to use the active vault. Enable the CLI in Obsidian → Settings → General.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, 1, "settings", "Launcher density", "Choose how compact or spacious the launcher feels.", func() {
					ui.RadioGroup(c.Key("density"), func() {
						ui.Row(c).Gap(density.scale(16)).Children(func() {
							ui.Radio(c.Key("compact"), &a.densityEdit, "compact", "Compact")
							ui.Radio(c.Key("comfortable"), &a.densityEdit, "comfortable", "Comfortable")
							ui.Radio(c.Key("spacious"), &a.densityEdit, "spacious", "Spacious")
						})
					})
					ui.Text(c, "Preview spacing instantly. Save to keep this density across the app.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, 2, "window", "Window size", "Choose how much screen space the launcher uses.", func() {
					ui.RadioGroup(c.Key("window-size"), func() {
						ui.Row(c).Gap(density.scale(16)).Children(func() {
							ui.Radio(c.Key("small"), &a.windowSizeEdit, "small", "Small")
							ui.Radio(c.Key("medium"), &a.windowSizeEdit, "medium", "Medium")
							ui.Radio(c.Key("large"), &a.windowSizeEdit, "large", "Large")
						})
					})
					ui.Text(c, "Small: 640 × 420 · Medium: 760 × 520 · Large: 960 × 680. Applies when you save; fits your screen.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, 3, "clip", "Clipboard", "History is stored on this device until you delete it.", func() {
					ui.Text(c, "Storage is unencrypted. Pause capture in Clipboard History whenever you need to.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, 4, "settings", "Keyboard", "Navigate without leaving the keyboard.", func() {
					for _, shortcut := range [][2]string{{"Open launcher", a.shortcut}, {"Switch views", "Ctrl+1–4"}, {"Focus search", "Ctrl+F"}, {"Pin application", "Ctrl+P"}, {"Run as administrator", "Ctrl+Shift+Enter"}, {"Create note", "Ctrl+Enter"}, {"Append to daily note", "Ctrl+Shift+Enter"}} {
						keys := shortcut[1]
						if keys == "" {
							keys = "Alt+Space"
						}
						ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
							ui.Text(c, shortcut[0]).FontSize(12).Grow(1).MinWidth(0)
							keycap(c, keys)
						})
					}
				})
			})
		})
		ui.Row(c).Padding(7, density.inset-4, 7, density.inset).Gap(6).AlignItems(ui.Center).Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(inkLine).Shrink(0).Children(func() {
			ui.Icon(c, appMark).Size(16, 16).TextColor(inkMuted).Shrink(0)
			ui.Text(c, "Launchr / Settings").FontSize(12).TextColor(inkMuted).Grow(1).MinWidth(0).SingleLine()
			actionButton(c, "Discard", "Esc").OnClick(a.back)
			ui.Row(c).Size(1, 14).Background(inkLine).Shrink(0)
			actionButton(c, "Save", "Ctrl", "Enter").OnClick(a.saveSettings)
		})
	})
}
