import type { PropsWithChildren } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useThemeSync } from '../../shared/hooks/useResolvedTheme'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      staleTime: Infinity
    }
  }
})

function ThemeSync() {
  useThemeSync()
  return null
}

function ViewportChrome() {
  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-x-0 top-0 z-30 h-[env(safe-area-inset-top)] bg-canvas"
    />
  )
}

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeSync />
      <ViewportChrome />
      {children}
    </QueryClientProvider>
  )
}
