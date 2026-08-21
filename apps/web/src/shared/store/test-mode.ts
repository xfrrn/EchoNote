import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { EpisodeStatus } from '../types'

export type TitleLength = 'short' | 'long'
export type TranscriptLength = 'small' | 'large'
export type SpeakerCount = 1 | 2 | 4
export type NotesDensity = 'none' | 'few' | 'many'

export interface TestModeState {
  titleLength: TitleLength
  transcriptLength: TranscriptLength
  speakerCount: SpeakerCount
  notesDensity: NotesDensity
  primaryStatus: EpisodeStatus
  setTitleLength: (length: TitleLength) => void
  setTranscriptLength: (length: TranscriptLength) => void
  setSpeakerCount: (count: SpeakerCount) => void
  setNotesDensity: (density: NotesDensity) => void
  setPrimaryStatus: (status: EpisodeStatus) => void
}

export const useTestMode = create<TestModeState>()(
  persist(
    (set) => ({
      titleLength: 'long',
      transcriptLength: 'large',
      speakerCount: 2,
      notesDensity: 'few',
      primaryStatus: 'transcribed',
      setTitleLength: (titleLength) => set({ titleLength }),
      setTranscriptLength: (transcriptLength) => set({ transcriptLength }),
      setSpeakerCount: (speakerCount) => set({ speakerCount }),
      setNotesDensity: (notesDensity) => set({ notesDensity }),
      setPrimaryStatus: (primaryStatus) => set({ primaryStatus })
    }),
    {
      name: 'echonote.test-mode'
    }
  )
)
