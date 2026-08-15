import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { SpatialWorldProvider } from './contexts/SpatialWorldProvider.tsx'
import { ThemeProvider } from './contexts/ThemeContext'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
    <SpatialWorldProvider><App /></SpatialWorldProvider>
    </ThemeProvider>
  </StrictMode>,
)
