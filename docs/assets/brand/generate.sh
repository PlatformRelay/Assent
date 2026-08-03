#!/usr/bin/env bash
set -euo pipefail

brand_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

command -v rsvg-convert >/dev/null || {
  echo "rsvg-convert is required (librsvg)" >&2
  exit 1
}

rsvg-convert --width 1280 --height 640 \
  --output "${brand_dir}/assent-social-github.png" \
  "${brand_dir}/assent-social-github.svg"
rsvg-convert --width 1200 --height 630 \
  --output "${brand_dir}/assent-social-og.png" \
  "${brand_dir}/assent-social-og.svg"
rsvg-convert --width 512 --height 512 \
  --output "${brand_dir}/assent-mark-512.png" \
  "${brand_dir}/assent-mark.svg"
rsvg-convert --width 32 --height 32 \
  --output "${brand_dir}/favicon-32.png" \
  "${brand_dir}/assent-mark.svg"

echo "Generated assent brand raster assets in ${brand_dir}"
