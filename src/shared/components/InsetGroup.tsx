import type { PropsWithChildren } from 'react'

/**
 * Apple Settings 式分组列表(inset-grouped):
 * 圆角白色(深色为深灰)卡片,浮在浅灰/纯黑背景上,
 * 行与行之间用细分隔线,整组一个容器。
 */
export function InsetGroup({ children }: PropsWithChildren) {
  return (
    <div className="overflow-hidden rounded-md bg-surface">
      <div className="divide-y divide-hairline">{children}</div>
    </div>
  )
}
