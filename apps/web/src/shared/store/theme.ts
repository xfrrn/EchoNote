import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type ThemeMode = 'light' | 'dark' | 'system'

export const useThemeStore = create<{ theme: ThemeMode; setTheme: (theme: ThemeMode) => void }>()(
  persist(
    (set) => ({ theme: 'system', setTheme: (theme) => set({ theme }) }),
    { name: 'echonote.theme' }
  )
)
