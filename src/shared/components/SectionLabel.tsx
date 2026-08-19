import type { PropsWithChildren } from 'react'

export function SectionLabel({ children, className = '' }: PropsWithChildren<{ className?: string }>) {
  return (
    <div className={`px-4 pt-6 pb-2 text-caption-medium text-ink-secondary ${className}`}>
      {children}
    </div>
  )
}
