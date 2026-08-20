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
    <div className="pb-6">
      <p className="px-4 pt-4 text-caption text-ink-tertiary">
        自动转录 · 已区分 {new Set(segments.map((s) => s.speakerId)).size} 位说话人
      </p>
      <div className="mt-1">
        {segments.map((segment, index) => (
          <TranscriptSegmentItem
            key={segment.id}
            segment={segment}
            showSpeaker={index === 0 || segments[index - 1].speakerId !== segment.speakerId}
          />
        ))}
      </div>
      <p className="px-4 pt-8 text-center text-caption text-ink-tertiary">
        —— 本期 Transcript 完 ——
      </p>
    </div>
  )
}
