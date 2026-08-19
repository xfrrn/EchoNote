import React from 'react'
import ReactDOM from 'react-dom/client'
import './styles/tokens.css'
import './styles/globals.css'
import { AppProviders } from './app/providers/AppProviders'
import { AppRouter } from './app/router'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <AppProviders>
      <AppRouter />
    </AppProviders>
  </React.StrictMode>
)
