import { useId } from 'react'

interface CoverTheme {
  from: string
  to: string
  glyph: string
  wave: string
}

/**
 * 节目封面母题：暖调双色素场 + 衬线大字 + 一条声波 + 纸纹。
 * 四档常驻节目手工调色；未知节目（如导入）按标题哈希从同色系中取用，
 * 保证整个资料库视觉同源、不花哨。
 */
const CURATED: Record<string, CoverTheme> = {
  硅谷101: { from: '#3d4b5c', to: '#232d38', glyph: '#f2e9da', wave: '#e07a52' },
  '原点 The Origin': { from: '#b26a3f', to: '#7d4426', glyph: '#f8ecdd', wave: '#2e1c10' },
  声动早咖啡: { from: '#8a6a48', to: '#584026', glyph: '#f5ead6', wave: '#e8b04b' },
  '晚点聊 LateTalk': { from: '#3d5a4a', to: '#243329', glyph: '#e9efdf', wave: '#d98a52' }
}

const FALLBACK_PALETTE: CoverTheme[] = [
  { from: '#5a4a6a', to: '#3c2f49', glyph: '#efe6f2', wave: '#e07a52' },
  { from: '#6a5a3a', to: '#463c26', glyph: '#f2ecda', wave: '#d98a52' },
  { from: '#3a5a66', to: '#26404a', glyph: '#dfeef2', wave: '#e8b04b' },
  { from: '#66463e', to: '#462e28', glyph: '#f2e4de', wave: '#e07a52' }
]

function hashTitle(title: string): number {
  let h = 0
  for (let i = 0; i < title.length; i++) {
    h = (h * 31 + title.charCodeAt(i)) >>> 0
  }
  return h
}

export function coverThemeFor(showTitle: string): CoverTheme {
  if (CURATED[showTitle]) return CURATED[showTitle]
  return FALLBACK_PALETTE[hashTitle(showTitle) % FALLBACK_PALETTE.length]
}

interface ShowCoverProps {
  showTitle: string
  glyph: string
  size?: 'sm' | 'md' | 'lg' | 'xl'
  className?: string
}

const DIMENSIONS: Record<NonNullable<ShowCoverProps['size']>, string> = {
  sm: 'h-10 w-10 rounded-md',
  md: 'h-12 w-12 rounded-lg',
  lg: 'h-16 w-16 rounded-lg',
  xl: 'h-24 w-24 rounded-xl'
}

export function ShowCover({ showTitle, glyph, size = 'md', className = '' }: ShowCoverProps) {
  const theme = coverThemeFor(showTitle)
  const uid = useId().replace(/[^a-zA-Z0-9]/g, '')
  const gradientId = `cover-g-${uid}`
  const grainId = `cover-n-${uid}`

  return (
    <div
      aria-hidden
      className={`relative shrink-0 overflow-hidden ${DIMENSIONS[size]} ${className}`}
      style={{ background: theme.to }}
    >
      <svg viewBox="0 0 96 96" className="block h-full w-full" preserveAspectRatio="xMidYMid slice">
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stopColor={theme.from} />
            <stop offset="100%" stopColor={theme.to} />
          </linearGradient>
          <filter id={grainId}>
            <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" stitchTiles="stitch" />
            <feColorMatrix type="saturate" values="0" />
          </filter>
        </defs>

        <rect width="96" height="96" fill={`url(#${gradientId})`} />

        {/* 回声涟漪（右上，极淡） */}
        <g stroke={theme.wave} fill="none" opacity="0.5">
          <circle cx="74" cy="22" r="6" strokeWidth="1.4" opacity="0.8" />
          <circle cx="74" cy="22" r="11" strokeWidth="1.1" opacity="0.45" />
        </g>

        {/* 声波线（中下） */}
        <path
          d="M10 66 Q 20 58, 30 66 T 50 66 T 70 66 T 90 66"
          stroke={theme.wave}
          strokeWidth="1.6"
          fill="none"
          strokeLinecap="round"
          opacity="0.85"
        />

        {/* 大字 glyph（衬线） */}
        <text
          x="14"
          y="46"
          fill={theme.glyph}
          fontSize="40"
          fontWeight="600"
          fontFamily='"Songti SC","Noto Serif CJK SC",ui-serif,Georgia,serif'
        >
          {glyph}
        </text>

        {/* 纸纹 */}
        <rect width="96" height="96" filter={`url(#${grainId})`} opacity="0.06" />
      </svg>
    </div>
  )
}
