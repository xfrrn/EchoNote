import type { AiSummary, Episode } from '../types'

export interface ShareOptions {
  notes: boolean
  oneLiner: boolean
  corePoints: boolean
  worthReviewing: boolean
  transcript: boolean
}

export const defaultShareOptions: ShareOptions = {
  notes: true,
  oneLiner: true,
  corePoints: true,
  worthReviewing: true,
  transcript: false
}

/**
 * 把一期节目的整理结果排版成适合粘贴进 Apple 备忘录的纯文本。
 * Apple Notes 会把 text 当作带换行的便签内容，因此用轻量标记而非富文本。
 */
export function composeEpisodeMarkdown(
  episode: Episode,
  summary: AiSummary,
  options: ShareOptions,
  transcriptExcerpt: string[] = []
): string {
  const lines: string[] = []

  lines.push(episode.episodeTitleLong || episode.episodeTitle)
  lines.push(`${episode.showTitle} · ${episode.durationMin} 分钟 · ${episode.recordedLabel}`)
  lines.push('')

  if (options.oneLiner) {
    lines.push('【一句话总结】')
    lines.push(summary.oneLiner)
    lines.push('')
  }

  if (options.corePoints && summary.corePoints.length > 0) {
    lines.push('【核心观点】')
    summary.corePoints.forEach((point, index) => {
      lines.push(`${index + 1}. ${point}`)
    })
    lines.push('')
  }

  if (options.worthReviewing && summary.worthReviewing.length > 0) {
    lines.push('【值得回顾】')
    summary.worthReviewing.forEach((item) => {
      lines.push(`${item.timestamp} 「${item.quote}」`)
    })
    lines.push('')
  }

  if (options.notes && episode.notes.length > 0) {
    lines.push('【我的笔记】')
    episode.notes.forEach((note) => {
      lines.push(`${note.createdAt}  ${note.text}`)
    })
    lines.push('')
  }

  if (options.transcript && transcriptExcerpt.length > 0) {
    lines.push('【Transcript 节选】')
    transcriptExcerpt.forEach((line) => lines.push(line))
    lines.push('')
  }

  lines.push('—— 用 EchoNote 整理')

  return lines.join('\n').replace(/\n{3,}/g, '\n\n').trim()
}
