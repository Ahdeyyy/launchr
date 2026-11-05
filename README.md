# Launchr

A fast, lightweight launcher for Windows. Press a shortcut, type, and press Enter to open apps, switch windows, search your clipboard history, do quick maths, convert currencies, or capture notes in Obsidian.

Launchr is a native Go app with no bundled browser. It runs in the system tray and opens with **Alt+Space** (or **Alt+Shift+Space** if Alt+Space is already taken).

## Features

- **Apps and windows**: launch installed apps or switch to open windows. Pin your favourites with Ctrl+P, and run an app as administrator with Ctrl+Shift+Enter.
- **Clipboard history**: saves copied text and images while Launchr runs. You can search, preview, restore, or delete entries. History is stored locally in `%LOCALAPPDATA%\Launchr\launchr.db`, unencrypted.
- **Calculator**: arithmetic, percentages, powers, and functions like `sqrt`, `sin`, and `log`. Press Enter to copy the result.
- **Currency conversion**: type something like `100 USD to NGN`. Rates come from the [Frankfurter API](https://frankfurter.dev/) and are cached so they also work offline.
- **Obsidian**: search and open notes, create a new note, or append to today's daily note. This needs the [Obsidian CLI](https://obsidian.md/help/cli) installed and enabled.
- **Settings**: choose a layout density and window size, and set your Obsidian vault.

### Keyboard shortcuts

| Shortcut | Action |
| --- | --- |
| Alt+Space | Show or hide Launchr |
| Enter | Run the selected result |
| Escape | Go back, clear the search, or hide |
| Ctrl+1 – Ctrl+4 | Switch views |
| Ctrl+, | Open Settings |
| Ctrl+Q | Quit |

## Running locally

Requirements: Windows and [Go](https://go.dev/dl/) 1.27 or later.

```powershell
git clone <repo-url>
cd launchr
go run .
```

## Building

```powershell
go tool mygo build -skip-installer .
```

The app is written to `build/windows-amd64/launchr.exe`. If you leave out `-skip-installer`, the build also produces an installer.

## Testing

```powershell
$env:CGO_ENABLED='0'
go test ./...
go tool mygo vet .
```
