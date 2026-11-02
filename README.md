# Launchr

A compact native Windows launcher built with Go and MyGo. No bundled browser, background polling, or extension runtime.

Run `go run .` for development, or open `build/windows-amd64/launchr.exe` after building:

```powershell
go tool mygo build -skip-installer .
```

The app lives in the tray. Alt+Space toggles it; Alt+Shift+Space is used if that shortcut is already taken. Closing the panel hides it. Quit from the tray or with Ctrl+Q.

| View | Behavior |
| --- | --- |
| Apps | Switch to an open window or launch an installed app. Pin applications with Ctrl+P or the Pin button. |
| Clipboard | Capture text and images while Launchr runs. Search full text, preview, restore to clipboard with Enter, or delete selected items. Paste into the destination app with Ctrl+V. |
| Calculator | Arithmetic, parentheses, powers, percentages, pi, e, sqrt, abs, sin, cos, tan, log and ln. Trigonometry uses radians. Enter copies the result. |
| Currency | Type `100 USD to NGN` in Apps or Calculator. Also accepts `in` instead of `to`. |
| Obsidian | Search and open notes, create a note with Quick capture, or append the capture text to today's daily note. |

Ctrl+1 through Ctrl+4 switches views. Arrow keys select results. Escape clears a search, leaves a form, or hides the launcher. Settings accepts an optional Obsidian vault name; empty uses the active vault.

Clipboard data and images stay in `%LOCALAPPDATA%\Launchr\launchr.db`, with no item limit or automatic expiry. Storage is limited by available disk space. Identical content is deduplicated and moved to the top. The embedded bbolt database keeps payloads on disk; the UI loads 100 results at a time and virtualizes rows. Search runs in cancellable background work. Clipboard capture uses Windows notifications and runs only while Launchr is open. Pause capture before copying content you do not want saved. Storage is unencrypted. Deleting an item removes it from history and makes its space reusable; it is not a secure disk erase or immediate file compaction. `LAUNCHR_DATA_DIR` overrides the storage directory.

Currency conversion uses the [Frankfurter API](https://frankfurter.dev/), with daily reference rates cached for six hours. Previously fetched rates work offline and show their date. Unsupported currencies and uncached offline requests show an error. Only the currency pair is sent; calculations and clipboard contents stay local. Displayed conversions round to two decimal places and are estimates, not bank quotes.

The [official Obsidian CLI](https://obsidian.md/help/cli) must be installed, registered in PATH, and enabled in Obsidian's Settings → General. Keep Obsidian running. Commands run directly as separate arguments, without a shell. The daily-note action uses Obsidian's daily-note configuration. Note creation does not request overwrite. Search returns up to 100 matching notes; narrow the query for more specific results.

Verification:

```powershell
$env:CGO_ENABLED='0'
go test ./...
go tool mygo vet .
```

Set `LAUNCHR_RENDER` to a directory to render all four views during tests. Set `LAUNCHR_LIVE_OBSIDIAN=1` for an additional read-only CLI search check against a running Obsidian instance. The normal tests use a local HTTP fixture for currency caching and do not access your vault or modify your system clipboard.

This first version targets Windows. App focus, shortcut conflicts, and clipboard formats from individual applications still need broader desktop testing. Full-text clipboard search scans disk records, so searches over very large archives may take longer; they do not block the interface. The executable's size is not a measurement of runtime memory or CPU usage.
