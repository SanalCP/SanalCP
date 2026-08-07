import React, { Suspense } from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import './styles.css'
import { bootTheme } from '@/lib/theme'
import { bootLang, applyServerDefaultLang, i18nReady } from '@/i18n'

bootTheme()
bootLang()

async function baslat() {
  // İlk boyamada common namespace hazır olsun; sayfa/bileşen namespace'leri
  // Suspense sınırı içinde ihtiyaç anında yüklenir.
  await i18nReady
  void applyServerDefaultLang()

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <Suspense fallback={
        <div className="flex min-h-screen items-center justify-center bg-[#f9fafb] text-sm font-medium text-slate-500 dark:bg-[#101828] dark:text-slate-400" role="status">
          SanalCP yükleniyor…
        </div>
      }>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </Suspense>
    </React.StrictMode>,
  )
}

void baslat()
