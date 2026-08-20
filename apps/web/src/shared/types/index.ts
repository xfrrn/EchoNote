export type EpisodeStatus = 'transcribed' | 'transcribing' | 'waiting' | 'failed'

export interface Note {
  id: string
  createdAt: string
  text: string
}

export interface Speaker {
  id: 'host' | 'peng' | 'guest' | 'audience'
  name: string
  shortName?: string
}

export interface TranscriptSegment {
  id: string
  speakerId: Speaker['id']
  timestamp: string
  text: string
}

export interface AiSummary {
  oneLiner: string
  corePoints: string[]
  viewpoints: { speaker: string; point: string }[]
  worthReviewing: { timestamp: string; quote: string; reason: string }[]
  noteConnections: { note: string; insight: string }[]
}

export interface Episode {
  id: string
  showTitle: string
  episodeTitle: string
  episodeTitleLong: string
  durationMin: number
  status: EpisodeStatus
  recordedLabel: string
  notes: Note[]
  coverLabel: string
  transcriptAvailable: boolean
  aiAvailable: boolean
}

export interface SearchResultItem {
  kind: 'note' | 'transcript' | 'ai'
  episodeId: string
  episodeTitle: string
  showTitle: string
  snippet: string
  meta: string
}
