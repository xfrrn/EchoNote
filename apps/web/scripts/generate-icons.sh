#!/usr/bin/env bash
set -euo pipefail

# Requires ImageMagick 7 (magick). Regenerates PWA icons and iOS launch images.
# 启动图底色与 tokens.css 的 --bg-primary 保持一致，避免启动闪屏。
MAGICK=${MAGICK:-magick}

for spec in 'pwa-192x192.png:192' 'pwa-512x512.png:512' 'pwa-maskable-512x512.png:512'; do
  file=${spec%%:*}
  size=${spec##*:}
  "$MAGICK" -background none public/icons/icon-source.svg -resize "${size}x${size}" -depth 8 "public/icons/${file}"
done
"$MAGICK" -background none public/icons/icon-source.svg -resize 180x180 -depth 8 -alpha off -type TrueColor public/icons/apple-touch-icon.png

"$MAGICK" -background none -density 192 public/icons/splash-glyph.svg -resize 512x512 -depth 8 /tmp/echonote-splash-glyph.png

for size in 1320x2868 1206x2622 1290x2796 1179x2556 1284x2778 1170x2532 1242x2688 1125x2436 1080x2340 828x1792 750x1334; do
  "$MAGICK" -size "$size" xc:'#f5f3ee' \( /tmp/echonote-splash-glyph.png -resize 360x360 \) -gravity center -composite -alpha off -type TrueColor -depth 8 "public/icons/apple-splash-light-$size.png"
  "$MAGICK" -size "$size" xc:'#15120e' \( /tmp/echonote-splash-glyph.png -resize 360x360 \) -gravity center -composite -alpha off -type TrueColor -depth 8 "public/icons/apple-splash-dark-$size.png"
done
