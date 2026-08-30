import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type Ozet = { aktif: number; basarisiz: number; bildirim: number; kritik: number }
type Islem = { anahtar:string; tur:string; baslik:string; aciklama:string; durum:'bekliyor'|'calisiyor'|'basarili'|'basarisiz'|'geri_alindi'; ilerleme:number; mesaj:string; yol:string; baslangic:string; bitis:string }
const BOS: Ozet = { aktif: 0, basarisiz: 0, bildirim: 0, kritik: 0 }

function renk(d: Islem['durum']) {
  if (d === 'basarili') return 'bg-emerald-500'
  if (d === 'basarisiz') return 'bg-red-500'
  if (d === 'geri_alindi') return 'bg-amber-500'
  return 'bg-blue-500'
}

export default function IslemMerkezi({ admin }: { admin: boolean }) {
  const { t } = useTranslation('TopBar'); const navigate = useNavigate()
  const [acik,setAcik]=useState(false), [ozet,setOzet]=useState<Ozet>(BOS)
  const [islemler,setIslemler]=useState<Islem[]>([]), [yukleniyor,setYukleniyor]=useState(false), [hata,setHata]=useState('')
  const ozetYukle=useCallback(()=>{if(admin)api.get<Ozet>('/islemler/ozet').then(r=>setOzet(r.data)).catch(()=>{})},[admin])
  const listeYukle=useCallback(async()=>{if(!admin)return;setYukleniyor(true);setHata('');try{const{data}=await api.get<Islem[]>('/islemler');setIslemler(Array.isArray(data)?data:[])}catch(e){setHata(apiHata(e,t('operations.load_failed')))}finally{setYukleniyor(false)}},[admin,t])
  useEffect(()=>{if(!admin)return;ozetYukle();const timer=window.setInterval(ozetYukle,ozet.aktif?10000:60000);return()=>clearInterval(timer)},[admin,ozet.aktif,ozetYukle])
  useEffect(()=>{if(!acik||!admin)return;void listeYukle();if(!ozet.aktif)return;const timer=window.setInterval(()=>{void listeYukle();ozetYukle()},5000);return()=>clearInterval(timer)},[acik,admin,ozet.aktif,listeYukle,ozetYukle])
  if(!admin)return null
  const rozet=ozet.aktif+ozet.basarisiz+ozet.bildirim
  const git=(yol:string)=>{setAcik(false);navigate(yol)}
  return <div className="relative">
    <button onClick={()=>setAcik(v=>!v)} type="button" aria-label={t('operations.title')} aria-expanded={acik} title={t('operations.title')} className="relative inline-flex rounded-md p-2 text-slate-500 transition hover:bg-slate-100 hover:text-slate-700 dark:bg-slate-800 dark:text-slate-400 dark:hover:bg-slate-700">
      <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}><path strokeLinecap="round" d="M4 6h16M4 12h16M4 18h10"/><circle cx="19" cy="18" r="2.5"/></svg>
      {rozet>0&&<span className={`absolute -right-1 -top-1 min-w-4 rounded-full px-1 text-center text-[10px] font-bold text-white ${ozet.kritik||ozet.basarisiz?'bg-red-600':ozet.aktif?'bg-blue-600':'bg-amber-500'}`}>{rozet>99?'99+':rozet}</span>}
    </button>
    {acik&&<><button type="button" aria-label={t('operations.close')} className="fixed inset-0 z-40 cursor-default" onClick={()=>setAcik(false)}/><section className="fixed right-2 top-20 z-50 mt-1 max-h-[min(75vh,40rem)] w-[calc(100vw-1rem)] overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900 sm:absolute sm:right-0 sm:top-full sm:w-[28rem]">
      <header className="flex items-center justify-between border-b border-slate-100 px-4 py-3 dark:border-slate-800"><div><h2 className="text-sm font-semibold dark:text-white">{t('operations.title')}</h2><p className="text-xs text-slate-500">{t('operations.subtitle')}</p></div><button onClick={()=>void listeYukle()} disabled={yukleniyor} className="rounded-lg px-2 py-1 text-xs text-brand-600 hover:bg-brand-50 disabled:opacity-50">{t('operations.refresh')}</button></header>
      <div className="grid grid-cols-3 gap-2 border-b border-slate-100 p-3 dark:border-slate-800"><OzetKart deger={ozet.aktif} etiket={t('operations.active')} sinif="text-blue-600"/><OzetKart deger={ozet.basarisiz} etiket={t('operations.failed_24h')} sinif="text-red-600"/><button onClick={()=>git('/guvenlik-bildirimleri')} className="rounded-xl bg-slate-50 p-2 text-left hover:bg-slate-100 dark:bg-slate-800/70"><strong className={`block text-lg ${ozet.kritik?'text-red-600':'text-amber-600'}`}>{ozet.bildirim}</strong><span className="text-[11px] text-slate-500">{t('operations.alerts')}</span></button></div>
      <div className="max-h-[calc(min(75vh,40rem)-9.5rem)] overflow-y-auto p-2">
        {hata&&<div className="m-2 rounded-lg bg-red-50 p-3 text-xs text-red-700">{hata} <button onClick={()=>void listeYukle()} className="underline">{t('operations.retry')}</button></div>}
        {!hata&&yukleniyor&&!islemler.length&&<div className="p-8 text-center text-sm text-slate-500">{t('operations.loading')}</div>}
        {!hata&&!yukleniyor&&!islemler.length&&<div className="p-8 text-center text-sm text-slate-500">{t('operations.empty')}</div>}
        {islemler.map(x=><button key={x.anahtar} onClick={()=>git(x.yol)} className="block w-full rounded-xl p-3 text-left hover:bg-slate-50 dark:hover:bg-slate-800/70"><div className="flex items-start gap-3"><span className={`mt-1.5 h-2.5 w-2.5 flex-none rounded-full ${renk(x.durum)} ${x.durum==='calisiyor'?'animate-pulse':''}`}/><span className="min-w-0 flex-1"><span className="flex justify-between gap-2"><strong className="truncate text-sm font-medium dark:text-slate-100">{x.baslik}</strong><span className="flex-none text-[10px] uppercase text-slate-400">{t(`operations.states.${x.durum}`)}</span></span><span className="block truncate text-xs text-slate-500">{x.aciklama}</span>{['calisiyor','bekliyor'].includes(x.durum)&&<span className="mt-2 block h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-700"><span className="block h-full bg-brand-600" style={{width:`${Math.max(2,Math.min(100,x.ilerleme))}%`}}/></span>}{x.mesaj&&<span className={`mt-1 block truncate text-xs ${x.durum==='basarisiz'?'text-red-600':'text-slate-500'}`}>{x.mesaj}</span>}<span className="mt-1 block text-[10px] text-slate-400">{x.bitis||x.baslangic}</span></span></div></button>)}
      </div>
    </section></>}
  </div>
}

function OzetKart({deger,etiket,sinif}:{deger:number;etiket:string;sinif:string}){return <div className="rounded-xl bg-slate-50 p-2 dark:bg-slate-800/70"><strong className={`block text-lg ${sinif}`}>{deger}</strong><span className="text-[11px] text-slate-500">{etiket}</span></div>}
