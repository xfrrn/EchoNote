import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Note } from '../types'

interface CaptureState {
  episodeId: string
  draft: string
  extraNotes: Record<string, Note[]>
  setEpisodeId: (episodeId: string) => void
  setDraft: (draft: string) => void
  addNote: (episodeId: string, note: Note) => void
  clearDraft: () => void
}

export function currentTimeLabel(): string {
  const now = new Date()
  return `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
}

export const useCaptureStore = create<CaptureState>()(
  persist(
    (set) => ({
      episodeId: 'e1',
      draft: '',
      extraNotes: {},
      setEpisodeId: (episodeId) => set({ episodeId }),
      setDraft: (draft) => set({ draft }),
      addNote: (episodeId, note) =>
        set((state) => ({
          extraNotes: {
            ...state.extraNotes,
            [episodeId]: [note, ...(state.extraNotes[episodeId] ?? [])]
          }
        })),
      clearDraft: () => set({ draft: '' })
    }),
    {
      name: 'echonote.capture'
    }
  )
)
