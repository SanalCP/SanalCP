import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Breadcrumb from '@/components/Breadcrumb'
import { api, apiHata } from '@/lib/api'

type Profil='kapali'|'dengeli'|'siki'|'ozel'
type Ayar={profil:Profil;istek_dakika:number;burst:number;bot_engelle:boolean;ip_istisnalari:string[];yol_istisnalari:string[]}
type Olay={zaman:string;ip:string;yol:string;durum:number}
type Yanit={alan_adi:string;ayar:Ayar;olaylar:Olay[]}

export default function DomainRateLimitPage(){
 const {id}=useParams();const {t}=useTranslation(['DomainRateLimitPage','common'])
 const [y,setY]=useState<Yanit|null>(null),[a,setA]=useState<Ayar|null>(null);const [hata,setHata]=useState(''),[ok,setOk]=useState(''),[busy,setBusy]=useState(false)
 function yukle(){api.get<Yanit>(`/domains/${id}/rate-limit`).then(r=>{setY(r.data);setA(r.data.ayar)}).catch(e=>setHata(apiHata(e)))}
 useEffect(yukle,[id])
 async function kaydet(){if(!a)return;setBusy(true);setHata('');setOk('');try{await api.put(`/domains/${id}/rate-limit`,{ayar:a});setOk(t('DomainRateLimitPage:saved'));yukle()}catch(e){setHata(apiHata(e,t('DomainRateLimitPage:save_failed')))}finally{setBusy(false)}}
 const metin=(xs:string[])=>xs.join('\n'), dizi=(s:string)=>s.split(/[\n,]+/).map(x=>x.trim()).filter(Boolean)
 return <div className="w-full px-6 py-5">
  <Breadcrumb items={[{etiket:t('common:home'),href:'/'},{etiket:y?.alan_adi||'…',href:`/abonelikler/${id}`},{etiket:t('DomainRateLimitPage:title')}]}/>
  <h1 className="mb-1 text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('DomainRateLimitPage:title')}</h1>
  <p className="mb-5 text-sm text-slate-500"><Link className="font-medium text-brand-600" to={`/abonelikler/${id}`}>{y?.alan_adi}</Link>{' · '}{t('DomainRateLimitPage:subtitle')}</p>
  {hata&&<div className="mb-3 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{hata}</div>}{ok&&<div className="mb-3 rounded-lg bg-emerald-50 p-3 text-sm text-emerald-700">{ok}</div>}
  {!a?<div className="py-12 text-center text-slate-400">{t('common:loading')}</div>:<div className="grid gap-4 lg:grid-cols-2">
   <section className="rounded-2xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
    <h2 className="mb-3 text-sm font-semibold">{t('DomainRateLimitPage:profile')}</h2>
    <div className="grid grid-cols-2 gap-2">{(['kapali','dengeli','siki','ozel'] as Profil[]).map(p=><button key={p} onClick={()=>setA({...a,profil:p})} className={`rounded-xl border p-3 text-left text-sm ${a.profil===p?'border-brand-500 bg-brand-50 dark:bg-brand-900/20':'border-slate-200 dark:border-slate-700'}`}><strong>{t(`DomainRateLimitPage:profiles.${p}.title`)}</strong><span className="mt-1 block text-xs text-slate-500">{t(`DomainRateLimitPage:profiles.${p}.desc`)}</span></button>)}</div>
    {a.profil==='ozel'&&<div className="mt-4 grid grid-cols-2 gap-3"><label className="text-xs">{t('DomainRateLimitPage:requests')}<input type="number" min={1} max={60000} value={a.istek_dakika} onChange={e=>setA({...a,istek_dakika:Number(e.target.value)})} className="mt-1 w-full rounded-lg border p-2 dark:bg-slate-900"/></label><label className="text-xs">Burst<input type="number" min={0} max={10000} value={a.burst} onChange={e=>setA({...a,burst:Number(e.target.value)})} className="mt-1 w-full rounded-lg border p-2 dark:bg-slate-900"/></label></div>}
    <label className="mt-4 flex items-center gap-2 text-sm"><input type="checkbox" checked={a.bot_engelle} onChange={e=>setA({...a,bot_engelle:e.target.checked})}/>{t('DomainRateLimitPage:block_bots')}</label>
    <button disabled={busy} onClick={()=>void kaydet()} className="mt-5 rounded-lg bg-brand-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">{busy?t('DomainRateLimitPage:saving'):t('DomainRateLimitPage:save')}</button>
   </section>
   <section className="rounded-2xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800"><h2 className="mb-3 text-sm font-semibold">{t('DomainRateLimitPage:exceptions')}</h2>
    <label className="text-xs">{t('DomainRateLimitPage:ip_exceptions')}<textarea rows={5} value={metin(a.ip_istisnalari)} onChange={e=>setA({...a,ip_istisnalari:dizi(e.target.value)})} placeholder="203.0.113.10&#10;192.0.2.0/24" className="mt-1 w-full rounded-lg border p-2 font-mono text-xs dark:bg-slate-900"/></label>
    <label className="mt-3 block text-xs">{t('DomainRateLimitPage:path_exceptions')}<textarea rows={5} value={metin(a.yol_istisnalari)} onChange={e=>setA({...a,yol_istisnalari:dizi(e.target.value)})} placeholder="/webhook/*&#10;/health" className="mt-1 w-full rounded-lg border p-2 font-mono text-xs dark:bg-slate-900"/></label>
   </section>
   <section className="lg:col-span-2 rounded-2xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800"><h2 className="mb-3 text-sm font-semibold">{t('DomainRateLimitPage:events')}</h2>{!y?.olaylar.length?<p className="text-sm text-slate-500">{t('DomainRateLimitPage:no_events')}</p>:<div className="overflow-x-auto"><table className="w-full text-left text-xs"><thead><tr className="text-slate-500"><th className="p-2">{t('DomainRateLimitPage:time')}</th><th>IP</th><th>{t('DomainRateLimitPage:path')}</th><th>{t('DomainRateLimitPage:status')}</th></tr></thead><tbody>{y.olaylar.map((o,i)=><tr key={i} className="border-t border-slate-100 dark:border-slate-700"><td className="p-2">{o.zaman}</td><td>{o.ip}</td><td className="font-mono">{o.yol}</td><td>{o.durum}</td></tr>)}</tbody></table></div>}</section>
  </div>}
 </div>
}
