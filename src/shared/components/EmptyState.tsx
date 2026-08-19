interface EmptyStateProps {
  title: string
  detail?: string
}

export function EmptyState({ title, detail }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-start px-4 py-10">
      <p className="text-headline text-ink">{title}</p>
      {detail ? <p className="mt-2 max-w-sm text-body text-ink-secondary">{detail}</p> : null}
    </div>
  )
}
