package main

import "github.com/egoist/mygo/ui"

func settingsCard(c *ui.Context, icon, title, detail string, body func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(4*t.Spacing).Gap(3*t.Spacing).Radius(12).Background(t.Surface).Border(1, t.Border.Alpha(.65)).Children(func() {
		ui.Row(c).Gap(3 * t.Spacing).AlignItems(ui.Center).Children(func() {
			ink := pluginColor(icon)
			ui.Row(c).Size(32, 32).Center().Radius(9).Background(ink.Alpha(.18)).Shrink(0).Children(func() { ui.Icon(c, rowIcons[icon]).Size(18, 18).TextColor(ink) })
			ui.Column(c).Grow(1).Gap(t.Spacing).Children(func() {
				ui.Text(c, title).Bold()
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
		ui.Row(c).Padding(density.scale(12), density.scale(20), density.scale(16), density.scale(20)).Gap(density.scale(14)).AlignItems(ui.Center).Children(func() {
			plainButton(c, "Back").Padding(6, 10).Radius(8).Background(t.Surface).TextColor(t.Accent).OnClick(a.back)
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "Settings").FontSize(22).Bold()
				ui.Text(c, "Preferences and integrations").FontSize(12).TextColor(t.TextMuted)
			})
		})
		ui.Scroll(c.Key("settings-scroll")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(0, density.scale(20)+8, density.scale(16), density.scale(20)).Gap(density.gap).Children(func() {
				settingsCard(c, "note", "Obsidian", "Choose where notes and quick captures are saved.", func() {
					ui.Text(c, "Vault name").FontSize(12).TextColor(t.TextMuted)
					ui.TextInput(c.Key("vault"), &a.vaultEdit).Bind(&a.formInput).Label("Obsidian vault").Placeholder("Active vault (default)").FillWidth()
					ui.Text(c, "Leave empty to use the active vault. Enable the CLI in Obsidian → Settings → General.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, "settings", "Launcher density", "Choose how compact or spacious the launcher feels.", func() {
					ui.RadioGroup(c.Key("density"), func() {
						ui.Row(c).Gap(density.scale(16)).Children(func() {
							ui.Radio(c.Key("compact"), &a.densityEdit, "compact", "Compact")
							ui.Radio(c.Key("comfortable"), &a.densityEdit, "comfortable", "Comfortable")
							ui.Radio(c.Key("spacious"), &a.densityEdit, "spacious", "Spacious")
						})
					})
					ui.Text(c, "Preview spacing instantly. Save to keep this density across the app.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, "window", "Window size", "Choose how much screen space the launcher uses.", func() {
					ui.RadioGroup(c.Key("window-size"), func() {
						ui.Row(c).Gap(density.scale(16)).Children(func() {
							ui.Radio(c.Key("small"), &a.windowSizeEdit, "small", "Small")
							ui.Radio(c.Key("medium"), &a.windowSizeEdit, "medium", "Medium")
							ui.Radio(c.Key("large"), &a.windowSizeEdit, "large", "Large")
						})
					})
					ui.Text(c, "Small: 640 × 420 · Medium: 760 × 520 · Large: 960 × 680. Applies when you save; fits your screen.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, "clip", "Clipboard", "History is stored on this device until you delete it.", func() {
					ui.Text(c, "Storage is unencrypted. Pause capture in Clipboard History whenever you need to.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
				settingsCard(c, "settings", "Keyboard", "Navigate without leaving the keyboard.", func() {
					for _, shortcut := range [][2]string{{"Open launcher", a.shortcut}, {"Switch views", "Ctrl+1–4"}, {"Focus search", "Ctrl+F"}, {"Pin application", "Ctrl+P"}, {"Create note", "Ctrl+Enter"}, {"Append to daily note", "Ctrl+Shift+Enter"}} {
						keys := shortcut[1]
						if keys == "" {
							keys = "Alt+Space"
						}
						ui.Row(c).Gap(16).Children(func() {
							ui.Text(c, shortcut[0]).FontSize(12).Grow(1)
							ui.Text(c, keys).FontSize(12).TextColor(t.TextMuted)
						})
					}
				})
			})
		})
		ui.Row(c).Padding(density.scale(12), density.scale(20)).Gap(density.gap).AlignItems(ui.Center).Background(t.Surface).Children(func() {
			ui.Text(c, "Esc Back").FontSize(12).TextColor(t.TextMuted).Grow(1)
			ui.Text(c, "Ctrl+Enter").FontSize(12).TextColor(t.TextMuted)
			ui.PrimaryButton(c, "Save").OnClick(a.saveSettings)
		})
	})
}
