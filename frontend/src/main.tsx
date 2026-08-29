import React, { Suspense } from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import './styles.css'
import HataSiniri from '@/components/HataSiniri'
import { bootTheme } from '@/lib/theme'
import { bootLang, applyServerDefaultLang, i18nReady } from '@/i18n'
import { chunkYenilemeDene } from '@/lib/chunk'

bootTheme()
bootLang()

// Vite, bir parçanın ön-yüklemesi başarısız olduğunda bu olayı tetikler. Yeni
// sürüm yayınlandıktan sonra açık kalmış sekmelerde tipik durum budur; hatayı
// React'e ulaştırmadan önce sayfayı yenileyip taze dosyalarla açıyoruz.
window.addEventListener('vite:preloadError', ((olay: Event) => {
  if (chunkYenilemeDene()) olay.preventDefault()
}) as EventListener)

async function baslat() {
  // İlk boyamada common namespace hazır olsun; sayfa/bileşen namespace'leri
  // Suspense sınırı içinde ihtiyaç anında yüklenir.
  await i18nReady
  void applyServerDefaultLang()

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <HataSiniri tamEkran>
        <Suspense fallback={
          <div className="flex min-h-screen items-center justify-center bg-[#f9fafb] text-sm font-medium text-slate-500 dark:bg-[#101828] dark:text-slate-400" role="status">
            SanalCP yükleniyor…
          </div>
        }>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </Suspense>
      </HataSiniri>
    </React.StrictMode>,
  )
}

void baslat()
