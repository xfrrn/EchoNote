import { forwardRef, type ComponentProps } from 'react'
import { Link } from 'react-router-dom'

type RowLinkProps = ComponentProps<typeof Link>

/**
 * 列表行链接:iOS 式轻触反馈。
 * 按下时行内容轻微回弹缩放(0.97)+ 高亮,松手弹回,克制而不夸张。
 * 供「可点的列表行」复用,保持全 App 触感一致。
 */
export const RowLink = forwardRef<HTMLAnchorElement, RowLinkProps>(function RowLink(
  { className = '', ...props },
  ref
) {
  return (
    <Link
      ref={ref}
      {...props}
      className={`transition-all duration-fast ease-ios active:scale-[0.97] active:bg-subtle ${className}`}
    />
  )
})
