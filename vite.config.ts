import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    react(),
    VitePWA({
      // 开发模式不注入/缓存 SW，避免干扰 UI 调试；生产构建仍会完整生成 PWA。
      disable: process.env.NODE_ENV === 'development',
      registerType: 'autoUpdate',
      includeAssets: [
        'icons/apple-touch-icon.png',
        'icons/favicon.svg',
        'icons/apple-splash-light-*.png',
        'icons/apple-splash-dark-*.png',
        'manifest.webmanifest',
        'manifest-dark.webmanifest'
      ],
      manifest: false,
      workbox: {
        globPatterns: ['**/*.{js,css,html,ico,png,svg,webmanifest}'],
        navigateFallback: '/index.html',
        cleanupOutdatedCaches: true
      },
      devOptions: {
        enabled: false
      }
    })
  ]
})
