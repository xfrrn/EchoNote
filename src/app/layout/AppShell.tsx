import { useEffect } from 'react'
import { Outlet, useLocation, useNavigationType } from 'react-router-dom'
import { BottomNav } from './BottomNav'

const TAB_ORDER = ['/library', '/search', '/mine']

function isTabRoute(pathname: string): boolean {
  return TAB_ORDER.includes(pathname)
}

/**
 * 页面切换动画(贴合 iOS 的克制动效):
 * - Tab 之间切换:仅淡入
 * - 进入下一级(列表 → 详情):从右侧轻推入(前进)
 * - 返回上一级:从左侧轻推入(后退),形成「上一页被接回来」的视差回拉
 * 全程尊重 prefers-reduced-motion(全局 CSS 已兜底)。
 */
export function AppShell() {
  const location = useLocation()
  const navigationType = useNavigationType()

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [location.pathname])

  let animationClass = 'animate-page-fade'
  if (isTabRoute(location.pathname)) {
    // Tab 之间:淡入;返回 Tab 时略带从左侧的视差回拉,像上一页被「接回来」
    animationClass =
      navigationType === 'POP' ? 'animate-page-back' : 'animate-page-fade'
  } else if (navigationType === 'POP') {
    animationClass = 'animate-page-back'
  } else {
    animationClass = 'animate-page-forward'
  }

  return (
    <div className="safe-sides app-viewport w-full bg-canvas text-ink">
      <div className="mx-auto flex app-viewport w-full max-w-app flex-col bg-canvas">
        <div className="safe-top" />
        <main className="flex-1 pb-32">
          <div key={location.pathname} className={animationClass}>
            <Outlet />
          </div>
        </main>
        <BottomNav />
      </div>
    </div>
  )
}
