import { useEffect, useState } from 'react'
import { useTestMode, type ThemeMode } from '../store/test-mode'

function systemDark(): boolean {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function mediaWithoutColorScheme(media: string): string {
  return media
    .replace('(prefers-color-scheme: light) and ', '')
    .replace('(prefers-color-scheme: dark) and ', '')
}

export function syncAppleLaunchScreen(mode: ThemeMode, resolved: 'light' | 'dark'): void {
  const links = document.querySelectorAll<HTMLLinkElement>('link[rel="apple-touch-startup-image"]')

  links.forEach((link) => {
    const originalMedia = link.getAttribute('data-original-media') ?? link.getAttribute('media') ?? ''
    const isDarkImage = (link.getAttribute('href') ?? '').includes('/apple-splash-dark-')

    if (mode === 'system') {
      link.setAttribute('media', originalMedia)
      return
    }

    const wanted = resolved === 'dark' ? isDarkImage : !isDarkImage
    link.setAttribute('media', wanted ? mediaWithoutColorScheme(originalMedia) : '(min-width: 100000px)')
  })
}

export function useResolvedTheme(): 'light' | 'dark' {
  const theme = useTestMode((state) => state.theme)
  const [systemDarkState, setSystemDarkState] = useState(systemDark)

  useEffect(() => {
    const query = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (event: MediaQueryListEvent) => setSystemDarkState(event.matches)
    setSystemDarkState(query.matches)
    query.addEventListener('change', onChange)
    return () => query.removeEventListener('change', onChange)
  }, [])

  return theme === 'dark' || (theme === 'system' && systemDarkState) ? 'dark' : 'light'
}

export function useThemeSync(): void {
  const theme = useTestMode((state) => state.theme)
  const resolved = useResolvedTheme()

  useEffect(() => {
    const root = document.documentElement
    root.classList.toggle('dark', resolved === 'dark')
    root.style.colorScheme = resolved
    const meta = document.querySelector('meta[name="theme-color"]')
    if (meta) {
      const background = getComputedStyle(root).getPropertyValue('--bg-primary').trim()
      if (background) meta.setAttribute('content', background)
    }
    const manifest = document.getElementById('pwa-manifest')
    if (manifest) {
      manifest.setAttribute('href', resolved === 'dark' ? '/manifest-dark.webmanifest' : '/manifest.webmanifest')
    }
    const statusBar = document.querySelector('meta[name="apple-mobile-web-app-status-bar-style"]')
    if (statusBar) {
      statusBar.setAttribute('content', resolved === 'dark' ? 'black-translucent' : 'default')
    }
    syncAppleLaunchScreen(theme, resolved)
  }, [resolved, theme])
}

export function formatThemeMode(theme: ThemeMode): string {
  if (theme === 'light') return '浅色'
  if (theme === 'dark') return '深色'
  return '跟随系统'
}
