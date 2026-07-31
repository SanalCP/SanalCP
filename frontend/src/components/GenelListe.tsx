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

export type Kolon<T> = {
  baslik: string
  hucre: (satir: T) => React.ReactNode
  sinif?: string // hücre hizalaması vb.
  dar?: boolean
}

export type Rozet = { etiket: string; deger: React.ReactNode; vurgu?: 'normal' | 'uyari' | 'tehlike' }

export default function GenelListe<T>({
  baslik, aciklama, uc, kolonlar, araAlan, satirAnahtar, bosMesaj, ozet,
}: {
  baslik: string
  aciklama: string
  uc: string
  kolonlar: Kolon<T>[]
  araAlan: (satir: T) => string
  satirAnahtar: (satir: T) => string | number
  bosMesaj: string
  ozet?: (liste: T[]) => Rozet[]
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
  }, [uc])

  const suzulmus = useMemo(() => {
    const t = aranan.trim().toLowerCase()
    if (!t) return liste
    return liste.filter((s) => araAlan(s).toLowerCase().includes(t))
  }, [liste, aranan, araAlan])

  const rozetler = ozet && liste.length > 0 ? ozet(liste) : []

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: baslik }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{baslik}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">{aciklama}</p>
      </div>

      {rozetler.length > 0 && (
        <div className="flex flex-wrap gap-2 mb-4">
          {rozetler.map((r) => (
            <div
              key={r.etiket}
              className={`px-3 py-2 rounded-lg border text-sm ${
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
        <div className="mb-3">
          <input
            value={aranan}
            onChange={(e) => setAranan(e.target.value)}
            placeholder={t('common:search_placeholder')}
            className="w-full sm:w-72 px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-1 focus:ring-brand-500"
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
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/60">
              <tr>
                {kolonlar.map((k) => (
                  <th
                    key={k.baslik}
                    className={`px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap ${k.sinif ?? ''}`}
                  >
                    {k.baslik}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800 bg-white dark:bg-slate-950">
              {suzulmus.map((s) => (
                <tr key={satirAnahtar(s)} className="hover:bg-slate-50 dark:hover:bg-slate-900/60 transition">
                  {kolonlar.map((k) => (
                    <td key={k.baslik} className={`px-3 py-2.5 text-slate-700 dark:text-slate-300 ${k.dar ? 'whitespace-nowrap' : ''} ${k.sinif ?? ''}`}>
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
