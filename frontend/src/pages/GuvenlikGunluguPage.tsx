// Güvenlik günlüğü — audit_log'un okunabilir yüzü. Tablo panelin ilk
// sürümünden beri doluyordu ama panelde hiçbir yerden görünmüyordu; başarısız
// giriş denemelerini görmek için sunucuya SSH ile girmek gerekiyordu.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'

type Kayit = {
  id: number
  zaman: string
  kullanici: string
  ip: string
  eylem: string
  hedef: string
  basarili: boolean
}

export default function GuvenlikGunluguPage() {
  const { t } = useTranslation(['GuvenlikGunluguPage', 'common'])
  const [liste, setListe] = useState<Kayit[]>([])
  const [eylemler, setEylemler] = useState<string[]>([])
  const [yukleniyor, setYukleniyor] = useState(true)
  const [hata, setHata] = useState<string | null>(null)

  const [eylem, setEylem] = useState('')
  const [sadeceHata, setSadeceHata] = useState(false)
  const [limit, setLimit] = useState(200)
  const [aranan, setAranan] = useState('')

  const getir = useCallback(async () => {
    setYukleniyor(true)
    try {
      const p = new URLSearchParams()
      p.set('limit', String(limit))
      if (eylem) p.set('eylem', eylem)
      if (sadeceHata) p.set('sadece_hata', '1')
      const r = await api.get<Kayit[]>(`/audit?${p.toString()}`)
      setListe(Array.isArray(r.data) ? r.data : [])
      setHata(null)
    } catch (e) {
      setHata(apiHata(e, t('GuvenlikGunluguPage:load_failed')))
    } finally {
      setYukleniyor(false)
    }
  }, [eylem, sadeceHata, limit, t])

  useEffect(() => { getir() }, [getir])

  useEffect(() => {
    api.get<string[]>('/audit/eylemler')
      .then((r) => setEylemler(Array.isArray(r.data) ? r.data : []))
      .catch(() => {})
  }, [])

  // Arama istemcide: sunucu filtresi eylem/başarı için, serbest metin (IP,
  // hedef, kullanıcı) çekilmiş sayfa üzerinde aranıyor.
  const suzulmus = useMemo(() => {
    const q = aranan.trim().toLowerCase()
    if (!q) return liste
    return liste.filter((k) =>
      `${k.kullanici} ${k.ip} ${k.eylem} ${k.hedef}`.toLowerCase().includes(q))
  }, [liste, aranan])

  const basarisiz = liste.filter((k) => !k.basarili).length

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('GuvenlikGunluguPage:breadcrumb_title') }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('GuvenlikGunluguPage:title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {t('GuvenlikGunluguPage:subtitle')}
        </p>
      </div>

      {liste.length > 0 && (
        <div className="flex flex-wrap gap-2 mb-4">
          <div className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-sm text-slate-700 dark:text-slate-300">
            <span className="font-semibold">{liste.length}</span> <span className="opacity-75">{t('GuvenlikGunluguPage:records_label')}</span>
          </div>
          {basarisiz > 0 && (
            <div className="px-3 py-2 rounded-lg border border-red-200 bg-red-50 text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300 text-sm">
              <span className="font-semibold">{basarisiz}</span> <span className="opacity-75">{t('GuvenlikGunluguPage:failed_label')}</span>
            </div>
          )}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2 mb-4">
        <input
          value={aranan}
          onChange={(e) => setAranan(e.target.value)}
          placeholder={t('GuvenlikGunluguPage:search_placeholder')}
          className="w-full sm:w-64 px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-1 focus:ring-brand-500"
        />
        <select
          value={eylem}
          onChange={(e) => setEylem(e.target.value)}
          className="px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
        >
          <option value="">{t('GuvenlikGunluguPage:all_actions')}</option>
          {eylemler.map((a) => <option key={a} value={a}>{a}</option>)}
        </select>
        <select
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
          className="px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
        >
          <option value={100}>{t('GuvenlikGunluguPage:limit_100')}</option>
          <option value={200}>{t('GuvenlikGunluguPage:limit_200')}</option>
          <option value={500}>{t('GuvenlikGunluguPage:limit_500')}</option>
          <option value={1000}>{t('GuvenlikGunluguPage:limit_1000')}</option>
        </select>
        <label className="inline-flex items-center gap-1.5 px-3 py-2 text-sm text-slate-600 dark:text-slate-400 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={sadeceHata}
            onChange={(e) => setSadeceHata(e.target.checked)}
            className="rounded border-slate-300 dark:border-slate-700"
          />
          {t('GuvenlikGunluguPage:only_failed')}
        </label>
      </div>

      {hata && (
        <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{hata}</div>
      )}

      {yukleniyor ? (
        <div className="py-16 text-center text-sm text-slate-400">{t('common:loading')}</div>
      ) : liste.length === 0 && !hata ? (
        <EmptyState baslik={t('GuvenlikGunluguPage:empty_title')} aciklama={t('GuvenlikGunluguPage:empty_desc')} />
      ) : suzulmus.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">{t('GuvenlikGunluguPage:no_search_results')}</div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/60">
              <tr>
                {[t('GuvenlikGunluguPage:col_time'), t('GuvenlikGunluguPage:col_result'), t('GuvenlikGunluguPage:col_action'), t('GuvenlikGunluguPage:col_user'), t('GuvenlikGunluguPage:col_ip'), t('GuvenlikGunluguPage:col_target')].map((b) => (
                  <th key={b} className="px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">
                    {b}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800 bg-white dark:bg-slate-950">
              {suzulmus.map((k) => (
                <tr key={k.id} className="hover:bg-slate-50 dark:hover:bg-slate-900/60 transition">
                  <td className="px-3 py-2 whitespace-nowrap font-mono text-xs text-slate-500">{k.zaman}</td>
                  <td className="px-3 py-2 whitespace-nowrap">
                    {k.basarili
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('GuvenlikGunluguPage:result_success')}</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">{t('GuvenlikGunluguPage:result_failed')}</span>}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap font-mono text-xs">{k.eylem}</td>
                  <td className="px-3 py-2 whitespace-nowrap">{k.kullanici || <span className="text-slate-400">—</span>}</td>
                  <td className="px-3 py-2 whitespace-nowrap font-mono text-xs text-slate-500">{k.ip || '—'}</td>
                  <td className="px-3 py-2 text-slate-600 dark:text-slate-400">{k.hedef || <span className="text-slate-400">—</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}