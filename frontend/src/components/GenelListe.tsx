// sanal-dark-swept-v2
// Sunucu geneli özet listelerinin ortak kabuğu (DNS / SSL / E-posta /
// Veritabanları). Dördü de aynı işi yapıyor: tek bir salt-okunur ucu çek,
// ara, tabloda göster, satırdan ilgili domain sayfasına gönder. Kolon
// tanımları dışında fark kalmadığı için ortak tutuldu.
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from './Breadcrumb'
import EmptyState from './EmptyState'
import { T } from '@/lib/tablo'

export type Kolon<T> = {
  baslik: string
  hucre: (satir: T) => React.ReactNode
  sinif?: string // hücre hizalaması vb.
  dar?: boolean
}

export type Rozet = { etiket: string; deger: React.ReactNode; vurgu?: 'normal' | 'uyari' | 'tehlike' }

export default function GenelListe<T>({
  baslik, aciklama, uc, kolonlar, araAlan, satirAnahtar, bosMesaj, ozet, yenilemeTetik,
}: {
  baslik: string
  aciklama: string
  uc: string
  kolonlar: Kolon<T>[]
  araAlan: (satir: T) => string
  satirAnahtar: (satir: T) => string | number
  bosMesaj: string
  ozet?: (liste: T[]) => Rozet[]
  // Değeri artınca listeyi yeniden çeker — satır-içi bir işlem (silme/parola
  // sıfırlama gibi) sonrası veriyi tazelemek isteyen sayfalar için.
  yenilemeTetik?: number
}) {
  const { t } = useTranslation(['GenelListe', 'common'])
  const [liste, setListe] = useState<T[]>([])
  const [yukleniyor, setYukleniyor] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [aranan, setAranan] = useState('')

  useEffect(() => {
    let iptal = false
    setYukleniyor(true)
    api.get<T[]>(uc)
      .then((r) => { if (!iptal) { setListe(Array.isArray(r.data) ? r.data : []); setHata(null) } })
      .catch((e) => { if (!iptal) setHata(apiHata(e, t('common:load_failed'))) })
      .finally(() => { if (!iptal) setYukleniyor(false) })
    return () => { iptal = true }
  }, [uc, yenilemeTetik])

  const suzulmus = useMemo(() => {
    const t = aranan.trim().toLowerCase()
    if (!t) return liste
    return liste.filter((s) => araAlan(s).toLowerCase().includes(t))
  }, [liste, aranan, araAlan])

  const rozetler = ozet && liste.length > 0 ? ozet(liste) : []

  return (
    <div className="ta-page">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: baslik }]} />

      <div className="mb-7">
        <div className="ta-eyebrow mb-2">SanalCP Management</div>
        <h1 className="ta-page-title">{baslik}</h1>
        <p className="ta-page-description">{aciklama}</p>
      </div>

      {rozetler.length > 0 && (
        <div className="grid grid-cols-2 gap-3 mb-5 sm:flex sm:flex-wrap">
          {rozetler.map((r) => (
            <div
              key={r.etiket}
              className={`px-4 py-3 rounded-xl border text-sm shadow-sm ${
                r.vurgu === 'tehlike'
                  ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300'
                  : r.vurgu === 'uyari'
                  ? 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300'
                  : 'border-slate-200 bg-white text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300'
              }`}
            >
              <span className="font-semibold">{r.deger}</span>{' '}
              <span className="opacity-75">{r.etiket}</span>
            </div>
          ))}
        </div>
      )}

      {liste.length > 0 && (
        <div className="mb-4 relative w-full sm:w-80">
          <svg className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            value={aranan}
            onChange={(e) => setAranan(e.target.value)}
            placeholder={t('common:search_placeholder')}
            className="ta-input w-full pl-10"
          />
        </div>
      )}

      {hata && (
        <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">
          {hata}
        </div>
      )}

      {yukleniyor ? (
        <div className="py-16 text-center text-sm text-slate-400">{t('common:loading')}</div>
      ) : liste.length === 0 && !hata ? (
        <EmptyState baslik={bosMesaj} />
      ) : suzulmus.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">{t('GenelListe:no_search_results')}</div>
      ) : (
        <div className="lg:overflow-x-auto lg:rounded-2xl lg:border lg:border-slate-200 lg:bg-white lg:shadow-sm dark:lg:border-slate-800 dark:lg:bg-slate-900">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50/80 dark:bg-slate-800/50`}>
              <tr>
                {kolonlar.map((k) => (
                  <th
                    key={k.baslik}
                    className={`${T.baslik} whitespace-nowrap ${k.sinif ?? ''}`}
                  >
                    {k.baslik}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
              {suzulmus.map((s) => (
                <tr key={satirAnahtar(s)} className={T.satir}>
                  {kolonlar.map((k, index) => (
                    <td
                      key={k.baslik}
                      data-etiket={index === 0 ? undefined : k.baslik}
                      className={`${index === 0 ? T.hucreBaslik : T.hucre} ${k.dar ? 'lg:whitespace-nowrap' : ''} ${k.sinif ?? ''}`}
                    >
                      {k.hucre(s)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
