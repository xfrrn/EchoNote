import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

const apiProxy = process.env.ECHONOTE_API_PROXY ?? 'http://[::1]:8080'

export default defineConfig({
  server: {
    proxy: {
      '/api': apiProxy
    }
  },
  preview: {
    proxy: {
      '/api': apiProxy
    }
  },
  plugins: [
    react(),
    VitePWA({
      // 开发模式不注入/缓存 SW，避免干扰 UI 调试；生产构建仍会完整生成 PWA。
      disable: process.env.NODE_ENV === 'development',
      registerType: 'autoUpdate',
      manifest: false,
      workbox: {
        // API、SSE 与登录走网络；导航只回退到随 Service Worker revision 更新的静态壳。
        globPatterns: ['**/*.{js,css,html,ico,png,svg,webmanifest}'],
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//],
        cleanupOutdatedCaches: true
      },
      devOptions: {
        enabled: false
      }
    })
  ]
})
