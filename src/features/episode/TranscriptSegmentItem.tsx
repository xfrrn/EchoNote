import type { TranscriptSegment } from '../../shared/types'
import { getSpeaker, speakerTone } from '../../shared/mock/episodes'

interface TranscriptSegmentItemProps {
  segment: TranscriptSegment
  /** 是否与上一段为不同说话人（决定是否渲染署名行） */
  showSpeaker: boolean
}

export function TranscriptSegmentItem({ segment, showSpeaker }: TranscriptSegmentItemProps) {
  const speaker = getSpeaker(segment.speakerId)
  return (
    <article className={showSpeaker ? 'px-4 pt-7' : 'px-4 pt-4'}>
      {showSpeaker ? (
        <header className="flex items-baseline gap-2">
          <span
            className="h-2 w-2 shrink-0 translate-y-[-1px] rounded-full"
            style={{ backgroundColor: speakerTone(segment.speakerId) }}
            aria-hidden
          />
          <h3 className="text-subheadline font-semibold text-ink">{speaker.name}</h3>
          <time className="text-caption tabular-nums text-ink-tertiary">{segment.timestamp}</time>
        </header>
      ) : null}
      <p className={`font-serif text-body-serif text-ink ${showSpeaker ? 'mt-2.5' : ''}`}>
        {segment.text}
      </p>
    </article>
  )
}
