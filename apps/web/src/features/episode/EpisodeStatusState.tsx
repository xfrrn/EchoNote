import type { Episode } from '../../shared/types'

interface EpisodeStatusStateProps {
  episode: Episode
  kind: 'transcript' | 'ai'
}

export function EpisodeStatusState({ episode, kind }: EpisodeStatusStateProps) {
  const isTranscript = kind === 'transcript'

  const content = {
    transcribing: {
      title: isTranscript ? 'Transcript 正在生成' : 'AI 整理尚未生成',
      detail: isTranscript
        ? '云端正在区分 Speaker 并整理访谈文本。Demo 未连接真实转录服务，此处仅用于验证等待状态。'
        : 'Transcript 完成后，AI 会在这里给出一句话总结、核心观点与结合笔记的梳理。'
    },
    waiting: {
      title: isTranscript ? '等待转录' : '等待转录后整理',
      detail: isTranscript
        ? '节目已进入转录队列。Demo 中不会真的开始转录。'
        : '节目完成转录后，这里会出现 AI 整理内容。'
    },
    failed: {
      title: isTranscript ? '转录失败' : 'AI 整理暂不可用',
      detail: isTranscript
        ? 'Demo 模拟了失败状态，用于验证错误信息是否安静、明确、不打断阅读。'
        : '节目转录失败，因此暂时无法生成 AI 整理。'
    },
    transcribed: {
      title: isTranscript ? 'Transcript' : 'AI 整理',
      detail: ''
    }
  }[episode.status]

  return (
    <div className="px-4 py-10">
      <p className="text-headline text-ink">{content.title}</p>
      <p className="mt-2 max-w-sm text-body text-ink-secondary">{content.detail}</p>
    </div>
  )
}
