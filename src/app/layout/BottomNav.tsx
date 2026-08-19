import type { ReactElement } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { SquareStack, SquarePen, MagnifyingGlass, PersonCropCircle } from './TabIcons'

interface TabItem {
  to: string
  label: string
  /** 渲染线性(SF 风格)图标 */
  Icon: (props: { filled: boolean; size?: number }) => ReactElement
  match: string[]
}

const items: TabItem[] = [
  { to: '/library', label: '节目', Icon: SquareStack, match: ['/library', '/episode'] },
  { to: '/capture', label: '记录', Icon: SquarePen, match: ['/capture'] },
  { to: '/search', label: '搜索', Icon: MagnifyingGlass, match: ['/search'] },
  { to: '/mine', label: '我的', Icon: PersonCropCircle, match: ['/mine'] }
]

export function BottomNav() {
  const location = useLocation()
  const path = location.pathname

  return (
    <nav aria-label="底部导航" className="safe-sides pointer-events-none fixed inset-x-0 bottom-0 z-40">
      <div className="mx-auto w-full max-w-app px-4 pb-[max(env(safe-area-inset-bottom),10px)]">
        <div className="glass-nav pointer-events-auto grid h-[58px] grid-cols-4 rounded-full">
          {items.map((item) => {
            const active = item.match?.some((prefix) => path === prefix || path.startsWith(`${prefix}/`)) ?? false
            return (
              <Link
                key={item.to}
                to={item.to}
                aria-label={item.label}
                aria-current={active ? 'page' : undefined}
                className="flex min-h-11 flex-col items-center justify-center gap-[3px] transition-colors duration-fast ease-ios"
              >
                <span className={active ? 'text-accent' : 'text-ink-secondary'}>
                  <item.Icon filled={active} size={24} />
                </span>
                <span
                  className={`text-[10px] leading-none ${
                    active ? 'font-medium text-accent' : 'text-ink-secondary'
                  }`}
                >
                  {item.label}
                </span>
              </Link>
            )
          })}
        </div>
      </div>
    </nav>
  )
}
