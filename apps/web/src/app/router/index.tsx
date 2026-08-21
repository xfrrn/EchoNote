import { lazy, Suspense } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AppShell } from '../layout/AppShell'
import { LibraryPage } from '../../features/library/LibraryPage'
import { EpisodePage } from '../../features/episode/EpisodePage'
import { CapturePage } from '../../features/capture/CapturePage'
import { SearchPage } from '../../features/search/SearchPage'
import { MinePage } from '../../features/mine/MinePage'
import { AuthProvider, RequireAuth } from '../../features/auth/AuthProvider'
import { LoginPage } from '../../features/auth/LoginPage'

const DesignPlaygroundPage = import.meta.env.DEV
  ? lazy(() => import('../../features/design/DesignPlaygroundPage').then((module) => ({ default: module.DesignPlaygroundPage })))
  : null

export function AppRouter() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<Navigate to="/library" replace />} />
          <Route element={<RequireAuth><AppShell /></RequireAuth>}>
            <Route path="/library" element={<LibraryPage />} />
            <Route path="/episode/:id" element={<EpisodePage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/mine" element={<MinePage />} />
          </Route>
          <Route path="/capture" element={<RequireAuth><CapturePage /></RequireAuth>} />
          {DesignPlaygroundPage ? (
            <Route path="/dev/design" element={<Suspense fallback={null}><DesignPlaygroundPage /></Suspense>} />
          ) : null}
          <Route path="*" element={<Navigate to="/library" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}
