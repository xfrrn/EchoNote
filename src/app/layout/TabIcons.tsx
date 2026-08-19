/**
 * 底部导航图标:手绘 SF Symbols 风格。
 * 统一 24 视窗、1.8 描边、圆角端点;线性为默认,选中项用 filled 变体,
 * 颜色由父级 currentColor 控制(选中=系统蓝,未选中=中性灰)。
 * 单色、简洁、有区分度,贴合 Apple 原生气质。
 */

interface TabIconProps {
  filled?: boolean
  size?: number
}

const stroke = 1.8

/** 节目 —— 堆叠方块(参考 square.stack) */
export function SquareStack({ filled = false, size = 24 }: TabIconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden>
      {/* 底层卡片(后) */}
      <rect
        x="5.5"
        y="3.5"
        width="13"
        height="3.4"
        rx="1.7"
        stroke="currentColor"
        strokeWidth={stroke}
        opacity={filled ? 0 : 0.55}
        fill={filled ? 'currentColor' : 'none'}
        fillOpacity={filled ? 0.45 : 0}
      />
      {/* 中层卡片 */}
      <rect
        x="4"
        y="8.2"
        width="16"
        height="3.4"
        rx="1.7"
        stroke="currentColor"
        strokeWidth={stroke}
        opacity={filled ? 0 : 0.8}
        fill={filled ? 'currentColor' : 'none'}
        fillOpacity={filled ? 0.7 : 0}
      />
      {/* 顶层卡片(前) */}
      <rect
        x="2.5"
        y="13"
        width="19"
        height="7.5"
        rx="2.2"
        stroke="currentColor"
        strokeWidth={stroke}
        fill={filled ? 'currentColor' : 'none'}
      />
    </svg>
  )
}

/** 记录 —— 方形便签 + 笔(参考 square.and.pencil) */
export function SquarePen({ filled = false, size = 24 }: TabIconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden>
      <rect
        x="3.5"
        y="3.5"
        width="17"
        height="17"
        rx="4"
        stroke="currentColor"
        strokeWidth={stroke}
        fill={filled ? 'currentColor' : 'none'}
        fillOpacity={filled ? 0.16 : 0}
      />
      <path
        d="M9.6 14.4l.7-2.7 4.4-4.4a1.5 1.5 0 0 1 2.1 2.1l-4.4 4.4-2.8.7z"
        stroke="currentColor"
        strokeWidth={stroke}
        strokeLinejoin="round"
        fill={filled ? 'currentColor' : 'none'}
      />
    </svg>
  )
}

/** 搜索 —— 放大镜(参考 magnifyingglass) */
export function MagnifyingGlass({ filled = false, size = 24 }: TabIconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle
        cx="10.5"
        cy="10.5"
        r="6"
        stroke="currentColor"
        strokeWidth={stroke}
        fill={filled ? 'currentColor' : 'none'}
        fillOpacity={filled ? 0.18 : 0}
      />
      <path d="M15 15l4.5 4.5" stroke="currentColor" strokeWidth={stroke} strokeLinecap="round" />
    </svg>
  )
}

/** 我的 —— 人像圆(参考 person.crop.circle) */
export function PersonCropCircle({ filled = false, size = 24 }: TabIconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle
        cx="12"
        cy="12"
        r="8.5"
        stroke="currentColor"
        strokeWidth={stroke}
        fill={filled ? 'currentColor' : 'none'}
        fillOpacity={filled ? 0.16 : 0}
      />
      {/* 头 */}
      <circle
        cx="12"
        cy="9.2"
        r="2.7"
        stroke="currentColor"
        strokeWidth={stroke}
        fill={filled ? 'currentColor' : 'none'}
      />
      {/* 肩(弧线,不下沉到底) */}
      <path
        d="M7 16.6c.9-2.5 2.7-3.9 5-3.9s4.1 1.4 5 3.9"
        stroke="currentColor"
        strokeWidth={stroke}
        strokeLinecap="round"
        fill="none"
      />
    </svg>
  )
}
