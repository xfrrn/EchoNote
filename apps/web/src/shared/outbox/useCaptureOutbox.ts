import { useCallback, useEffect, useState } from 'react'
import { listCaptureOutbox, OUTBOX_EVENT, type CaptureOutboxItem } from './captureOutbox'

export function useCaptureOutbox(episodeId?: string) {
  const [items, setItems] = useState<CaptureOutboxItem[]>([])
  const refresh = useCallback(() => void listCaptureOutbox(episodeId).then(setItems), [episodeId])

  useEffect(() => {
    refresh()
    window.addEventListener(OUTBOX_EVENT, refresh)
    return () => window.removeEventListener(OUTBOX_EVENT, refresh)
  }, [refresh])

  return items
}
