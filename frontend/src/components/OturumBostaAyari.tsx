import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type Durum = { dakika: number }

const VARSAYILAN_DAKIKA = 30

export default function OturumBostaAyari() {
  const { t } = useTranslation(['OturumBostaAyari'])
  const [aktif, setAktif] = useState(false)
  const [dakika, setDakika] = useState(VARSAYILAN_DAKIKA)
  const [kayitliDakika, setKayitliDakika] = useState(0)
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [hata, setHata] = useState('')
  const [basari, setBasari] = useState('')

  const yukle = useCallback(async () => {
    try {
      const { data } = await api.get<Durum>('/system/oturum-bosta')
      setKayitliDakika(data.dakika)
      setAktif(data.dakika > 0)
      if (data.dakika > 0) setDakika(data.dakika)
    } catch (e) {
      setHata(apiHata(e, t('OturumBostaAyari:load_failed')))
    }
  }, [t])

  useEffect(() => { void yukle() }, [yukle])

  async function kaydet() {
    setHata('')
    setBasari('')
    setKaydediliyor(true)
    const yeniDeger = aktif ? dakika : 0
    try {
      const { data } = await api.put<Durum>('/system/oturum-bosta', { dakika: yeniDeger })
      setKayitliDakika(data.dakika)
      setBasari(data.dakika > 0
        ? t('OturumBostaAyari:saved_on', { dakika: data.dakika })
        : t('OturumBostaAyari:saved_off'))
    } catch (e) {
      setHata(apiHata(e, t('OturumBostaAyari:save_failed')))
    } finally {
      setKaydediliyor(false)
    }
  }

  const degisti = aktif !== (kayitliDakika > 0) || (aktif && dakika !== kayitliDakika)

  return (
    <div className="h-full rounded-2xl border border-sky-200 bg-sky-50 p-4 dark:border-sky-800/50 dark:bg-sky-900/15">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-sky-100 text-sky-700 dark:bg-sky-900/40 dark:text-sky-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 6v6l4 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
          </svg>
        </div>
        <div className="min-w-0 flex-1">
          <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('OturumBostaAyari:label')}</span>
          <p className="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">
            {t('OturumBostaAyari:desc')}
          </p>

          {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}
          {basari && <div className="mt-2 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{basari}</div>}

          <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
            <label className="flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300">
              <input
                type="checkbox"
                checked={aktif}
                onChange={e => setAktif(e.target.checked)}
                className="h-4 w-4 rounded border-slate-300 text-sky-600 focus:ring-sky-500 dark:border-slate-600"
              />
              {t('OturumBostaAyari:enable')}
            </label>
            <input
              type="number"
              min={1}
              max={1440}
              value={dakika}
              disabled={!aktif}
              onChange={e => setDakika(Math.max(1, Math.min(1440, Number(e.target.value) || 1)))}
              className="w-24 rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-sky-500 disabled:cursor-not-allowed disabled:opacity-40 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
            />
            <span className="text-xs text-slate-500 dark:text-slate-400">{t('OturumBostaAyari:minutes')}</span>
            <button
              type="button"
              onClick={kaydet}
              disabled={kaydediliyor || !degisti}
              className="rounded-lg bg-sky-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-sky-700 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {kaydediliyor ? t('OturumBostaAyari:saving') : t('OturumBostaAyari:submit')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
