# Launchr icon study

Twenty separately constructed vector silhouettes were explored in a shared monochrome palette. These are rough directions, including rejected work, rather than twenty variations of one symbol.

- `01-twenty-drafts.png`: the initial contact sheet; numbered SVG/PNG originals in `drafts/`.
- `concepts.json`: names and intentions for all twenty.
- `judge-round-1.md`: Claude provider, model `claude-opus-5-5`, visual evaluation and full ranking.
- `judge-round-2.md`: final visual review, approval of the master, and tray findings.
- `02-shortlist.svg`: #07 Open capsule, #03 Notched aperture, #13 Launch key.
- `03-refined.svg`: final selected Open capsule vector.
- `04-refined-proof.png`: final icon alongside actual-size and enlarged tray glyphs.
- `final-*.png`: final app artwork from 16 through 1024px.
- `tray-*.svg` / `tray-*.png`: white transparent glyphs tuned for 16, 20 and 24px.
- `tray-dark-*.png`: dark equivalents for use on light surfaces.

## Selection and refinement

The judge selected Open capsule because its two-part silhouette stays readable at small sizes and suggests an input bar appearing within an opening. The aperture runner-up resembled refresh/sync, and the launch-key runner-up repeated the rounded tile and lost its inner detail at small sizes.

The refinement thickens the ring from 38 to 40 units, extends its flat ends by 12 units, and makes the pill 44 units tall. Its left edge moves from 108 to 116, preserving a 32-unit gap to the inner arc. The pill sits two units above the mathematical center and extends to x=212. The final silhouette uses two forms and one ink color with no fine details or gradients.

The 16px tray glyph has a 3px ring and a 4px pill, with at least one fully transparent pixel between them. The 20px and 24px assets use individually snapped geometry. Following round two, the 20px pill was reduced to four pixels tall. Packaged dark-and-transparent PNGs are selected using Windows' small-icon metric at startup; unsupported sizes use the 32px fallback. MyGo's `IconIsTemplate` automatically changes the ink to white on dark taskbars and follows taskbar theme changes. Display scaling changes during a running session require restarting Launchr to select a different optical size.

## Reproduce

From the repository root:

```powershell
$env:CGO_ENABLED='0'
go run design/icon-study/render.go design/icon-study/03-refined.svg
```

Append `-install` to update the packaged icon SVG, 1024px PNG, seven-size ICO, and tray PNGs. The header's `resources/mark.svg` shares the final geometry with a `currentColor` fill.

Verification: `go test ./...`, `go tool mygo vet .`, and `go tool mygo build -skip-installer .`. Native Windows tray appearance beside other applications has not been checked interactively.
