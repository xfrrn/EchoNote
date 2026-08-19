import type { ReactElement } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { SquareStack, Plus, MagnifyingGlass, PersonCropCircle } from './TabIcons'

interface TabItem {
  to: string
  label: string
  /** 渲染线性(SF 风格)图标 */
  Icon: (props: { filled: boolean; size?: number }) => ReactElement
  match?: string[]
  prominent?: boolean
}

const items: TabItem[] = [
  { to: '/library', label: '节目', Icon: SquareStack, match: ['/library', '/episode'] },
  { to: '/capture', label: '记录', Icon: Plus, prominent: true },
  { to: '/search', label: '搜索', Icon: MagnifyingGlass, match: ['/search'] },
  { to: '/mine', label: '我的', Icon: PersonCropCircle, match: ['/mine'] }
]

export function BottomNav() {
  const location = useLocation()
  const path = location.pathname

  return (
    <nav
      aria-label="底部导航"
      className="safe-sides fixed inset-x-0 bottom-0 z-40 mx-auto w-full max-w-app glass-surface safe-bottom border-t border-hairline"
    >
      <div className="grid h-[50px] grid-cols-4">
        {items.map((item) => {
          const active = item.match?.some((prefix) => path === prefix || path.startsWith(`${prefix}/`)) ?? false
          return (
            <Link
              key={item.to}
              to={item.to}
              aria-label={item.label}
              aria-current={active ? 'page' : undefined}
              className="flex min-h-11 flex-col items-center justify-center gap-[3px] pt-0.5 transition-colors duration-fast ease-ios"
            >
              {item.prominent ? (
                <>
                  <span className="flex h-[26px] items-center justify-center text-ink-secondary">
                    <item.Icon filled={false} size={24} />
                  </span>
                  <span className="text-[10px] leading-none text-ink-secondary">{item.label}</span>
                </>
              ) : (
                <>
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
                </>
              )}
            </Link>
          )
        })}
      </div>
    </nav>
  )
}
