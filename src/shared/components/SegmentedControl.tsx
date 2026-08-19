import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

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

interface Thumb {
  left: number
  width: number
}

/**
 * Apple 风格分段控件:选中的白色滑块在选项之间平滑滑动,
 * 而不是瞬间跳变。滑动距离/宽度根据实际 DOM 测量,适配任意数量与文案。
 */
export function SegmentedControl<T extends string | number>({
  options,
  value,
  onChange,
  className = '',
  ariaLabel
}: SegmentedControlProps<T>) {
  const trackRef = useRef<HTMLDivElement>(null)
  const buttonRefs = useRef<(HTMLButtonElement | null)[]>([])
  const [thumb, setThumb] = useState<Thumb | null>(null)

  const selectedIndex = Math.max(
    0,
    options.findIndex((option) => option.value === value)
  )

  useLayoutEffect(() => {
    const measure = () => {
      const track = trackRef.current
      const button = buttonRefs.current[selectedIndex]
      if (!track || !button) return
      const trackRect = track.getBoundingClientRect()
      const buttonRect = button.getBoundingClientRect()
      setThumb({ left: buttonRect.left - trackRect.left, width: buttonRect.width })
    }
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [selectedIndex, options.length])

  return (
    <div
      ref={trackRef}
      role="tablist"
      aria-label={ariaLabel}
      className={`relative inline-flex min-h-11 w-full items-center gap-1 rounded-lg bg-subtle p-1 ${className}`}
    >
      {/* 滑动选中块 */}
      <span
        aria-hidden
        className="absolute left-0 top-1 bottom-1 rounded-md bg-surface shadow-control transition-[transform,width] duration-normal ease-ios"
        style={
          thumb
            ? { transform: `translateX(${thumb.left}px)`, width: `${thumb.width}px`, opacity: 1 }
            : { opacity: 0 }
        }
      />
      {options.map((option, index) => {
        const selected = option.value === value
        return (
          <button
            key={option.value}
            ref={(node) => {
              buttonRefs.current[index] = node
            }}
            role="tab"
            type="button"
            aria-selected={selected}
            onClick={() => onChange(option.value)}
            className={`relative z-10 min-h-9 flex-1 rounded-md px-2 text-subheadline transition-colors duration-fast ease-ios ${
              selected ? 'font-medium text-ink' : 'text-ink-secondary active:text-ink'
            }`}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
