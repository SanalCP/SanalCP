import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type Durum = {
  hostname: string
  korumali: boolean
  aciklama: string
}

export default function HostnameAyari() {
  const { t } = useTranslation(['HostnameAyari'])
  const [durum, setDurum] = useState<Durum | null>(null)
  const [hostname, setHostname] = useState('')
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [hata, setHata] = useState('')
  const [basari, setBasari] = useState('')

  const yukle = useCallback(async () => {
    try {
      const { data } = await api.get<Durum>('/system/hostname')
      setDurum(data)
      setHostname(data.hostname)
    } catch (e) {
      setHata(apiHata(e, t('HostnameAyari:load_failed')))
    }
  }, [t])

  useEffect(() => { void yukle() }, [yukle])

  async function kaydet() {
    setHata('')
    setBasari('')
    setKaydediliyor(true)
    try {
      const { data } = await api.put<Durum>('/system/hostname', { hostname: hostname.trim() })
      setDurum(data)
      setHostname(data.hostname)
      setBasari(t('HostnameAyari:saved', { hostname: data.hostname }))
    } catch (e) {
      setHata(apiHata(e, t('HostnameAyari:save_failed')))
    } finally {
      setKaydediliyor(false)
    }
  }

  return (
    <div className="rounded-2xl border border-sky-200 bg-sky-50 p-4 dark:border-sky-800/50 dark:bg-sky-900/15">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-sky-100 text-sky-700 dark:bg-sky-900/40 dark:text-sky-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 1 0 0 6h13.5a3 3 0 1 0 0-6m-13.5 0a3 3 0 1 1 0-6h13.5a3 3 0 1 1 0 6M18 11.25h.008v.008H18v-.008Zm0 6h.008v.008H18v-.008Z" />
          </svg>
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('HostnameAyari:label')}</span>
            {durum && (
              <span className={`rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide ${
                durum.korumali
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                  : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
              }`}>
                {durum.korumali ? t('HostnameAyari:protected') : t('HostnameAyari:unprotected')}
              </span>
            )}
          </div>
          <p className="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">
            {t('HostnameAyari:desc')} <code className="text-[11px]">server1.ornek.com</code>
          </p>

          {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}
          {basari && <div className="mt-2 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{basari}</div>}

          <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
            <input
              type="text"
              value={hostname}
              onChange={e => setHostname(e.target.value)}
              placeholder="server1.ornek.com"
              autoComplete="off"
              spellCheck={false}
              maxLength={253}
              className="w-full max-w-md rounded-lg border border-slate-300 bg-white px-3 py-1.5 font-mono text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-sky-500 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
            />
            <button
              type="button"
              onClick={kaydet}
              disabled={kaydediliyor || !hostname.trim() || hostname.trim().toLowerCase() === durum?.hostname.toLowerCase()}
              className="rounded-lg bg-sky-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-sky-700 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {kaydediliyor ? t('HostnameAyari:saving') : t('HostnameAyari:submit')}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
