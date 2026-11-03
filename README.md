# Launchr

A compact native Windows launcher built with Go and MyGo. No bundled browser, background polling, or extension runtime.

Run `go run .` for development, or open `build/windows-amd64/launchr.exe` after building:

```powershell
go tool mygo build -skip-installer .
```

The app lives in the tray. Alt+Space toggles it; Alt+Shift+Space is used if that shortcut is already taken. Closing the panel hides it. Quit from the tray or with Ctrl+Q.

The panel uses a neutral-black, translucent Acrylic backdrop on supported Windows 11 desktops, keeping text opaque over the blurred background. Other desktops use an opaque black fallback. Running windows have a green Running badge and a Switch action; installed apps use Launch. Plugin icons have distinct colors. Settings groups integrations, clipboard information, and keyboard shortcuts in scrollable cards with a fixed Save bar. Lists, text previews, and note inputs reserve space beside their scrollbars.

The app icon is an open capsule framing a floating input bar. `resources/icon.svg` and `resources/mark.svg` are the vector masters; `resources/icon.png` is the packaged 1024px app icon, `resources/icon.ico` contains seven Windows sizes, and `resources/tray.png` is the transparent tray mark with geometry tuned for small sizes. The header uses the same vector mark. The 20 initial concepts, Claude Opus 5.5 judging, shortlist, and final exports are preserved in [design/icon-study](design/icon-study/README.md). With `LAUNCHR_RENDER` set, `go test -run TestRenderBranding` exports the master artwork at 16, 32, 256, and 1024 pixels for review.

Settings → Launcher density offers Compact, Comfortable (default), and Spacious. Save to apply and remember your choice across restarts. Density adjusts result height, icons, search size, and spacing while retaining the scrollbar gutter and readable result text. Escape leaves settings without applying edits.

Settings → Window size offers Small (640 × 420), Medium (760 × 520, default), and Large (960 × 680), independently of density. Save resizes the launcher and remembers your choice across restarts. The window fits within the current screen's available work area, including when opened on another monitor. Escape discards an unsaved size selection.

| View | Behavior |
| --- | --- |
| Apps | Switch to an open window or launch an installed app. Ctrl+Shift+Enter runs the selected app as administrator after the Windows UAC prompt; packaged Store apps cannot be elevated. Pin applications with Ctrl+P or the Pin button. |
| Clipboard | Capture text and images while Launchr runs. Search full text, preview, restore to clipboard with Enter, or delete selected items. Paste into the destination app with Ctrl+V. |
| Calculator | Arithmetic, parentheses, powers, percentages, pi, e, sqrt, abs, sin, cos, tan, log and ln. Trigonometry uses radians. Enter copies the result. |
| Currency | Type `100 USD to NGN` in Apps or Calculator. Also accepts `in` instead of `to`. |
| Obsidian | Search and open notes, create a note with Quick capture, or append the capture text to today's daily note. |

Search for a command by name alongside your apps: Clipboard History, Calculator, Currency Converter, Obsidian, Quick Capture, Open Daily Note, or Settings. Enter runs an action immediately or opens its list/input screen. Escape returns to the previous screen with its search and selection restored; at the main search it clears the query, then hides the launcher. Installed apps and open windows show cached system icons beside their names, with fallback symbols when Windows cannot supply an icon.

Ctrl+1 through Ctrl+4 switches views. Up/Down selects results; Page Up/Down moves through results while the search is focused. Ctrl+F focuses search, Ctrl+N selects the next result, Ctrl+P pins an app, Ctrl+Shift+Enter runs the selected app as administrator, Ctrl+Delete deletes the selected clipboard entry, and Ctrl+L loads older clipboard entries when available. Tab/Shift+Tab moves between controls, with a visible focus ring. Ctrl+, opens Settings; Ctrl+Enter saves settings or creates a note in Quick Capture, and Ctrl+Shift+Enter appends the capture to today's daily note. Settings accepts an optional Obsidian vault name; empty uses the active vault.

Clipboard data and images stay in `%LOCALAPPDATA%\Launchr\launchr.db`, with no item limit or automatic expiry. Storage is limited by available disk space. Identical content is deduplicated and moved to the top. The embedded bbolt database keeps payloads on disk; the UI loads 100 results at a time and virtualizes rows. Search uses a persistent Unicode-aware substring index, with exact verification and a bounded-index fallback for high-entropy items. Existing archives are indexed in cancellable background batches on first upgrade; text searches wait for that initial migration, while recent history remains available. Image previews use stored thumbnails, created lazily for older images, and a single cancellable preview worker. Clipboard capture uses Windows notifications and runs only while Launchr is open. Native reads, image conversion, and storage use workers outside the UI thread. Notifications for the same current clipboard are coalesced. Native snapshots are limited to 64 MiB, and the pending queue is bounded to 32 snapshots / 64 MiB; during overload the newest snapshots replace older pending ones and a status message reports the overflow. History already on disk is not expired. Very large images can still be restored even when their decoded size exceeds the preview budget. If new copies arrive while you browse older pages, the current page is retained; reopen Clipboard to see the newest items. Pause capture before copying content you do not want saved. Storage is unencrypted. Deleting an item removes it from history and makes its space reusable; it is not a secure disk erase or immediate file compaction. `LAUNCHR_DATA_DIR` overrides the storage directory.

Currency conversion uses the [Frankfurter API](https://frankfurter.dev/), with daily reference rates cached for six hours. Previously fetched rates work offline and show their date. Expired cached rates appear immediately while a deduplicated background refresh runs; failed refreshes back off for one minute. Unsupported currencies and uncached offline requests show an error. Only the currency pair is sent; calculations and clipboard contents stay local. Displayed conversions round to two decimal places and are estimates, not bank quotes.

The [official Obsidian CLI](https://obsidian.md/help/cli) must be installed, registered in PATH, and enabled in Obsidian's Settings → General. Keep Obsidian running. Commands run directly as separate arguments, without a shell. The daily-note action uses Obsidian's daily-note configuration. Note creation does not request overwrite. Search returns up to 100 matching notes; narrow the query for more specific results.

Verification:

```powershell
$env:CGO_ENABLED='0'
go test ./...
go tool mygo vet .
```

Set `LAUNCHR_RENDER` to a directory to render all four views during tests. Set `LAUNCHR_LIVE_OBSIDIAN=1` for an additional read-only CLI search check against a running Obsidian instance. The normal tests use a local HTTP fixture for currency caching and do not access your vault or modify your system clipboard.

This first version targets Windows. App focus, shortcut conflicts, and clipboard formats from individual applications still need broader desktop testing. Clipboard search verifies indexed candidates on disk; common queries and high-entropy fallback items can still require more work on large archives, without blocking the interface. Shell icons are extracted on a dedicated COM worker and cached in bounded memory. The executable's size is not a measurement of runtime memory or CPU usage.
