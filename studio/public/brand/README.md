# BeastDB logo kit

The source artwork is SVG. Every logo is transparent and contains only the name **BeastDB**.

| Use | Light surfaces | Dark surfaces |
| --- | --- | --- |
| Header, website, documentation | `beastdb-horizontal-light.svg` | `beastdb-horizontal-dark.svg` |
| Square artwork, social profile | `beastdb-stacked-light.svg` | `beastdb-stacked-dark.svg` |
| App icon, compact navigation | `beastdb-mark-light.svg` | `beastdb-mark-dark.svg` |
| Print in one color | `beastdb-horizontal-mono-black.svg` | `beastdb-horizontal-mono-white.svg` |

Matching transparent PNG exports are included. Use `beastdb-favicon.svg` for browser tabs, the 16/32/48 px favicon PNGs as fallbacks, `beastdb-app-icon-512.png` for app listings, and `beastdb-apple-touch-icon.png` for iOS home screens. The `beastdb-social-*.png` files are 1200 × 630 share cards. The `beastdb-preview-*.png` files show the horizontal logos on their intended backgrounds.

Colors: graphite `#151A16`, dark surface `#101611`, lime on light `#82C900`, lime on dark `#A8F21A`. Keep the logo proportions when resizing and leave at least one claw width of clear space around it.

To use in Adobe Express, upload an SVG from this folder to a new design. The wordmark and database mark remain vector artwork in the SVG source. To change the paths or colors, edit the SVG source and run `node scripts/generate-brand-assets.mjs` from the `studio` directory to regenerate the lockups and PNGs.
