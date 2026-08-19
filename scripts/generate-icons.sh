#!/usr/bin/env bash
set -euo pipefail

# Requires ImageMagick (convert). Regenerates PWA icons and iOS launch images.
for spec in 'pwa-192x192.png:192' 'pwa-512x512.png:512' 'pwa-maskable-512x512.png:512'; do
  file=${spec%%:*}
  size=${spec##*:}
  convert -background none public/icons/icon-source.svg -resize "${size}x${size}" -depth 8 "public/icons/${file}"
done
convert -background none public/icons/icon-source.svg -resize 180x180 -depth 8 -alpha off -type TrueColor public/icons/apple-touch-icon.png

convert -background none -density 192 public/icons/splash-glyph.svg -resize 512x512 -depth 8 /tmp/echonote-splash-glyph.png

for size in 1320x2868 1206x2622 1290x2796 1179x2556 1284x2778 1170x2532 1242x2688 1125x2436 1080x2340 828x1792 750x1334; do
  convert -size "$size" xc:'#f7f7f8' \( /tmp/echonote-splash-glyph.png -resize 360x360 \) -gravity center -composite -alpha off -type TrueColor -depth 8 "public/icons/apple-splash-light-$size.png"
  convert -size "$size" xc:'#0a0a0a' \( /tmp/echonote-splash-glyph.png -resize 360x360 \) -gravity center -composite -alpha off -type TrueColor -depth 8 "public/icons/apple-splash-dark-$size.png"
done
