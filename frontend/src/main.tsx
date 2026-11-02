import '@fontsource-variable/bricolage-grotesque'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './styles.css'
import './detail.css'
import './popover.css'
import './dialogs.css'
import { ConfirmProvider } from './components/ConfirmDialog'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfirmProvider>
      <App />
    </ConfirmProvider>
  </StrictMode>,
)
