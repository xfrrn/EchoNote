import type { ReactNode } from 'react'
import { EchoMark } from './EchoMark'

interface EmptyStateProps {
  title: string
  detail?: string
  icon?: ReactNode
}

export function EmptyState({ title, detail, icon }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-start px-4 py-12">
      <div className="text-accent">{icon ?? <EchoMark size={30} />}</div>
      <p className="mt-4 text-headline text-ink">{title}</p>
      {detail ? <p className="mt-2 max-w-sm text-body text-ink-secondary">{detail}</p> : null}
    </div>
  )
}
