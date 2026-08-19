import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { EpisodeStatus } from '../types'

export type ThemeMode = 'light' | 'dark' | 'system'
export type TitleLength = 'short' | 'long'
export type TranscriptLength = 'small' | 'large'
export type SpeakerCount = 1 | 2 | 4
export type NotesDensity = 'none' | 'few' | 'many'

export interface TestModeState {
  theme: ThemeMode
  titleLength: TitleLength
  transcriptLength: TranscriptLength
  speakerCount: SpeakerCount
  notesDensity: NotesDensity
  primaryStatus: EpisodeStatus
  setTheme: (theme: ThemeMode) => void
  setTitleLength: (length: TitleLength) => void
  setTranscriptLength: (length: TranscriptLength) => void
  setSpeakerCount: (count: SpeakerCount) => void
  setNotesDensity: (density: NotesDensity) => void
  setPrimaryStatus: (status: EpisodeStatus) => void
}

export const useTestMode = create<TestModeState>()(
  persist(
    (set) => ({
      theme: 'system',
      titleLength: 'long',
      transcriptLength: 'large',
      speakerCount: 2,
      notesDensity: 'few',
      primaryStatus: 'transcribed',
      setTheme: (theme) => set({ theme }),
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
