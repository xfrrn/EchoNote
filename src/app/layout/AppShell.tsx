import { useEffect } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { BottomNav } from './BottomNav'

export function AppShell() {
  const location = useLocation()

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [location.pathname])

  return (
    <div className="safe-sides app-viewport w-full bg-canvas text-ink">
      <div className="mx-auto flex app-viewport w-full max-w-app flex-col bg-canvas">
        <div className="safe-top" />
        <main className="flex-1 pb-28">
          <Outlet />
        </main>
        <BottomNav />
      </div>
    </div>
  )
}
