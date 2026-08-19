import type { TranscriptSegment } from '../../shared/types'
import { getSpeaker } from '../../shared/mock/episodes'

export function TranscriptSegmentItem({ segment }: { segment: TranscriptSegment }) {
  const speaker = getSpeaker(segment.speakerId)
  return (
    <article className="px-4 py-5">
      <header className="flex items-baseline justify-between gap-4">
        <h3 className="text-subheadline font-semibold text-ink">{speaker.name}</h3>
        <time className="shrink-0 text-caption text-ink-tertiary">{segment.timestamp}</time>
      </header>
      <p className="mt-2 text-body text-ink">{segment.text}</p>
    </article>
  )
}
