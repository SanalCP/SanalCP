import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Kurulu = {
  tur: string; ad: string; dizin: string; surum: string; son_surum: string
  durum: 'guncel' | 'eski' | 'bilinmiyor'; kurulum_tarihi: string
  site_url: string; admin_url: string
}
type FormAlan = { anahtar: string; etiket: string; tur: 'text' | 'email' | 'password'; zorunlu: boolean; yer_tutucu?: string }
type TurBilgi = { slug: string; ad: string; form_alanlari: FormAlan[] }
type Sonuc = {
  tur: string; site_url: string; admin_url: string
  admin_kullanici: string; admin_parola: string; surum: string; ekstra?: Record<string, string>
}

const ICONS: Record<string, string> = {
  wordpress: 'M12 21a9 9 0 100-18 9 9 0 000 18zm0 0c2.5-2.5 3-6 3-9s-.5-6.5-3-9m0 18c-2.5-2.5-3-6-3-9s.5-6.5 3-9M3.6 9h16.8M3.6 15h16.8',
  prestashop: 'M12 3l7 4v10l-7 4-7-4V7l7-4z',
}

export default function DomainAppsPage() {
  const { t } = useTranslation(['DomainAppsPage', 'common'])
  const { id } = useParams()
  const navigate = useNavigate()
  const [alanAdi, setAlanAdi] = useState('')
  const [liste, setListe] = useState<Kurulu[]>([])
  const [yuk, setYuk] = useState(true)
  const [turler, setTurler] = useState<TurBilgi[]>([])
  const [hata, setHata] = useState<string | null>(null)
  const [mesgul, setMesgul] = useState<string | null>(null)

  const [sihirbazAcik, setSihirbazAcik] = useState(false)
  const [seciliTur, setSeciliTur] = useState<TurBilgi | null>(null)
  const [altDizin, setAltDizin] = useState('')
  const [alanlar, setAlanlar] = useState<Record<string, string>>({})
  const [kuruyor, setKuruyor] = useState(false)
  const [sonuc, setSonuc] = useState<Sonuc | null>(null)

  useEffect(() => {
    if (!id) return
    api.get<{ alan_adi: string }>(`/domains/${id}`).then(r => setAlanAdi(r.data.alan_adi || '')).catch(() => {})
    api.get<TurBilgi[]>(`/domains/${id}/apps/turler`).then(r => setTurler(r.data || [])).catch(() => {})
  }, [id])

  const listele = useCallback(() => {
    if (!id) return
    setYuk(true)
    api.get<Kurulu[]>(`/domains/${id}/apps`).then(r => setListe(r.data || [])).catch(() => setListe([])).finally(() => setYuk(false))
  }, [id])
  useEffect(() => { listele() }, [listele])

  function turSec(tb: TurBilgi) {
    setSeciliTur(tb)
    const bos: Record<string, string> = {}
    tb.form_alanlari.forEach(fa => { bos[fa.anahtar] = '' })
    setAlanlar(bos)
    setAltDizin('')
    setHata(null)
  }

  async function kur(e: React.FormEvent) {
    e.preventDefault()
    if (!seciliTur) return
    setHata(null); setSonuc(null); setKuruyor(true)
    try {
      const { data } = await api.post<Sonuc>(`/domains/${id}/apps/${seciliTur.slug}/kur`, {
        alt_dizin: altDizin.trim(), alanlar,
      })
      setSonuc(data); setSihirbazAcik(false); setSeciliTur(null)
      listele()
    } catch (err) { setHata(apiHata(err, t('DomainAppsPage:install_failed'))) }
    finally { setKuruyor(false) }
  }

  async function guncelle(k: Kurulu) {
    const key = k.tur + k.dizin
    setMesgul(key); setHata(null)
    try { await api.post(`/domains/${id}/apps/${k.tur}/guncelle`, { dizin: k.dizin }); listele() }
    catch (err) { setHata(apiHata(err, t('DomainAppsPage:update_failed'))) }
    finally { setMesgul(null) }
  }

  async function sil(k: Kurulu) {
    if (!confirm(t('DomainAppsPage:confirm_delete', { ad: k.ad, yol: k.dizin }))) return
    const key = k.tur + k.dizin
    setMesgul(key); setHata(null)
    try {
      await api.delete(`/domains/${id}/apps/${k.tur}`, { data: { dizin: k.dizin, db_sil: true } })
      listele()
    } catch (err) { setHata(apiHata(err, t('DomainAppsPage:delete_failed'))) }
    finally { setMesgul(null) }
  }

  function yonetHedefi(k: Kurulu): string | null {
    if (k.tur === 'wordpress') return `/abonelikler/${id}/wordpress`
    return null
  }

  return (
    <div className="w-full px-6 py-6">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: alanAdi || t('DomainAppsPage:breadcrumb_subscription'), href: `/abonelikler/${id}` },
        { etiket: t('DomainAppsPage:breadcrumb_apps') },
      ]} />
      <div className="flex items-center justify-between gap-4 mb-6 flex-wrap">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('DomainAppsPage:title')}</h1>
        <button onClick={() => { setSihirbazAcik(true); setSeciliTur(null) }} className="ta-primary-button">
          {t('DomainAppsPage:new_install_button')}
        </button>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {sonuc && (
        <div className="mb-4 rounded-2xl border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/15 p-4">
          <div className="text-sm font-semibold text-emerald-700 dark:text-emerald-300 mb-2">
            {t('DomainAppsPage:installed_ok', { version: sonuc.surum })}
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_site')}</span> <a href={sonuc.site_url} target="_blank" rel="noreferrer" className="text-brand-600 dark:text-brand-400 hover:underline font-mono text-xs">{sonuc.site_url}</a></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_admin')}</span> <a href={sonuc.admin_url} target="_blank" rel="noreferrer" className="text-brand-600 dark:text-brand-400 hover:underline font-mono text-xs">{sonuc.admin_url}</a></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_user')}</span> <span className="font-mono text-xs">{sonuc.admin_kullanici}</span></div>
            <div><span className="text-[11px] uppercase text-slate-400 font-semibold">{t('DomainAppsPage:result_password')}</span> <span className="font-mono text-xs">{sonuc.admin_parola}</span></div>
          </div>
          <p className="text-[11px] text-amber-700 dark:text-amber-400 mt-2">{t('DomainAppsPage:password_warning')}</p>
        </div>
      )}

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden mb-6">
        <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('DomainAppsPage:installed_title')}</h3>
        </div>
        {yuk ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500 dark:text-slate-400">{t('DomainAppsPage:scanning')}</div>
        ) : liste.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500 dark:text-slate-400">{t('DomainAppsPage:no_installations')}</div>
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-700/60">
            {liste.map(k => {
              const key = k.tur + k.dizin
              const yonet = yonetHedefi(k)
              return (
                <div key={key} className="flex items-center justify-between gap-4 px-4 py-3 flex-wrap">
                  <div className="flex items-center gap-3 min-w-0">
                    <svg viewBox="0 0 24 24" className="w-6 h-6 text-slate-400 shrink-0" fill="none" stroke="currentColor" strokeWidth={1.5}><path d={ICONS[k.tur] || ICONS.wordpress} /></svg>
                    <div className="min-w-0">
                      <div className="font-medium text-slate-800 dark:text-slate-100">{k.ad} <span className="text-xs text-slate-400 font-mono">{k.dizin}</span></div>
                      <div className="text-xs text-slate-500 dark:text-slate-400">
                        {t('DomainAppsPage:version_label')} {k.surum || '—'}
                        {k.durum === 'eski' && <span className="text-amber-600 dark:text-amber-400 font-medium ml-1">{t('DomainAppsPage:status_update_to', { version: k.son_surum })}</span>}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <a href={k.admin_url} target="_blank" rel="noreferrer" className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('DomainAppsPage:admin_link')}</a>
                    {yonet && <button onClick={() => navigate(yonet)} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('DomainAppsPage:manage_link')}</button>}
                    <button disabled={!!mesgul} onClick={() => guncelle(k)} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">
                      {mesgul === key ? '…' : t('DomainAppsPage:update_link')}
                    </button>
                    <button disabled={!!mesgul} onClick={() => sil(k)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">{t('DomainAppsPage:delete')}</button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {sihirbazAcik && !seciliTur && (
        <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4">
          <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">{t('DomainAppsPage:pick_app_title')}</h3>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {turler.map(tb => (
              <button key={tb.slug} onClick={() => turSec(tb)} className="flex flex-col items-center gap-2 p-4 border border-slate-200 dark:border-slate-700 rounded-xl hover:border-brand-400 dark:hover:border-brand-500 hover:bg-slate-50 dark:hover:bg-slate-800">
                <svg viewBox="0 0 24 24" className="w-8 h-8 text-slate-500" fill="none" stroke="currentColor" strokeWidth={1.5}><path d={ICONS[tb.slug] || ICONS.wordpress} /></svg>
                <span className="text-sm font-medium text-slate-700 dark:text-slate-200">{tb.ad}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {sihirbazAcik && seciliTur && (
        <form onSubmit={kur} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 max-w-2xl">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{t('DomainAppsPage:install_title', { ad: seciliTur.ad })}</h3>
            <button type="button" onClick={() => setSeciliTur(null)} className="text-xs text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">{t('DomainAppsPage:pick_different')}</button>
          </div>
          <div className="mb-3">
            <label className="ta-label-sm">{t('DomainAppsPage:subdir_label')}</label>
            <input value={altDizin} onChange={e => setAltDizin(e.target.value)} placeholder={t('DomainAppsPage:subdir_placeholder')} className="ta-input ta-input-sm w-full font-mono" />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {seciliTur.form_alanlari.map(fa => (
              <label key={fa.anahtar} className="block">
                <span className="ta-label-sm">{fa.etiket}</span>
                <input
                  value={alanlar[fa.anahtar] || ''}
                  onChange={e => setAlanlar(a => ({ ...a, [fa.anahtar]: e.target.value }))}
                  required={fa.zorunlu}
                  placeholder={fa.yer_tutucu}
                  type={fa.tur === 'password' ? 'password' : fa.tur === 'email' ? 'email' : 'text'}
                  className="ta-input ta-input-sm w-full"
                />
              </label>
            ))}
          </div>
          <button disabled={kuruyor} className="ta-primary-button mt-3 w-full sm:w-auto">
            {kuruyor ? t('DomainAppsPage:installing_button') : t('DomainAppsPage:install_button')}
          </button>
        </form>
      )}
    </div>
  )
}
