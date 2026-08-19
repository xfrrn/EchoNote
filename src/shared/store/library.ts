import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Episode } from '../types'

interface LibraryState {
  /** 通过「导入」加入的节目（Mock，转录中），排在最前 */
  imported: Episode[]
  addImported: (episode: Episode) => void
}

export const useLibraryStore = create<LibraryState>()(
  persist(
    (set) => ({
      imported: [],
      addImported: (episode) => set((state) => ({ imported: [episode, ...state.imported] }))
    }),
    {
      name: 'echonote.library'
    }
  )
)
