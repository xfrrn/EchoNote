import { Link, useLocation } from 'react-router-dom'
import { Library, Plus, Search, UserRound } from 'lucide-react'

const items = [
  { to: '/library', label: '节目', icon: Library, match: ['/library', '/episode'] },
  { to: '/capture', label: '记录', icon: Plus, prominent: true },
  { to: '/search', label: '搜索', icon: Search, match: ['/search'] },
  { to: '/mine', label: '我的', icon: UserRound, match: ['/mine'] }
]

export function BottomNav() {
  const location = useLocation()
  const path = location.pathname

  return (
    <nav
      aria-label="底部导航"
      className="safe-sides fixed inset-x-0 bottom-0 z-40 mx-auto w-full max-w-app glass-surface safe-bottom border-t border-hairline"
    >
      <div className="grid h-14 grid-cols-4">
        {items.map((item) => {
          const Icon = item.icon
          const active = item.match?.some((prefix) => path === prefix || path.startsWith(`${prefix}/`))
          const color = item.prominent || active ? 'text-ink' : 'text-ink-secondary'
          return (
            <Link
              key={item.to}
              to={item.to}
              aria-label={item.label}
              aria-current={active ? 'page' : undefined}
              className={`flex min-h-11 flex-col items-center justify-center gap-0.5 transition-colors duration-fast ease-ios ${color}`}
            >
              {item.prominent ? (
                <span className="flex h-7 w-7 items-center justify-center rounded-full bg-accent text-on-accent">
                  <Icon size={17} strokeWidth={2.2} aria-hidden />
                </span>
              ) : (
                <Icon size={21} strokeWidth={active ? 2 : 1.8} aria-hidden />
              )}
              <span className={`text-caption ${item.prominent || active ? 'font-medium' : ''}`}>
                {item.label}
              </span>
            </Link>
          )
        })}
      </div>
    </nav>
  )
}
