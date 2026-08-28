import { useCallback, useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Breadcrumb from '@/components/Breadcrumb'
import { api, apiHata } from '@/lib/api'

type Kontrol = { ad: string; ok: boolean; detay: string }
type Durum = { surum: string; php: string; bakim: boolean; modul_toplam: number; modul_aktif: number; cache_mb: number; kontroller: Kontrol[] }
type Loglar = { dosya: string; satirlar: string[] }

export default function DomainPrestaShopPage() {
  const { t } = useTranslation(['DomainPrestaShopPage', 'common'])
  const { id } = useParams()
  const [params, setParams] = useSearchParams()
  const [dizin, setDizin] = useState(params.get('dizin') || 'public_html')
  const [alanAdi, setAlanAdi] = useState('')
  const [durum, setDurum] = useState<Durum | null>(null)
  const [loglar, setLoglar] = useState<Loglar | null>(null)
  const [yuk, setYuk] = useState(true)
  const [mesgul, setMesgul] = useState('')
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)

  useEffect(() => { if (id) api.get<{ alan_adi: string }>(`/domains/${id}`).then(r => setAlanAdi(r.data.alan_adi || '')).catch(() => {}) }, [id])
  const yukle = useCallback(async (hedef = dizin) => {
    if (!id) return
    setYuk(true); setHata(null)
    try {
      const [d, l] = await Promise.all([
        api.get<Durum>(`/domains/${id}/prestashop/durum`, { params: { dizin: hedef } }),
        api.get<Loglar>(`/domains/${id}/prestashop/loglar`, { params: { dizin: hedef, limit: 200 } }),
      ])
      setDurum(d.data); setLoglar(l.data); setParams({ dizin: hedef }, { replace: true })
    } catch (e) { setDurum(null); setLoglar(null); setHata(apiHata(e, t('DomainPrestaShopPage:not_found'))) }
    finally { setYuk(false) }
  }, [id, dizin, setParams, t])
  useEffect(() => { yukle() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  async function calistir(ad: string, fn: () => Promise<unknown>, mesaj: string) {
    setMesgul(ad); setHata(null); setBasari(null)
    try { await fn(); setBasari(mesaj); await yukle() }
    catch (e) { setHata(apiHata(e, t('DomainPrestaShopPage:operation_failed'))) }
    finally { setMesgul('') }
  }
  function bakim() {
    if (!durum || !confirm(t(durum.bakim ? 'DomainPrestaShopPage:confirm_disable' : 'DomainPrestaShopPage:confirm_enable'))) return
    void calistir('bakim', () => api.post(`/domains/${id}/prestashop/bakim`, { dizin, aktif: !durum.bakim }), t('DomainPrestaShopPage:maintenance_changed'))
  }

  return <div className="w-full px-6 py-6">
    <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: alanAdi || t('DomainPrestaShopPage:subscription'), href: `/abonelikler/${id}` }, { etiket: t('DomainPrestaShopPage:title') }]} />
    <div className="mb-6"><h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('DomainPrestaShopPage:title')}</h1><p className="text-sm text-slate-500 mt-1">{t('DomainPrestaShopPage:subtitle')}</p></div>
    <form className="flex gap-2 mb-5 max-w-xl" onSubmit={e => { e.preventDefault(); void yukle(dizin.trim()) }}><input value={dizin} onChange={e => setDizin(e.target.value)} className="ta-input flex-1" placeholder="public_html" /><button className="ta-primary-button" disabled={yuk}>{t('DomainPrestaShopPage:scan')}</button></form>
    {hata && <Mesaj renk="red">{hata}</Mesaj>}{basari && <Mesaj renk="green">{basari}</Mesaj>}
    {yuk ? <div className="text-sm text-slate-400 py-10">{t('common:loading')}</div> : durum && <>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5"><Metrik ad={t('DomainPrestaShopPage:version')} deger={durum.surum || '—'} /><Metrik ad={t('DomainPrestaShopPage:php')} deger={durum.php || '—'} /><Metrik ad={t('DomainPrestaShopPage:modules')} deger={`${durum.modul_aktif} / ${durum.modul_toplam}`} /><Metrik ad={t('DomainPrestaShopPage:cache')} deger={`${durum.cache_mb} MB`} /></div>
      <div className="grid lg:grid-cols-2 gap-5 mb-5">
        <section className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/40 p-5"><h2 className="font-semibold mb-4 dark:text-white">{t('DomainPrestaShopPage:health')}</h2><div className="space-y-3">{durum.kontroller.map(k => <div key={k.ad} className="flex gap-3 text-sm"><span className={k.ok ? 'text-emerald-500' : 'text-red-500'}>●</span><div><div className="font-medium text-slate-700 dark:text-slate-200">{t(`DomainPrestaShopPage:check_${k.ad}`, { defaultValue: k.ad })}</div><div className="text-xs text-slate-400">{k.detay}</div></div></div>)}</div></section>
        <section className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/40 p-5"><h2 className="font-semibold mb-2 dark:text-white">{t('DomainPrestaShopPage:operations')}</h2><p className="text-xs text-slate-400 mb-4">{t('DomainPrestaShopPage:recovery_note')}</p><div className="flex flex-wrap gap-2"><button onClick={bakim} disabled={!!mesgul} className="ta-primary-button">{mesgul === 'bakim' ? '…' : t(durum.bakim ? 'DomainPrestaShopPage:disable_maintenance' : 'DomainPrestaShopPage:enable_maintenance')}</button><button onClick={() => void calistir('cache', () => api.post(`/domains/${id}/prestashop/cache-temizle`, { dizin }), t('DomainPrestaShopPage:cache_cleared'))} disabled={!!mesgul} className="ta-secondary-button">{mesgul === 'cache' ? '…' : t('DomainPrestaShopPage:clear_cache')}</button><button onClick={() => void calistir('backup', () => api.post(`/domains/${id}/prestashop/kurtarma-noktasi`, { dizin }), t('DomainPrestaShopPage:recovery_created'))} disabled={!!mesgul} className="ta-secondary-button">{mesgul === 'backup' ? '…' : t('DomainPrestaShopPage:create_recovery')}</button></div></section>
      </div>
      <section className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/40 overflow-hidden"><div className="px-5 py-4 flex justify-between"><div><h2 className="font-semibold dark:text-white">{t('DomainPrestaShopPage:logs')}</h2><p className="text-xs text-slate-400">{loglar?.dosya || t('DomainPrestaShopPage:no_logs')}</p></div><button className="text-sm text-sky-600" onClick={() => void yukle()}>{t('DomainPrestaShopPage:refresh')}</button></div><pre className="bg-slate-950 text-slate-300 p-5 max-h-96 overflow-auto text-xs whitespace-pre-wrap">{loglar?.satirlar?.join('\n') || t('DomainPrestaShopPage:no_logs')}</pre></section>
    </>}
    <div className="mt-6"><Link to={`/abonelikler/${id}`} className="text-sm text-slate-500 hover:text-slate-800">{t('DomainPrestaShopPage:back')}</Link></div>
  </div>
}

function Metrik({ ad, deger }: { ad: string; deger: string }) { return <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/40 p-4"><div className="text-xs text-slate-400">{ad}</div><div className="text-lg font-semibold mt-1 text-slate-900 dark:text-white">{deger}</div></div> }
function Mesaj({ renk, children }: { renk: 'red' | 'green'; children: React.ReactNode }) { return <div className={`mb-4 px-4 py-3 rounded-xl text-sm ${renk === 'red' ? 'bg-red-50 text-red-600 dark:bg-red-900/20' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20'}`}>{children}</div> }
