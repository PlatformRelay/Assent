# Brand assets

The assent mark is an abstract lowercase **a** and decision gate. Coral and cobalt paths
enter a deterministic boundary and leave through the gate's central channel.

## Primary assets

| Asset | Use |
| --- | --- |
| `assent-mark.svg` | Standalone mark without writing; icons and square placements |
| `assent-logo.svg` | Horizontal mark and `assent` wordmark |
| `assent-social-github.svg` / `.png` | GitHub repository social preview, 1280×640 |
| `assent-social-og.svg` / `.png` | Open Graph social card, 1200×630 |
| `assent-mark-512.png` | Square raster mark |
| `favicon-32.png` | Small raster icon |

The SVGs are the source assets. Regenerate all raster outputs with:

```bash
./assets/brand/generate.sh
```

The generator requires `rsvg-convert` from librsvg.

## Palette

| Name | Hex |
| --- | --- |
| Ink | `#172033` |
| Cobalt | `#316BFF` |
| Coral | `#FF5C4D` |
| Warm white | `#F8F7F2` |

Keep the mark's proportions and colors intact. Use the standalone mark when the word
`assent` already appears next to it; otherwise prefer the horizontal logo.
