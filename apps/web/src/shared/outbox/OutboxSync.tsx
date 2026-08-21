import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { OUTBOX_EVENT, replayCaptureOutbox } from './captureOutbox'

export function OutboxSync() {
  const queryClient = useQueryClient()

  useEffect(() => {
    const replay = () => void replayCaptureOutbox()
    const refresh = (event: Event) => {
      const episodeId = (event as CustomEvent<{ episodeId?: string }>).detail?.episodeId
      void queryClient.invalidateQueries({ queryKey: ['episodes'] })
      if (episodeId) {
        void queryClient.invalidateQueries({ queryKey: ['episode', episodeId] })
        void queryClient.invalidateQueries({ queryKey: ['notes', episodeId] })
      }
    }
    replay()
    window.addEventListener('online', replay)
    window.addEventListener(OUTBOX_EVENT, refresh)
    return () => {
      window.removeEventListener('online', replay)
      window.removeEventListener(OUTBOX_EVENT, refresh)
    }
  }, [queryClient])

  return null
}
