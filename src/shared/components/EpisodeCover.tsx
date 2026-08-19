interface EpisodeCoverProps {
  label: string
  size?: 'sm' | 'md'
  className?: string
}

export function EpisodeCover({ label, size = 'md', className = '' }: EpisodeCoverProps) {
  const dimension = size === 'md' ? 'h-11 w-11' : 'h-9 w-9'
  return (
    <div
      aria-hidden
      className={`flex shrink-0 items-center justify-center rounded-md bg-subtle ${dimension} ${className}`}
    >
      <span className="text-title-2 text-ink-secondary">{label}</span>
    </div>
  )
}
