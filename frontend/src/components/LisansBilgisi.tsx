import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type SurumBilgi = { mevcut: string; build_tarihi?: string; kurulum_kimlik: string }

export default function LisansBilgisi() {
  const { t } = useTranslation(['LisansBilgisi'])
  const [bilgi, setBilgi] = useState<SurumBilgi | null>(null)
  const [hata, setHata] = useState('')
  const [kopyalandi, setKopyalandi] = useState(false)

  const yukle = useCallback(async () => {
    try {
      const { data } = await api.get<SurumBilgi>('/system/surum')
      setBilgi(data)
    } catch (e) {
      setHata(apiHata(e, t('LisansBilgisi:load_failed')))
    }
  }, [t])

  useEffect(() => { void yukle() }, [yukle])

  async function kopyala() {
    if (!bilgi?.kurulum_kimlik) return
    await navigator.clipboard.writeText(bilgi.kurulum_kimlik)
    setKopyalandi(true)
    setTimeout(() => setKopyalandi(false), 2000)
  }

  return (
    <div className="h-full rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900/60">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5" aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 5.25a3 3 0 0 1 3 3m3 0a6 6 0 0 1-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1 1 21.75 8.25Z" />
          </svg>
        </div>
        <div className="min-w-0 flex-1">
          <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('LisansBilgisi:label')}</span>
          <p className="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">{t('LisansBilgisi:desc')}</p>

          {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}

          {bilgi && (
            <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
              <code className="flex-1 truncate rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-700 dark:border-slate-800 dark:bg-slate-950/60 dark:text-slate-300">
                {bilgi.kurulum_kimlik}
              </code>
              <button
                type="button"
                onClick={kopyala}
                className="rounded-lg border border-slate-200 px-3 py-2 text-xs font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-300 dark:hover:bg-slate-800"
              >
                {kopyalandi ? t('LisansBilgisi:copied') : t('LisansBilgisi:copy')}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
