import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Breadcrumb from '@/components/Breadcrumb'
import ConfirmDialog from '@/components/ConfirmDialog'
import { api, apiHata } from '@/lib/api'
import { useAuth } from '@/store/auth'

type Zone = { id: string; name: string; status: string }
type Status = { configured: boolean; connected: boolean; domain: string; zone?: Zone }
type Record = { id: string; type: string; name: string; content: string; ttl: number; proxied: boolean; proxiable: boolean; priority?: number }

export default function DomainCloudflarePage() {
  const { id } = useParams(); const { t } = useTranslation(['DomainCloudflarePage','common'])
  const admin = useAuth(s => s.kullanici?.rol === 'admin')
  const [durum,setDurum]=useState<Status|null>(null); const [records,setRecords]=useState<Record[]>([])
  const [token,setToken]=useState(''); const [busy,setBusy]=useState(''); const [hata,setHata]=useState(''); const [ok,setOk]=useState('')
  const [form,setForm]=useState({type:'A',name:'',content:'',ttl:1,proxied:false,priority:0}); const [silinecek,setSilinecek]=useState<Record|null>(null)
  const yukle=useCallback(async()=>{if(!id)return;try{const {data}=await api.get<Status>(`/domains/${id}/cloudflare`);setDurum(data);if(data.connected){const rr=await api.get<Record[]>(`/domains/${id}/cloudflare/records`);setRecords(rr.data)}else setRecords([])}catch(e){setHata(apiHata(e))}},[id])
  useEffect(()=>{void yukle()},[yukle])
  async function islem(ad:string,fn:()=>Promise<unknown>,mesaj:string){setBusy(ad);setHata('');setOk('');try{await fn();setOk(mesaj);await yukle()}catch(e){setHata(apiHata(e))}finally{setBusy('')}}
  const tokenKaydet=()=>islem('token',()=>api.put('/cloudflare/token',{token}),t('token_saved'))
  const bagla=()=>islem('connect',()=>api.post(`/domains/${id}/cloudflare/connect`),t('connected'))
  const purge=()=>islem('purge',()=>api.post(`/domains/${id}/cloudflare/purge`),t('purged'))
  const ekle=()=>islem('create',()=>api.post(`/domains/${id}/cloudflare/records`,form),t('record_created'))
  const proxy=(r:Record)=>islem('proxy-'+r.id,()=>api.put(`/domains/${id}/cloudflare/records/${r.id}`,{type:r.type,name:r.name,content:r.content,ttl:r.ttl,priority:r.priority||0,proxied:!r.proxied}),t('record_updated'))
  const sil=()=>{if(!silinecek)return;const r=silinecek;setSilinecek(null);void islem('delete',()=>api.delete(`/domains/${id}/cloudflare/records/${r.id}`),t('record_deleted'))}
  return <div className="w-full px-6 py-5">
    <Breadcrumb items={[{etiket:t('common:home'),href:'/'},{etiket:t('domains'),href:'/domainler'},{etiket:durum?.domain||'...',href:`/abonelikler/${id}`},{etiket:'Cloudflare'}]}/>
    <div className="flex flex-wrap items-center justify-between gap-3 mb-5"><div><h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Cloudflare</h1><p className="text-sm text-slate-500">{t('subtitle')}</p></div>{durum?.connected&&<button onClick={purge} disabled={!!busy} className="ta-secondary-button">{busy==='purge'?t('common:loading'):t('purge')}</button>}</div>
    {hata&&<div className="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{hata}</div>}{ok&&<div className="mb-4 rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">{ok}</div>}
    {durum&&!durum.configured&&<div className="rounded-xl border border-amber-200 bg-amber-50 p-5 mb-5"><h2 className="font-semibold text-amber-900">{t('token_required')}</h2><p className="text-sm text-amber-800 my-2">{admin?t('token_help'):t('token_admin')}</p>{admin&&<div className="flex gap-2"><input type="password" value={token} onChange={e=>setToken(e.target.value)} className="ta-input flex-1" placeholder="Cloudflare API Token"/><button onClick={tokenKaydet} disabled={busy==='token'||token.length<20} className="ta-primary-button">{t('save')}</button></div>}</div>}
    {durum?.configured&&!durum.connected&&<div className="rounded-xl border border-sky-200 bg-sky-50 p-5"><h2 className="font-semibold text-sky-900">{t('not_connected')}</h2><p className="text-sm text-sky-800 my-2">{t('connect_help',{domain:durum.domain})}</p><button onClick={bagla} disabled={!!busy} className="ta-primary-button">{t('connect')}</button></div>}
    {durum?.connected&&<><div className="rounded-xl border border-slate-200 dark:border-slate-800 p-4 mb-5 flex justify-between"><div><div className="font-medium text-slate-900 dark:text-slate-100">{durum.zone?.name}</div><div className="text-xs text-slate-500">Zone: {durum.zone?.status}</div></div><span className="text-emerald-600 text-sm">● {t('active')}</span></div>
      <form onSubmit={e=>{e.preventDefault();void ekle()}} className="grid grid-cols-1 md:grid-cols-6 gap-2 mb-5 rounded-xl border border-slate-200 dark:border-slate-800 p-4">
        <select className="ta-input" value={form.type} onChange={e=>setForm({...form,type:e.target.value})}>{['A','AAAA','CNAME','MX','TXT','CAA'].map(x=><option key={x}>{x}</option>)}</select>
        <input required className="ta-input md:col-span-2" placeholder={t('name')} value={form.name} onChange={e=>setForm({...form,name:e.target.value})}/><input required className="ta-input md:col-span-2" placeholder={t('content')} value={form.content} onChange={e=>setForm({...form,content:e.target.value})}/><button className="ta-primary-button" disabled={!!busy}>{t('add')}</button>
        {['A','AAAA','CNAME'].includes(form.type)&&<label className="text-sm text-slate-600 md:col-span-6"><input type="checkbox" checked={form.proxied} onChange={e=>setForm({...form,proxied:e.target.checked})} className="mr-2"/>{t('proxy')}</label>}
      </form>
      <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800"><table className="w-full text-sm"><thead className="bg-slate-50 dark:bg-slate-900"><tr>{['type','name','content','proxy','actions'].map(k=><th key={k} className="px-3 py-2 text-left">{t(k)}</th>)}</tr></thead><tbody>{records.map(r=><tr key={r.id} className="border-t border-slate-100 dark:border-slate-800"><td className="px-3 py-2 font-mono">{r.type}</td><td className="px-3 py-2">{r.name}</td><td className="px-3 py-2 max-w-md break-all">{r.content}</td><td className="px-3 py-2">{r.proxiable?<button onClick={()=>void proxy(r)} disabled={!!busy} className={r.proxied?'text-orange-600':'text-slate-500'}>{r.proxied?t('proxied'):t('dns_only')}</button>:'—'}</td><td className="px-3 py-2"><button onClick={()=>setSilinecek(r)} className="text-red-600">{t('delete')}</button></td></tr>)}</tbody></table></div>
    </>}
    <div className="mt-5"><Link to={`/abonelikler/${id}/dns`} className="text-sm text-brand-600">← {t('local_dns')}</Link></div>
    <ConfirmDialog acik={!!silinecek} baslik={t('delete')} mesaj={t('delete_confirm')} onOnay={sil} onIptal={()=>setSilinecek(null)}/>
  </div>
}
