import type { Episode } from '../../shared/types'
import { useTranscript } from '../../shared/mock/episodes'
import { TranscriptSegmentItem } from './TranscriptSegmentItem'
import { EpisodeStatusState } from './EpisodeStatusState'

export function TranscriptTab({ episode }: { episode: Episode }) {
  const segments = useTranscript(episode.id)

  if (!episode.transcriptAvailable) {
    return <EpisodeStatusState episode={episode} kind="transcript" />
  }

  return (
    <div>
      <p className="px-4 pt-4 text-caption text-ink-tertiary">
        自动转录 · Speaker 已区分 · 本页为 Demo 模拟文本
      </p>
      <div className="mt-2 divide-y divide-hairline">
        {segments.map((segment) => (
          <TranscriptSegmentItem key={segment.id} segment={segment} />
        ))}
      </div>
      <p className="px-4 py-6 text-center text-caption text-ink-tertiary">
        已显示全部 Transcript · 继续阅读愉快
      </p>
    </div>
  )
}
