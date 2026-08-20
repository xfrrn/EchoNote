import { useEffect, useState } from 'react'

export function useVisualViewportHeight(): number | null {
  const [height, setHeight] = useState<number | null>(null)

  useEffect(() => {
    const viewport = window.visualViewport
    if (!viewport) {
      setHeight(window.innerHeight)
      return undefined
    }

    const update = () => setHeight(viewport.height)
    update()
    viewport.addEventListener('resize', update)
    viewport.addEventListener('scroll', update)
    return () => {
      viewport.removeEventListener('resize', update)
      viewport.removeEventListener('scroll', update)
    }
  }, [])

  return height
}
