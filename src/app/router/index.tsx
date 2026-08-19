import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppShell } from '../layout/AppShell'
import { LibraryPage } from '../../features/library/LibraryPage'
import { EpisodePage } from '../../features/episode/EpisodePage'
import { CapturePage } from '../../features/capture/CapturePage'
import { SearchPage } from '../../features/search/SearchPage'
import { MinePage } from '../../features/mine/MinePage'
import { DesignPlaygroundPage } from '../../features/design/DesignPlaygroundPage'

export function AppRouter() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Navigate to="/library" replace />} />
        <Route element={<AppShell />}>
          <Route path="/library" element={<LibraryPage />} />
          <Route path="/episode/:id" element={<EpisodePage />} />
          <Route path="/search" element={<SearchPage />} />
          <Route path="/mine" element={<MinePage />} />
        </Route>
        <Route path="/capture" element={<CapturePage />} />
        <Route path="/dev/design" element={<DesignPlaygroundPage />} />
        <Route path="*" element={<Navigate to="/library" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
