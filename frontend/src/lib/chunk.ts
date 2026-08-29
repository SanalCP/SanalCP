// Sürüm yayınlandığında frontend-dist yeni hash'li dosyalarla değişir. O sırada
// açık olan sekmeler hâlâ eski index.js'i çalıştırdığı için lazy() import'ları
// artık diskte olmayan bir dosyayı ister ve 404 alır; React ağacı çöker ve
// kullanıcı beyaz sayfa görür. Panelde main'e her push tüm kurulumlara yayın
// demek olduğundan bu senaryo her sürümde tekrarlanabilir.
//
// Çözüm: hatayı tanı ve sayfayı bir kez yenile. index.html "no-store" ile
// sunulduğu için yenileme taze hash'lerle açılır. Yenileme sorunu çözmediyse
// (gerçekten bozuk bir dağıtım) döngüye girmeden elle yenileme arayüzü gösterilir.

import { lazy, type ComponentType } from 'react'

const YENILEME_ANAHTARI = 'sanalcp.chunk-yenileme'
const YENILEME_PENCERESI_MS = 10_000

// Tarayıcılar aynı hatayı farklı metinlerle bildirir: Chrome "Failed to fetch
// dynamically imported module", Firefox "error loading dynamically imported
// module", Safari "Importing a module script failed". Vite'ın CSS ön-yükleme
// hatası da aynı sınıfa girer.
const IMZALAR = [
  'failed to fetch dynamically imported module',
  'error loading dynamically imported module',
  'importing a module script failed',
  'unable to preload css',
]

export function chunkYuklemeHatasiMi(hata: unknown): boolean {
  if (!hata) return false
  const metin = (hata instanceof Error ? `${hata.name} ${hata.message}` : String(hata)).toLowerCase()
  return IMZALAR.some((imza) => metin.includes(imza))
}

/**
 * Sayfa yenilemesini başlatır ve true döner. Aynı oturumda son 10 saniye içinde
 * zaten denenmişse — yani yenileme sorunu çözmemişse — hiçbir şey yapmadan
 * false döner; çağıran taraf o zaman sonsuz döngü yerine hata arayüzü gösterir.
 */
export function chunkYenilemeDene(): boolean {
  try {
    const onceki = Number(sessionStorage.getItem(YENILEME_ANAHTARI)) || 0
    if (Date.now() - onceki < YENILEME_PENCERESI_MS) return false
    sessionStorage.setItem(YENILEME_ANAHTARI, String(Date.now()))
  } catch {
    // sessionStorage kapalıysa (gizli sekme, site verisi engelli) döngüyü
    // durduracak kaydı tutamayız — otomatik yenileme yerine elle yenileme.
    return false
  }
  window.location.reload()
  return true
}

/**
 * lazy() yerine kullanılır: eski bir sekmenin artık var olmayan parçayı
 * istemesi durumunda hata sınırına düşmeden önce sayfayı yeniler.
 */
export function lazySayfa<T extends ComponentType<any>>(yukle: () => Promise<{ default: T }>) {
  return lazy(() =>
    yukle().catch((hata: unknown) => {
      if (chunkYuklemeHatasiMi(hata) && chunkYenilemeDene()) {
        // Yenileme başladı. Sayfa boşaltılırken React'in hata arayüzünü bir an
        // için göstermemesi adına hiç çözülmeyen bir promise döndürülür.
        return new Promise<{ default: T }>(() => {})
      }
      throw hata
    }),
  )
}
