import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'
import { applyTheme, getStoredTheme } from './theme'
import { TooltipProvider } from '@/components/ui/tooltip'

applyTheme(getStoredTheme())

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <TooltipProvider>
      <App />
    </TooltipProvider>
  </React.StrictMode>,
)
