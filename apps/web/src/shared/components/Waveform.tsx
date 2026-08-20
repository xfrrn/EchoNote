import { useMemo } from 'react'

interface WaveformProps {
  /** 竖条数量 */
  bars?: number
  /** 让波形看起来自然、但保持确定性的种子 */
  seed?: string
  className?: string
  /** 是否轻微起伏（用于「正在收音/转录」等状态） */
  animated?: boolean
}

function seedToNumber(seed: string): number {
  let h = 2166136261
  for (let i = 0; i < seed.length; i++) {
    h ^= seed.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return Math.abs(h)
}

/**
 * 声波纹理：EchoNote 的内容装饰母题。
 * 确定性生成，不使用随机数，保证每次渲染一致。
 */
export function Waveform({ bars = 28, seed = 'echonote', className = '', animated = false }: WaveformProps) {
  const heights = useMemo(() => {
    let h = seedToNumber(seed)
    const out: number[] = []
    for (let i = 0; i < bars; i++) {
      h = (h * 9301 + 49297) % 233280
      const rand = h / 233280
      const envelope = Math.sin((i / Math.max(1, bars - 1)) * Math.PI) // 中间高、两端低
      out.push(0.22 + 0.78 * (0.45 * envelope + 0.55 * rand))
    }
    return out
  }, [bars, seed])

  return (
    <div className={`flex items-center justify-center gap-[2.5px] ${className}`} aria-hidden>
      {heights.map((ratio, i) => (
        <span
          key={i}
          className={`w-[2px] shrink-0 rounded-full bg-current ${animated ? 'animate-wave-pulse' : ''}`}
          style={{
            height: `${Math.round(ratio * 100)}%`,
            transformOrigin: 'center',
            animationDelay: animated ? `${(i % 7) * 0.12}s` : undefined
          }}
        />
      ))}
    </div>
  )
}
