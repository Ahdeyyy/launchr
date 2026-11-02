I checked this visually. I looked at `01-twenty-drafts.png`, read the SVG geometry for the main contenders, and shrank every draft PNG to 16px and 24px to see how it holds up. Those test renders went to a temp folder outside the repo; I changed nothing in the workspace.

## Ranking (best to worst)

| # | Draft | Verdict |
|---|---|---|
| 1 | **07 Open capsule** | The silhouette is unusual: half a ring with a rounded bar ("pill") floating inside it. It reads as the launcher's input bar sitting in an opening, which fits a summon-and-type tool. It stays solid at 16px and leaves clean space between the two parts. |
| 2 | **03 Notched aperture** | The strongest outline at 16px, and the ring's empty center is well judged. The problem is meaning: a ring with a triangle in the corner looks like a refresh/sync arrow or a letter "C". |
| 3 | **13 Launch key** | The most relevant idea, since a keycap fits a keyboard-first app. But it's a rounded square inside a rounded square, which repeats the tile. At 16px the cut-out shrinks to a small "L" and it starts to look like a generic window or widget icon. |
| 4 | 17 Displaced square | Clean and readable at 16px, but it looks like an "add widget" or app-grid icon. Not memorable. |
| 5 | 09 Quarter orbit | The best use of empty space in the set, but it's three separate pieces. At 16px it breaks up and the dot starts to look like a head. |
| 6 | 15 Monoline knot | Reads fine at small size. Looks like a stock tech/crypto hexagon "C". |
| 7 | 18 Cut portal | A good idea, but the diagonal cut is only about 30 units thick and mostly disappears at 16px. What's left reads as "n" or "Ω". |
| 8 | 14 Wedge seam | Energetic, but it reads as a sporty letter "F". |
| 9 | 10 Switch rail | Looks like a flowchart or node-graph icon. Too many corners for its size. |
| 10 | 12 Star slit | Striking, but it looks like the overused AI "sparkle" or a compass, and it has nothing to do with launching. |
| 11 | 04 Magnetic gate | The gaps around the small square go muddy at 16px. Reads as a bracketed "E". |
| 12 | 02 Bent ribbon | Looks like a crop or expand button in a toolbar. |
| 13 | 16 Bracket pulse | Reads as a media play button, and the triangle is only 1–2px at 16px. |
| 14 | 06 Lifted step | A generic stairs or "level up" icon with no link to a launcher. |
| 15 | 20 L relay | Reads clearly as the letter "E", which is the wrong letter for Launchr. |
| 16 | 01 Split vector | The fast-forward media symbol. The most familiar shape in the set. |
| 17 | 05 Folded cursor | A paper-plane / "send" cliché. The fold and thin tips vanish at 16px. |
| 18 | 08 Slingshot | Reads as a person or the letter "Y". |
| 19 | 11 Terminal leaf | The terminal prompt inside is lost at 16px, and a leaf suggests "eco". |
| 20 | 19 Radial wing | The render doesn't match its description. Instead of three swept vanes it's a plain pentagon blob. This draft failed. |

## Shortlist: 07, 03, 13. Winner: **07 Open capsule**

It's the only draft that is simple, has a clear meaning (the bar appearing out of an opening), and isn't already a common UI symbol. Its main risks are being read as a letter ("C-" / "G") or as a logout or toggle icon. The changes below are aimed at those risks.

## Refinement brief for 07 (256 grid, centre at 128,128)

All measurements are in the SVG's 256-unit coordinate space.

1. **Ring (the half circle):**
   - Keep the outer radius at 84 and reduce the inner radius from 46 to 44, making the ring 40 thick.
   - Extend both ends straight right, from x=128 to x=140, still cut flat. The ring then reads as the left half of a big capsule rather than a "C", which ties it to the pill.
2. **Pill (the bar):**
   - Change it to x 116–204, y 106–150 (44 tall, end radius 22).
   - That leaves a 32-unit gap to the ring's inner edge (at x=84), so the gap survives at 16px. At the current 26 it drops to about 1.6px and starts to fill in.
   - The pill becomes slightly thicker than the ring (44 vs 40) so it clearly reads as the main element.
3. **Balance:**
   - Move the pill 2 units up (centre y=126) so it looks centred against the round ring; round ends look lower than they are.
   - Let the pill's right end sit about 8 units further right than the ring's left edge sits from the tile edge. Round ends look shorter than flat edges, so this makes the two sides look equal.
4. **Don't add** a caret, cursor, arrow or second dot. Any of those pushes it back toward "logout", "toggle" or "terminal".
5. **Colour:** keep it to one colour. If you add an accent, put it on the pill only, never on the ring.

## Small-size and tray concerns

- **Tray means no tile.** The 16, 20 and 24px tray icons won't have the dark rounded square behind them. Make a separate glyph with a transparent background, in a white version for dark taskbars and a dark version for light ones. Don't scale the tiled icon down.
- **Hand-tune the 16px version instead of just downscaling:**
  - Ring: outer radius 7, centred at (8,8), 3px thick, flat ends at column 9.
  - Pill: rows 6–9 (4px tall, so it sits centred on an even grid), columns 7–15, with 2px rounded ends.
  - Leave at least a 1px gap, kept as fully transparent pixels, between the ring and the pill.
  - Snap every straight edge to whole pixels.
- **Check 20px and 24px too** (125% and 150% display scaling). The ring should be about 4px thick at 20px and 5px at 24px. Don't let a half-pixel thickness blur into grey.
- **Test it next to real tray icons,** such as Wi-Fi, volume and Teams. The shape that would be easiest to confuse it with is a power or toggle symbol. If it looks too similar in that context, widen the pill-to-ring gap rather than adding detail.

**Fallback:** if 07 tests as too letter-like, go with 03. Remove its separate triangle and instead make the notch a square cut into the ring itself, so it stops looking like a refresh arrow.
