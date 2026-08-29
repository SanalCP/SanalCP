// Panelde hiçbir hata sınırı yoktu: bir sayfanın render sırasında ya da lazy
// import'unda patlaması tüm React ağacını unmount ediyor, kullanıcı boş beyaz
// sayfa görüyordu. Bu bileşen hatayı yakalar, sebebi eskimiş bir parça ise
// (yeni sürüm yayınlanmış, açık sekme eski dosyaları istiyor) sayfayı bir kez
// yeniler, aksi hâlde kullanıcıya toparlanabileceği bir arayüz gösterir.

import { Component, type ErrorInfo, type ReactNode } from 'react'
import i18n from '@/i18n'
import { chunkYenilemeDene, chunkYuklemeHatasiMi } from '@/lib/chunk'

type Props = {
  children: ReactNode
  /** Kabuk (menü/üst çubuk) yoksa tüm ekranı kaplayan yerleşim kullanılır. */
  tamEkran?: boolean
}

type State = {
  hata: Error | null
  /** Otomatik yenileme başlatıldı; hata kartı yerine kısa bir bilgi gösterilir. */
  yenileniyor: boolean
}

// Çeviriler yalnız önceden yüklenen "common" namespace'inden okunur. Hata anında
// yeni bir namespace indirmek aynı sebeple (eskimiş parça) başarısız olabilirdi.
const c = (anahtar: string, varsayilan: string) =>
  i18n.t(`common:error_boundary.${anahtar}`, { defaultValue: varsayilan })

export default class HataSiniri extends Component<Props, State> {
  state: State = { hata: null, yenileniyor: false }

  // Saf: yalnız durumu türetir. Parça hatasında doğrudan "yenileniyor" ile
  // başlanır ki hata kartı bir kare bile görünmesin.
  static getDerivedStateFromError(hata: Error): State {
    return { hata, yenileniyor: chunkYuklemeHatasiMi(hata) }
  }

  componentDidCatch(hata: Error, bilgi: ErrorInfo) {
    console.error('[SanalCP] yakalanan render hatası:', hata, bilgi.componentStack)
    // Yenileme reddedilirse (aynı oturumda az önce denenmiş) döngüye girmeyip
    // elle toparlama arayüzüne düşülür.
    if (this.state.yenileniyor && !chunkYenilemeDene()) {
      this.setState({ yenileniyor: false })
    }
  }

  render() {
    const { hata, yenileniyor } = this.state
    if (!hata) return this.props.children

    const sarmal = this.props.tamEkran
      ? 'flex min-h-screen items-center justify-center bg-[#f9fafb] p-6 dark:bg-[#101828]'
      : 'flex min-h-[60vh] items-center justify-center p-6'

    if (yenileniyor) {
      return (
        <div className={sarmal}>
          <p className="text-sm font-medium text-slate-500 dark:text-slate-400" role="status">
            {c('refreshing', 'Yeni sürüm yayınlandı, panel yenileniyor…')}
          </p>
        </div>
      )
    }

    return (
      <div className={sarmal}>
        <div
          role="alert"
          className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 text-center dark:border-slate-800 dark:bg-slate-900/60"
        >
          <svg
            className="mx-auto h-10 w-10 text-amber-500"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            strokeWidth={1.6}
            aria-hidden="true"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
            />
          </svg>

          <h1 className="mt-4 text-base font-semibold text-slate-800 dark:text-slate-100">
            {c('title', 'Bu sayfa yüklenemedi')}
          </h1>
          <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
            {c('desc', 'Sayfa beklenmedik bir hatayla karşılaştı. Panel yeni bir sürüme güncellenmiş olabilir; sayfayı yenilemek çoğu durumda sorunu çözer.')}
          </p>

          <div className="mt-5 flex justify-center gap-2">
            <button
              type="button"
              onClick={() => window.location.reload()}
              className="rounded-lg bg-brand-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-brand-700"
            >
              {c('reload', 'Sayfayı yenile')}
            </button>
            {/* Router durumu da bozulmuş olabileceği için sert yönlendirme. */}
            <a
              href="/"
              className="rounded-lg border border-slate-200 px-4 py-2 text-sm font-medium text-slate-600 transition hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
            >
              {c('home', 'Anasayfaya dön')}
            </a>
          </div>

          <details className="mt-5 text-left">
            <summary className="cursor-pointer text-xs text-slate-400 dark:text-slate-500">
              {c('details', 'Teknik ayrıntı')}
            </summary>
            <pre className="mt-2 max-h-40 overflow-auto rounded-lg bg-slate-50 p-3 text-[11px] leading-relaxed text-slate-600 dark:bg-slate-950 dark:text-slate-400">
              {hata.message || String(hata)}
            </pre>
          </details>
        </div>
      </div>
    )
  }
}
