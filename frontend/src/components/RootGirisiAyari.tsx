import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type Durum = { acik: boolean }

export default function RootGirisiAyari() {
  const { t } = useTranslation(['RootGirisiAyari'])
  const [acik, setAcik] = useState(true)
  const [kayitli, setKayitli] = useState(true)
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [hata, setHata] = useState('')
  const [basari, setBasari] = useState('')

  const yukle = useCallback(async () => {
    try {
      const { data } = await api.get<Durum>('/system/root-girisi')
      setAcik(data.acik)
      setKayitli(data.acik)
    } catch (e) {
      setHata(apiHata(e, t('RootGirisiAyari:load_failed')))
    }
  }, [t])

  useEffect(() => { void yukle() }, [yukle])

  async function kaydet() {
    setHata('')
    setBasari('')
    setKaydediliyor(true)
    try {
      const { data } = await api.put<Durum>('/system/root-girisi', { acik })
      setKayitli(data.acik)
      setAcik(data.acik)
      setBasari(data.acik
        ? t('RootGirisiAyari:saved_on')
        : t('RootGirisiAyari:saved_off'))
    } catch (e) {
      // Sunucu, root dışında aktif admin yokken kapatmayı reddeder —
      // mesajı olduğu gibi göster, kullanıcı ne yapması gerektiğini öğrensin.
      setHata(apiHata(e, t('RootGirisiAyari:save_failed')))
      setAcik(kayitli)
    } finally {
      setKaydediliyor(false)
    }
  }

  const degisti = acik !== kayitli

  return (
    <div className="rounded-2xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-800/50 dark:bg-amber-900/15">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75M6.75 10.5h10.5a2.25 2.25 0 0 1 2.25 2.25v6a2.25 2.25 0 0 1-2.25 2.25H6.75a2.25 2.25 0 0 1-2.25-2.25v-6a2.25 2.25 0 0 1 2.25-2.25Z" />
          </svg>
        </div>
        <div className="min-w-0 flex-1">
          <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('RootGirisiAyari:label')}</span>
          <p className="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">
            {t('RootGirisiAyari:desc')}
          </p>

          {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}
          {basari && <div className="mt-2 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{basari}</div>}

          <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
            <label className="flex items-center gap-2 text-xs text-slate-700 dark:text-slate-300">
              <input
                type="checkbox"
                checked={acik}
                onChange={(e) => setAcik(e.target.checked)}
                className="h-4 w-4 rounded border-slate-300 text-amber-600 focus:ring-amber-500"
              />
              {acik ? t('RootGirisiAyari:on') : t('RootGirisiAyari:off')}
            </label>
            <button
              type="button"
              onClick={() => void kaydet()}
              disabled={!degisti || kaydediliyor}
              className="rounded-lg bg-amber-600 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
            >
              {kaydediliyor ? t('RootGirisiAyari:saving') : t('RootGirisiAyari:save')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
