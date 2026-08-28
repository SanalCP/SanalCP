import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type Durum = {
  saat_dilimi: string; yerel_saat: string
  ntp_aktif: boolean; ntp_senkron: boolean; donanim_saati_utc: boolean
  saat_dilimleri?: string[]
}

function saatYaz(v: string) {
  // Sunucunun verdiği duvar saatini tarayıcının kendi saat dilimine çevirmeden göster.
  return v.replace('T', ' ')
}

export default function SunucuSaatiAyari() {
  const { t } = useTranslation('SunucuSaatiAyari')
  const [durum, setDurum] = useState<Durum | null>(null)
  const [saatDilimi, setSaatDilimi] = useState('')
  const [ntp, setNtp] = useState(true)
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState('')
  const [basari, setBasari] = useState('')

  const yukle = useCallback(async () => {
    setHata('')
    try {
      const { data } = await api.get<Durum>('/system/saat')
      setDurum(data); setSaatDilimi(data.saat_dilimi); setNtp(data.ntp_aktif)
    } catch (e) { setHata(apiHata(e, t('load_failed'))) }
  }, [t])
  useEffect(() => { void yukle() }, [yukle])

  async function kaydet() {
    setIsleniyor(true); setHata(''); setBasari('')
    try {
      await api.put('/system/saat', { saat_dilimi: saatDilimi, ntp_aktif: ntp })
      setBasari(t('saved')); await yukle()
    } catch (e) { setHata(apiHata(e, t('save_failed'))) }
    finally { setIsleniyor(false) }
  }

  const degisti = !!durum && (saatDilimi !== durum.saat_dilimi || ntp !== durum.ntp_aktif)
  return (
    <div className="h-full rounded-2xl border border-violet-200 bg-violet-50 p-4 dark:border-violet-800/50 dark:bg-violet-900/15">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-violet-100 text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5"><circle cx="12" cy="12" r="9"/><path strokeLinecap="round" d="M12 7v5l3 2"/></svg>
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('title')}</span>
            {durum && <span className={`rounded-full px-2 py-0.5 text-[10px] font-medium uppercase ${durum.ntp_senkron ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'}`}>{durum.ntp_senkron ? t('synced') : t('not_synced')}</span>}
          </div>
          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t('desc')}</p>
          {durum && <div className="mt-2 text-[11px] text-slate-600 dark:text-slate-300"><span>{t('server_time')}: <code>{saatYaz(durum.yerel_saat)}</code></span></div>}
          {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}
          {basari && <div className="mt-2 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{basari}</div>}
          <div className="mt-3 flex flex-col gap-2">
            <select value={saatDilimi} onChange={e => setSaatDilimi(e.target.value)} disabled={!durum || isleniyor} className="w-full rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-xs dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200">
              {(durum?.saat_dilimleri || []).map(z => <option key={z} value={z}>{z}</option>)}
            </select>
            <div className="flex flex-wrap items-center gap-3">
              <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300"><input type="checkbox" checked={ntp} onChange={e => setNtp(e.target.checked)} disabled={!durum || isleniyor}/>{t('ntp')}</label>
              <button type="button" onClick={kaydet} disabled={isleniyor || !degisti} className="rounded-lg bg-violet-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-violet-700 disabled:opacity-40">{isleniyor ? t('saving') : t('save')}</button>
              <button type="button" onClick={() => void yukle()} disabled={isleniyor} className="text-xs text-slate-500 hover:text-slate-700 dark:text-slate-400">{t('refresh')}</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
