import type { TranscriptSegment, TranscriptSpeaker } from '../../shared/api/client'

function time(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

export function TranscriptSegmentItem({ segment, speaker, showSpeaker }: { segment: TranscriptSegment; speaker?: TranscriptSpeaker; showSpeaker: boolean }) {
  return (
    <article id={`segment-${segment.id}`} className={showSpeaker ? 'px-4 pt-7' : 'px-4 pt-4'}>
      {showSpeaker ? <header className="flex items-baseline gap-2"><span className="h-2 w-2 shrink-0 translate-y-[-1px] rounded-full bg-accent" aria-hidden /><h3 className="text-subheadline font-semibold text-ink">{speaker?.display_name ?? '说话人'}</h3><time className="text-caption tabular-nums text-ink-tertiary">{time(segment.start_ms)}</time></header> : null}
      <p className={`font-serif text-body-serif text-ink ${showSpeaker ? 'mt-2.5' : ''}`}>{segment.text}</p>
    </article>
  )
}
