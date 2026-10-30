import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import AyarKartDurumu from './AyarKartDurumu'

type Ayar = { acik: boolean; telegram_token_ayarli: boolean; telegram_chat_id: string; eposta: string }

function ayarDogrula(a: Partial<Ayar> | null | undefined): Ayar | null {
  if (!a || typeof a.acik !== 'boolean') return null
  return { acik: a.acik, telegram_token_ayarli: !!a.telegram_token_ayarli, telegram_chat_id: a.telegram_chat_id || '', eposta: a.eposta || '' }
}

export default function UyariBildirimAyari() {
  const { t } = useTranslation(['UyariBildirimAyari', 'common'])
  const [ayar,setAyar]=useState<Ayar|null>(null), [token,setToken]=useState('')
  const [hata,setHata]=useState(''), [basari,setBasari]=useState(''), [yukleniyor,setYukleniyor]=useState(true), [mesgul,setMesgul]=useState(false)
  const yukle = useCallback(async () => {
    setYukleniyor(true); setHata('')
    try {
      const { data } = await api.get<Ayar>('/system/uyari-bildirim'); const gelen = ayarDogrula(data)
      if (!gelen) throw new Error('geçersiz API yanıtı')
      setAyar(gelen); setToken('')
    } catch (e) { setHata(apiHata(e, t('common:load_failed'))) }
    finally { setYukleniyor(false) }
  }, [t])
  useEffect(() => { void yukle() }, [yukle])
  async function kaydet() {
    if (!ayar) return
    setMesgul(true); setHata(''); setBasari('')
    try {
      await api.put('/system/uyari-bildirim', { acik: ayar.acik, telegram_token: token.trim(), telegram_chat_id: ayar.telegram_chat_id, eposta: ayar.eposta })
      setBasari(t('UyariBildirimAyari:saved')); await yukle()
    } catch (e) { setHata(apiHata(e, t('common:save_failed'))) }
    finally { setMesgul(false) }
  }
  async function dene() {
    setMesgul(true); setHata(''); setBasari('')
    try { await api.post('/system/uyari-bildirim/test'); setBasari(t('UyariBildirimAyari:test_sent')) }
    catch (e) { setHata(apiHata(e, t('UyariBildirimAyari:test_failed'))) }
    finally { setMesgul(false) }
  }
  const girdi = 'w-full rounded-lg border p-2 text-xs dark:bg-slate-950'
  return <div className="rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900/60">
    <h3 className="text-sm font-semibold">{t('UyariBildirimAyari:title')}</h3><p className="mt-1 text-xs text-slate-500">{t('UyariBildirimAyari:desc')}</p>
    <AyarKartDurumu yukleniyor={yukleniyor} hata={!ayar?hata:''} tekrar={()=>void yukle()} yukleniyorMetni={t('common:loading')} tekrarMetni={t('common:retry')}/>
    {ayar&&<>
      <label className="mt-3 flex items-center gap-2 text-xs"><input type="checkbox" checked={ayar.acik} onChange={e=>setAyar({...ayar,acik:e.target.checked})}/>{t('UyariBildirimAyari:enabled')}</label>
      <div className="mt-3 grid gap-2 sm:grid-cols-2">
        <input aria-label={t('UyariBildirimAyari:token')} type="password" autoComplete="off" value={token} onChange={e=>setToken(e.target.value)} placeholder={ayar.telegram_token_ayarli?t('UyariBildirimAyari:token_saved'):t('UyariBildirimAyari:token')} className={`${girdi} font-mono`}/>
        <input aria-label={t('UyariBildirimAyari:chat_id')} value={ayar.telegram_chat_id} onChange={e=>setAyar({...ayar,telegram_chat_id:e.target.value})} placeholder={t('UyariBildirimAyari:chat_id')} className={`${girdi} font-mono`}/>
        <input aria-label={t('UyariBildirimAyari:email')} type="email" value={ayar.eposta} onChange={e=>setAyar({...ayar,eposta:e.target.value})} placeholder={t('UyariBildirimAyari:email')} className={`${girdi} sm:col-span-2`}/>
      </div>
      <p className="mt-2 text-xs text-slate-500">{t('UyariBildirimAyari:hint')}</p>
      {hata&&<p role="alert" className="mt-2 text-xs text-red-600">{hata}</p>}{basari&&<p role="status" className="mt-2 text-xs text-emerald-600">{basari}</p>}
      <div className="mt-2 flex flex-wrap gap-2">
        <button type="button" onClick={()=>void kaydet()} disabled={mesgul} className="rounded-lg bg-brand-600 px-3 py-1.5 text-xs text-white disabled:opacity-50">{t('UyariBildirimAyari:save')}</button>
        <button type="button" onClick={()=>void dene()} disabled={mesgul} className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs disabled:opacity-50 dark:border-slate-700">{t('UyariBildirimAyari:test')}</button>
      </div></>}
  </div>
}
