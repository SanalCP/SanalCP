import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

// cidrler sunucudan boş listede null gelebiliyordu; tip bunu yansıtır, okuma
// noktaları dizi()'den geçer.
type Durum = { cidrler: string[] | null; istemci_ip: string; gecici_cidr: string; gecici_bitis: string }

const dizi = (x: string[] | null | undefined) => x ?? []

export default function PanelErisimAyari() {
  const { t } = useTranslation(['PanelErisimAyari'])
  const [deger, setDeger] = useState('')
  const [istemciIP, setIstemciIP] = useState('')
  const [geciciCIDR, setGeciciCIDR] = useState('')
  const [geciciBitis, setGeciciBitis] = useState('')
  const [dakika, setDakika] = useState(60)
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [hata, setHata] = useState('')
  const [basari, setBasari] = useState('')

  const yukle = useCallback(async () => {
    try {
      const { data } = await api.get<Durum>('/system/panel-erisim')
      setDeger(dizi(data.cidrler).join('\n'))
      setIstemciIP(data.istemci_ip)
      setGeciciCIDR(data.gecici_cidr || '')
      setGeciciBitis(data.gecici_bitis || '')
    } catch (e) { setHata(apiHata(e, t('PanelErisimAyari:load_failed'))) }
  }, [t])
  useEffect(() => { void yukle() }, [yukle])

  async function kaydet() {
    setKaydediliyor(true); setHata(''); setBasari('')
    try {
      const cidrler = deger.split(/[\n,]+/).map(x => x.trim()).filter(Boolean)
      const { data } = await api.put<Durum>('/system/panel-erisim', { cidrler })
      setDeger(dizi(data.cidrler).join('\n')); setIstemciIP(data.istemci_ip)
      setBasari(t('PanelErisimAyari:saved'))
    } catch (e) { setHata(apiHata(e, t('PanelErisimAyari:save_failed'))) }
    finally { setKaydediliyor(false) }
  }

  async function geciciKaydet(iptal = false) {
    setKaydediliyor(true); setHata(''); setBasari('')
    try {
      const { data } = await api.put<Durum>('/system/panel-erisim/gecici', { cidr: iptal ? '' : geciciCIDR, dakika: iptal ? 0 : dakika })
      setGeciciCIDR(data.gecici_cidr || ''); setGeciciBitis(data.gecici_bitis || '')
      setBasari(t(iptal ? 'PanelErisimAyari:temporary_cancelled' : 'PanelErisimAyari:temporary_saved'))
    } catch (e) { setHata(apiHata(e, t('PanelErisimAyari:save_failed'))) }
    finally { setKaydediliyor(false) }
  }

  return <div className="rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900/60">
    <div className="flex items-start gap-3">
      <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-900/30 dark:text-brand-300">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} className="h-5 w-5" aria-hidden="true"><path strokeLinecap="round" strokeLinejoin="round" d="M12 3 4.5 6v5.25c0 4.64 3.2 8.73 7.5 9.75 4.3-1.02 7.5-5.11 7.5-9.75V6L12 3Zm-2.25 9 1.5 1.5 3-3" /></svg>
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('PanelErisimAyari:label')}</div>
        <p className="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">{t('PanelErisimAyari:desc')}</p>
        <p className="mt-1 text-xs font-medium text-slate-600 dark:text-slate-300">{t('PanelErisimAyari:current_ip', { ip: istemciIP || '—' })}</p>
        {hata && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}
        {basari && <div className="mt-2 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{basari}</div>}
        <textarea value={deger} onChange={e => setDeger(e.target.value)} rows={4} placeholder={t('PanelErisimAyari:placeholder')}
          className="mt-3 w-full rounded-lg border border-slate-200 bg-white px-3 py-2 font-mono text-xs text-slate-900 focus:border-brand-400 focus:outline-none focus:ring-2 focus:ring-brand-500/20 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" />
        <div className="mt-2 flex items-center justify-between gap-3">
          <span className="text-[11px] text-slate-500">{t('PanelErisimAyari:empty_hint')}</span>
          <button type="button" onClick={() => void kaydet()} disabled={kaydediliyor} className="rounded-lg bg-brand-600 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50">{kaydediliyor ? t('PanelErisimAyari:saving') : t('PanelErisimAyari:save')}</button>
        </div>
        <p className="mt-2 text-[11px] text-slate-500">{t('PanelErisimAyari:recovery_hint')} <code className="rounded bg-slate-100 px-1 dark:bg-slate-800">sudo sanalcp-panel-erisim-kurtar --disable</code></p>
        <div className="mt-4 border-t border-slate-200 pt-3 dark:border-slate-700">
          <div className="text-xs font-semibold text-slate-700 dark:text-slate-200">{t('PanelErisimAyari:temporary_title')}</div>
          <p className="mt-1 text-[11px] text-slate-500">{t('PanelErisimAyari:temporary_desc')}</p>
          {geciciBitis && <p className="mt-1 text-[11px] font-medium text-amber-600 dark:text-amber-300">{t('PanelErisimAyari:temporary_active', { cidr: geciciCIDR, bitis: geciciBitis })}</p>}
          <div className="mt-2 flex flex-col gap-2 sm:flex-row">
            <input value={geciciCIDR} onChange={e => setGeciciCIDR(e.target.value)} placeholder={istemciIP || '203.0.113.10'} className="min-w-0 flex-1 rounded-lg border border-slate-200 bg-white px-3 py-1.5 font-mono text-xs dark:border-slate-700 dark:bg-slate-950" />
            <input type="number" min={5} max={1440} value={dakika} onChange={e => setDakika(Number(e.target.value))} className="w-24 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs dark:border-slate-700 dark:bg-slate-950" aria-label={t('PanelErisimAyari:minutes')} />
            <button type="button" onClick={() => void geciciKaydet()} disabled={kaydediliyor || !geciciCIDR.trim()} className="rounded-lg bg-slate-700 px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50">{t('PanelErisimAyari:temporary_open')}</button>
            {geciciBitis && <button type="button" onClick={() => void geciciKaydet(true)} disabled={kaydediliyor} className="rounded-lg border border-red-200 px-3 py-1.5 text-xs font-medium text-red-600 dark:border-red-800">{t('PanelErisimAyari:temporary_cancel')}</button>}
          </div>
        </div>
      </div>
    </div>
  </div>
}
