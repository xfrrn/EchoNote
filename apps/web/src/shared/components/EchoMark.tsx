interface EchoMarkProps {
  size?: number
  className?: string
  /** 是否让最外圈产生「回声」扩散动画（用于收音/等待等场景） */
  animated?: boolean
}

/**
 * EchoNote 品牌母题：一圈圈向外扩散、逐渐淡出的涟漪。
 * 声音落下，文字浮现 —— 回声被接住的那一刻。
 */
export function EchoMark({ size = 24, className = '', animated = false }: EchoMarkProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      className={className}
      aria-hidden
    >
      <circle cx="12" cy="12" r="2.1" fill="currentColor" />
      <circle cx="12" cy="12" r="5.4" stroke="currentColor" strokeWidth="1.7" opacity="0.7" />
      {animated ? (
        <circle
          cx="12"
          cy="12"
          r="8.6"
          stroke="currentColor"
          strokeWidth="1.5"
          className="origin-center animate-echo-ripple"
          style={{ transformBox: 'fill-box' }}
        />
      ) : (
        <circle cx="12" cy="12" r="8.6" stroke="currentColor" strokeWidth="1.5" opacity="0.34" />
      )}
    </svg>
  )
}
