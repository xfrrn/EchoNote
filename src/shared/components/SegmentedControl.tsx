import type { ReactNode } from 'react'

export interface SegmentedOption<T extends string | number> {
  value: T
  label: ReactNode
}

interface SegmentedControlProps<T extends string | number> {
  options: SegmentedOption<T>[]
  value: T
  onChange: (value: T) => void
  className?: string
  ariaLabel?: string
}

export function SegmentedControl<T extends string | number>({
  options,
  value,
  onChange,
  className = '',
  ariaLabel
}: SegmentedControlProps<T>) {
  return (
    <div
      role="tablist"
      aria-label={ariaLabel}
      className={`inline-flex min-h-11 w-full items-center gap-1 rounded-lg bg-subtle p-1 ${className}`}
    >
      {options.map((option) => {
        const selected = option.value === value
        return (
          <button
            key={option.value}
            role="tab"
            type="button"
            aria-selected={selected}
            onClick={() => onChange(option.value)}
            className={`min-h-9 flex-1 rounded-md px-2 text-subheadline transition-all duration-fast ease-ios ${
              selected
                ? 'bg-surface font-medium text-ink shadow-control'
                : 'text-ink-secondary active:bg-subtle'
            }`}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
