import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import AyarKartDurumu from './AyarKartDurumu'

type Profil = 'kapali' | 'dengeli' | 'siki' | 'ozel'
type Ayar = { profil: Profil; istek_dakika: number; burst: number; ip_istisnalari: string[] }
type Olay = { zaman: string; ip: string; yol: string; durum: number }
type Yanit = { ayar?: Partial<Ayar> | null; olaylar?: Olay[] | null; istemci_ip?: string }
const profiller: Profil[] = ['kapali', 'dengeli', 'siki', 'ozel']

function ayarDogrula(data: Yanit): Ayar | null {
  const a = data?.ayar
  if (!a || !profiller.includes(a.profil as Profil)) return null
  return { profil: a.profil as Profil, istek_dakika: Number(a.istek_dakika) || 0, burst: Number(a.burst) || 0, ip_istisnalari: Array.isArray(a.ip_istisnalari) ? a.ip_istisnalari : [] }
}

export default function PanelHizLimitiAyari() {
  const { t } = useTranslation(['PanelHizLimitiAyari', 'common'])
  const [ayar,setAyar]=useState<Ayar|null>(null), [olaylar,setOlaylar]=useState<Olay[]>([]), [ip,setIP]=useState('')
  const [hata,setHata]=useState(''), [basari,setBasari]=useState(''), [yukleniyor,setYukleniyor]=useState(true), [kaydediliyor,setKaydediliyor]=useState(false)
  const yukle = useCallback(async () => {
    setYukleniyor(true); setHata('')
    try {
      const { data } = await api.get<Yanit>('/system/panel-hiz-limiti'); const gelen = ayarDogrula(data)
      if (!gelen) throw new Error('geçersiz API yanıtı')
      setAyar(gelen); setOlaylar(Array.isArray(data.olaylar) ? data.olaylar : []); setIP(data.istemci_ip || '')
    } catch (e) { setHata(apiHata(e, t('common:load_failed'))) }
    finally { setYukleniyor(false) }
  }, [t])
  useEffect(() => { void yukle() }, [yukle])
  async function kaydet() {
    if (!ayar) return
    setKaydediliyor(true); setHata(''); setBasari('')
    try { await api.put('/system/panel-hiz-limiti',{ayar}); setBasari(t('PanelHizLimitiAyari:saved')); await yukle() }
    catch(e) { setHata(apiHata(e,t('common:save_failed'))) }
    finally { setKaydediliyor(false) }
  }
  return <div className="rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900/60">
    <h3 className="text-sm font-semibold">{t('PanelHizLimitiAyari:title')}</h3><p className="mt-1 text-xs text-slate-500">{t('PanelHizLimitiAyari:desc')}</p>
    <AyarKartDurumu yukleniyor={yukleniyor} hata={!ayar?hata:''} tekrar={()=>void yukle()} yukleniyorMetni={t('common:loading')} tekrarMetni={t('common:retry')}/>
    {ayar&&<><div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">{profiller.map(p=><button type="button" key={p} onClick={()=>setAyar({...ayar,profil:p})} className={`rounded-lg border p-2 text-xs ${ayar.profil===p?'border-brand-500 bg-brand-50 dark:bg-brand-900/20':'border-slate-200 dark:border-slate-700'}`}>{t(`PanelHizLimitiAyari:profiles.${p}`)}</button>)}</div>
      {ayar.profil==='ozel'&&<div className="mt-3 flex gap-2"><input aria-label="request-limit" type="number" value={ayar.istek_dakika} onChange={e=>setAyar({...ayar,istek_dakika:Number(e.target.value)})} className="w-32 rounded-lg border p-2 text-xs dark:bg-slate-950"/><input aria-label="burst-limit" type="number" value={ayar.burst} onChange={e=>setAyar({...ayar,burst:Number(e.target.value)})} className="w-24 rounded-lg border p-2 text-xs dark:bg-slate-950"/></div>}
      <textarea rows={3} value={ayar.ip_istisnalari.join('\n')} onChange={e=>setAyar({...ayar,ip_istisnalari:e.target.value.split(/[\n,]+/).map(x=>x.trim()).filter(Boolean)})} placeholder={`${t('PanelHizLimitiAyari:exceptions')} (${ip})`} className="mt-3 w-full rounded-lg border p-2 font-mono text-xs dark:bg-slate-950"/>
      {hata&&<p className="mt-2 text-xs text-red-600">{hata}</p>}{basari&&<p className="mt-2 text-xs text-emerald-600">{basari}</p>}
      <button type="button" onClick={()=>void kaydet()} disabled={kaydediliyor} className="mt-2 rounded-lg bg-brand-600 px-3 py-1.5 text-xs text-white disabled:opacity-50">{kaydediliyor?t('PanelHizLimitiAyari:saving'):t('PanelHizLimitiAyari:save')}</button>
      {olaylar.length>0&&<details className="mt-3 text-xs"><summary>{t('PanelHizLimitiAyari:events',{count:olaylar.length})}</summary><div className="mt-2 max-h-32 overflow-auto font-mono">{olaylar.map((o,i)=><div key={`${o.zaman}-${i}`}>{o.zaman} · {o.ip} · {o.yol}</div>)}</div></details>}</>}
  </div>
}
