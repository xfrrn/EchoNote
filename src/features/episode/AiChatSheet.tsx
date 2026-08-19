import { useState } from 'react'
import { ArrowUp } from 'lucide-react'
import { Sheet } from '../../shared/components/Sheet'

interface AiChatSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  episodeTitle: string
}

const mockAnswer =
  '如果从这期节目的视角看，FDE 更像“企业 AI 的现场翻译”：把一线业务语言翻译成模型能执行的步骤，再把执行结果翻译回业务语言。它的价值不在于模型层，而在于对流程、异常和人的理解。'

export function AiChatSheet({ open, onOpenChange, episodeTitle }: AiChatSheetProps) {
  const [query, setQuery] = useState('')
  const [asked, setAsked] = useState('')

  const ask = () => {
    const value = query.trim()
    if (!value) return
    setAsked(value)
    setQuery('')
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="问这期节目" description={`基于 ${episodeTitle} 的模拟问答`}>
      <div className="px-4">
        {asked ? (
          <div>
            <div className="pt-2">
              <p className="text-caption-medium text-ink-secondary">你的问题</p>
              <p className="mt-1.5 text-body text-ink">{asked}</p>
            </div>
            <div className="mt-6">
              <p className="text-caption-medium text-ink-secondary">AI 回答 · Demo</p>
              <p className="mt-1.5 text-body text-ink">{mockAnswer}</p>
            </div>
          </div>
        ) : (
          <p className="pt-2 text-body text-ink-secondary">
            基于本期 Transcript 和你的笔记回答。Demo 使用模拟答案，不会连接真实 AI 服务。
          </p>
        )}

        <div className="mt-6 flex items-end gap-2 border-t border-hairline pt-3">
          <input
            type="text"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') ask()
            }}
            placeholder="问这期节目…"
            className="h-11 min-w-0 flex-1 rounded-md bg-subtle px-3 text-body text-ink placeholder:text-ink-tertiary"
          />
          <button
            type="button"
            aria-label="发送"
            onClick={ask}
            disabled={!query.trim()}
            className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-accent text-on-accent transition-opacity duration-fast ease-ios disabled:opacity-40"
          >
            <ArrowUp size={19} strokeWidth={2.2} aria-hidden />
          </button>
        </div>
      </div>
    </Sheet>
  )
}
