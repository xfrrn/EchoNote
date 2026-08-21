import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface CaptureState {
  episodeId: string
  draft: string
  setEpisodeId: (episodeId: string) => void
  setDraft: (draft: string) => void
  clearDraft: () => void
}

export const useCaptureStore = create<CaptureState>()(
  persist(
    (set) => ({
      episodeId: '',
      draft: '',
      setEpisodeId: (episodeId) => set({ episodeId }),
      setDraft: (draft) => set({ draft }),
      clearDraft: () => set({ draft: '' })
    }),
    {
      name: 'echonote.capture'
    }
  )
)
