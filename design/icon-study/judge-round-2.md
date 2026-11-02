**Verdict: ship the master. Don't ship the tray yet.** The icon artwork is done. Three small fixes are needed before release, all in how the tray icon is produced and packaged.

## Master icon (`03-refined.svg`, `final-256.png`)
- Compared with `drafts/07.png`, the refinement is clearly better. The longer arc tips make the ring read as the left half of a capsule rather than a "C". The thicker pill is now the main element, and the gaps are wider.
- The geometry checks out in the SVG:
  - The glyph spans x 44 to 212, so both side margins are 44 and it sits centred on 128.
  - The pill is 2 units higher than true centre (gap above it 20, below it 24). That reads as centred.
  - The gap between the pill and the inside of the ring is 32 units.
- The longer pill (ending at 212 rather than the brief's 204) is the right call. With rounded ends on both sides, equal margins look balanced.
- It's polished and minimal. I have no objection to the artwork itself.
- **Power/toggle confusion:** the risk is real but small. It shares the basic idea of a power or logout icon: a ring with a bar entering through its opening. It's saved by three things: the opening is a full 180° rather than the power symbol's small notch, the pill is thick with rounded ends, and the pill extends past the ring. It doesn't look like a toggle switch. At 16px, tell it apart by eye from the "C-"/"G" letter reading, not from the power icon.
- **Uniqueness:** a half-ring with a bar is a common geometric logo shape. It's distinctive for this app, not provably unique, so don't claim it is.

## Tray glyphs: alpha values checked pixel by pixel
- **16px:** good. The pill is exactly rows 6–9 and columns 7–14, the ring's flat ends are crisp at column 8, and the ring is 3px thick. There's a 2px fully transparent gap between ring and pill sideways, and about 2px vertically (row 5 is clear; row 4 has one faint pixel at alpha 21).
- **24px:** good. Ring 5px, pill 6×13, a 3px gap sideways and 2px vertically, and pill and ring share the same centre. The only soft pixels are the unavoidable anti-aliasing on the rounded pill ends.
- **20px: proportions are off.** The pill is 6px tall against a 4px ring and only 10px long (about 1.7:1). Scaling the master would give a pill about 4.7px tall and a length ratio of about 2.2:1. The result looks like a stubby blob next to the 16 and 24 versions. Fix it in `tray-20.svg` with `<rect x="9" y="8" width="10" height="4" rx="2"/>`. That stays on the pixel grid, matches the ring weight, and widens the vertical gap.

## Required fixes
1. **The tuned tray sizes never ship.** `render.go` rasterises only `tray-16.svg`, at 32px, into `resources/tray.png`, and `main.go:761` embeds that one PNG. At 125% and 150% display scaling, Windows shrinks that 32px image to 20 or 24 pixels, which blurs it. The hand-tuned 20 and 24 versions are never used. Pack 16, 20, 24 and 32 into a tray `.ico`, or choose the PNG based on display scale.
2. **There's no tray variant for a light taskbar.** The tray glyph is `#f2f2f0` (near-white), so on Windows' light taskbar theme it's nearly invisible. `tray-dark-*.png` files exist but aren't wired up. Either switch between light and dark based on the system theme, or add a dark outline.
3. **Apply the 20px pill fix above.**

## Not blockers
- **The 4× zoom boxes in the proof sheet are misleading.** `04-refined-proof.png` shows them as faint white-on-white. That's because `proof.Set` copies pixels straight from the image, transparency included, so the boxes are see-through and the viewer shows white through them. The tray PNGs themselves are fine. To make the proof readable, draw the zooms over the grey proof background (`draw.Over` onto `proof`) instead of copying pixels straight in with `proof.Set`.
- **The tiled `final-16` and `final-32` are soft.** At 16px the pill is about 2 bright rows with grey halos above and below. At 32px the arc tips (x = 140) and the pill's bottom edge (y = 148) land on half pixels and come out around 130 grey. They're legible, but if you want them crisp in Explorer and title bars, make a hand-tuned 32px version (for example, tips at x = 144 and pill at y 104–152).

I made no edits to the workspace. The pixel dumps came from a throwaway Go script in `/tmp`.
