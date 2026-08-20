import { Fragment } from 'react'

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function HighlightedText({ text, query }: { text: string; query: string }) {
  const escaped = escapeRegExp(query.trim())
  if (!escaped) return <>{text}</>

  const parts = text.split(new RegExp(`(${escaped})`, 'ig'))
  return (
    <>
      {parts.map((part, index) => {
        const isMatch = part.toLocaleLowerCase('zh-CN') === query.trim().toLocaleLowerCase('zh-CN')
        return isMatch ? (
          <mark key={`${part}-${index}`} className="search-highlight">
            {part}
          </mark>
        ) : (
          <Fragment key={`${part}-${index}`}>{part}</Fragment>
        )
      })}
    </>
  )
}
