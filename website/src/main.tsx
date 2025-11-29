import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/theme.scss'
import './styles/animations.css'
import { ThemeProvider } from './context/ThemeContext'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <App />
    </ThemeProvider>
  </StrictMode>,
)
