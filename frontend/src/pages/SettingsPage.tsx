import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { useAuth } from '@/store/auth'
import { setLang, type Lang } from '@/i18n'

type Ben = {
  id: number; adi: string; rol: string; eposta: string; ad_soyad: string
  durum: string; iki_fa: boolean; tercih_tema: string; tercih_dil: string
}

type ApiToken = {
  id: number; ad: string; onek: string; aktif: boolean
  bitis_at: string; son_kullanim_at: string; son_kullanim_ip: string; created_at: string
}

function Kart({ baslik, aciklama, ikon, cocuk }: { baslik: string; aciklama?: string; ikon: React.ReactNode; cocuk: React.ReactNode }) {
  return (
    <section className="ta-card p-6">
      <div className="flex items-start gap-3 mb-5">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-brand-100 bg-brand-50 text-brand-600 dark:border-brand-900/50 dark:bg-brand-900/20 dark:text-brand-400">{ikon}</div>
        <div>
          <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{baslik}</h2>
          {aciklama && <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">{aciklama}</p>}
        </div>
      </div>
      {cocuk}
    </section>
  )
}

function Girdi({ etiket, ...p }: { etiket: string } & React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="block">
      <span className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{etiket}</span>
      <input {...p} className="ta-input disabled:cursor-not-allowed disabled:opacity-60" />
    </label>
  )
}

function Uyari({ tip, mesaj }: { tip: 'ok' | 'err'; mesaj: string }) {
  if (!mesaj) return null
  const c = tip === 'ok'
    ? 'bg-emerald-50 dark:bg-emerald-900/20 border-emerald-200 dark:border-emerald-800 text-emerald-700 dark:text-emerald-300'
    : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-700 dark:text-red-300'
  return <div className={`text-sm px-3 py-2 rounded-lg border ${c}`}>{mesaj}</div>
}

export default function SettingsPage() {
  const { t } = useTranslation(['SettingsPage', 'common'])
  const guncelleAd = useAuth((s) => s.guncelleAd)

  // Nameserver: admin panel genelini, bayi kendi white-label çiftini yönetir.
  // Müşteriler bunu Bağlantı Bilgisi ekranında yalnız görüntüler.
  const [ns, setNS] = useState<{ ns1: string; ns2: string; kaynak?: string; uyari?: string; oneri1?: string; oneri2?: string } | null>(null)
  const [ns1, setNS1] = useState('')
  const [ns2, setNS2] = useState('')
  const [nsYuk, setNSYuk] = useState(false)
  const [nsOk, setNSOk] = useState('')
  const [nsErr, setNSErr] = useState('')
  const [ben, setBen] = useState<Ben | null>(null)
  const [yukHata, setYukHata] = useState('')

  const [ad, setAd] = useState(''); const [eposta, setEposta] = useState('')
  const [pOk, setPOk] = useState(''); const [pErr, setPErr] = useState(''); const [pYuk, setPYuk] = useState(false)

  const [mevcut, setMevcut] = useState(''); const [yeni, setYeni] = useState(''); const [yeni2, setYeni2] = useState('')
  const [paOk, setPaOk] = useState(''); const [paErr, setPaErr] = useState(''); const [paYuk, setPaYuk] = useState(false)

  const [f2Kur, setF2Kur] = useState<{ secret: string; otpauth: string; otpauth_uri?: string; qr_data_uri?: string } | null>(null)
  const [f2Kod, setF2Kod] = useState(''); const [f2Err, setF2Err] = useState(''); const [f2Yuk, setF2Yuk] = useState(false)
  const [f2Kapat, setF2Kapat] = useState(false); const [kapatKod, setKapatKod] = useState('')

  const [tema, setTema] = useState('system'); const [dil, setDil] = useState('tr')
  const [tOk, setTOk] = useState(''); const [tYuk, setTYuk] = useState(false)

  // API token'ları. Ham token yalnız oluşturma yanıtında döner ve bir daha
  // elde edilemez — bu yüzden yeniUretilen ayrı state'te tutulup kullanıcı
  // kapatana kadar ekranda bırakılır.
  const [tokenlar, setTokenlar] = useState<ApiToken[] | null>(null)
  const [tkAd, setTkAd] = useState(''); const [tkGun, setTkGun] = useState('0')
  const [tkYuk, setTkYuk] = useState(false); const [tkErr, setTkErr] = useState('')
  const [yeniUretilen, setYeniUretilen] = useState<{ ad: string; token: string } | null>(null)
  const [kopyalandi, setKopyalandi] = useState(false)

  function tokenlariYukle() {
    api.get<{ tokenlar: ApiToken[] }>('/me/api-tokenlari')
      .then(r => setTokenlar(r.data.tokenlar || []))
      .catch(() => setTokenlar([]))
  }

  async function tokenOlustur(e: React.FormEvent) {
    e.preventDefault()
    setTkErr(''); setTkYuk(true); setKopyalandi(false)
    try {
      const { data } = await api.post('/me/api-tokenlari', {
        ad: tkAd.trim(), gun_sonra: parseInt(tkGun, 10) || 0,
      })
      setYeniUretilen({ ad: data.ad, token: data.token })
      setTkAd('')
      tokenlariYukle()
    } catch (err) { setTkErr(apiHata(err, t('SettingsPage:apitoken.create_failed'))) }
    finally { setTkYuk(false) }
  }

  async function tokenSil(tk: ApiToken) {
    if (!confirm(t('SettingsPage:apitoken.confirm_delete', { ad: tk.ad }))) return
    setTkErr('')
    try { await api.delete(`/me/api-tokenlari/${tk.id}`); tokenlariYukle() }
    catch (err) { setTkErr(apiHata(err, t('SettingsPage:apitoken.delete_failed'))) }
  }

  function yukle() {
    api.get<Ben>('/me').then(r => {
      setBen(r.data); setAd(r.data.ad_soyad || ''); setEposta(r.data.eposta || '')
      setTema(r.data.tercih_tema || 'system'); setDil(r.data.tercih_dil || 'tr')
    }).catch(e => setYukHata(apiHata(e)))
  }
  useEffect(yukle, [])
  useEffect(tokenlariYukle, [])

  async function profilKaydet(e: React.FormEvent) {
    e.preventDefault(); setPOk(''); setPErr(''); setPYuk(true)
    try {
      await api.put('/me', { ad_soyad: ad, eposta })
      guncelleAd(ad) // sağ üst bar dinamik güncellensin
      setPOk(t('SettingsPage:account.saved')); setTimeout(() => setPOk(''), 3000); yukle()
    } catch (e) { setPErr(apiHata(e, t('SettingsPage:account.save_failed'))) } finally { setPYuk(false) }
  }

  async function parolaDegistir(e: React.FormEvent) {
    e.preventDefault(); setPaOk(''); setPaErr('')
    if (yeni.length < 8) { setPaErr(t('SettingsPage:password.too_short')); return }
    if (yeni !== yeni2) { setPaErr(t('SettingsPage:password.mismatch')); return }
    setPaYuk(true)
    try {
      await api.post('/me/parola', { mevcut, yeni })
      setPaOk(t('SettingsPage:password.changed'))
      setMevcut(''); setYeni(''); setYeni2(''); setTimeout(() => setPaOk(''), 5000)
    } catch (e) { setPaErr(apiHata(e, t('SettingsPage:password.change_failed'))) } finally { setPaYuk(false) }
  }

  async function f2Baslat() {
    setF2Err(''); setF2Kod('')
    try { const r = await api.get<{ secret: string; otpauth: string; otpauth_uri?: string; qr_data_uri?: string }>('/me/2fa/setup'); setF2Kur(r.data) }
    catch (e) { setF2Err(apiHata(e)) }
  }
  async function f2Etkinlestir(e: React.FormEvent) {
    e.preventDefault(); setF2Err(''); setF2Yuk(true)
    try {
      await api.post('/me/2fa/enable', { secret: f2Kur!.secret, kod: f2Kod })
      setF2Kur(null); setF2Kod(''); yukle()
    } catch (e) { setF2Err(apiHata(e, t('SettingsPage:twofa.verify_failed'))) } finally { setF2Yuk(false) }
  }
  async function f2KapatOnay(e: React.FormEvent) {
    e.preventDefault(); setF2Err(''); setF2Yuk(true)
    try { await api.post('/me/2fa/disable', { kod: kapatKod }); setF2Kapat(false); setKapatKod(''); yukle() }
    catch (e) { setF2Err(apiHata(e, t('SettingsPage:twofa.verify_failed'))) } finally { setF2Yuk(false) }
  }

  async function tercihKaydet() {
    setTOk(''); setTYuk(true)
    try {
      await api.put('/me', { ad_soyad: ad, eposta, tercih_tema: tema, tercih_dil: dil })
      try { localStorage.setItem('sanal.tema', tema) } catch { /* yoksay */ }
      setLang(dil as Lang)
      setTOk(t('common:saved')); setTimeout(() => setTOk(''), 3000)
    } catch { setTOk('') } finally { setTYuk(false) }
  }

  const bayiMi = ben?.rol === 'reseller'
  const nsUcu = '/bayi/nameserver'

  useEffect(() => {
    if (!bayiMi) return
    api.get(nsUcu).then(r => {
      setNS(r.data)
      // Ayarlı değilse öneriyi ALANA yazar ama KAYDETMEZ — admin görüp
      // onaylamadan hiçbir zone'a yazılmaz (yanlış NS = çözülemeyen domain).
      setNS1(r.data.ns1 || r.data.oneri1 || '')
      setNS2(r.data.ns2 || r.data.oneri2 || '')
    }).catch(() => { /* yetkisizse kart gizli kalır */ })
  }, [bayiMi, nsUcu])

  async function nsKaydet() {
    setNSYuk(true); setNSOk(''); setNSErr('')
    try {
      const { data } = await api.put(nsUcu, { ns1: ns1.trim(), ns2: ns2.trim() })
      setNS(data)
      setNSOk(data.uyari || t('SettingsPage:nameserver.saved'))
    } catch (e) { setNSErr(apiHata(e)) } finally { setNSYuk(false) }
  }


  const btn = 'ta-primary-button disabled:cursor-not-allowed disabled:opacity-50'
  const secretGruplu = f2Kur ? (f2Kur.secret.match(/.{1,4}/g) || []).join(' ') : ''

  return (
    <div className="ta-page">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('SettingsPage:title') }]} />
      <p className="ta-eyebrow">SanalCP</p>
      <h1 className="ta-page-title">{t('SettingsPage:title')}</h1>
      <p className="ta-page-description mb-7">{t('SettingsPage:subtitle')}</p>
      {yukHata && <div className="mb-4"><Uyari tip="err" mesaj={yukHata} /></div>}

      <div className="space-y-5">
        {/* 1) Hesap Bilgileri */}
        <Kart baslik={t('SettingsPage:account.title')} aciklama={t('SettingsPage:account.desc')}
          ikon={<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>}
          cocuk={
            <form onSubmit={profilKaydet} className="space-y-4">
              <div className="grid sm:grid-cols-2 gap-4">
                <Girdi etiket={t('SettingsPage:account.username')} value={ben?.adi || 'root'} disabled />
                <div>
                  <span className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('SettingsPage:account.role_status')}</span>
                  <div className="flex gap-2 pt-1.5">
                    <span className="text-[11px] uppercase tracking-wider px-2 py-1 rounded bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 font-semibold">{ben?.rol || 'admin'}</span>
                    <span className="text-[11px] uppercase tracking-wider px-2 py-1 rounded bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 font-semibold">{ben?.durum || 'active'}</span>
                  </div>
                </div>
                <Girdi etiket={t('SettingsPage:account.full_name')} value={ad} onChange={e => setAd(e.target.value)} placeholder={t('SettingsPage:account.full_name_placeholder')} />
                <Girdi etiket={t('SettingsPage:account.email')} type="email" value={eposta} onChange={e => setEposta(e.target.value)} placeholder={t('SettingsPage:account.email_placeholder')} />
              </div>
              <div className="flex items-center gap-3 flex-wrap">
                <button type="submit" disabled={pYuk} className={btn}>{pYuk ? t('common:saving') : t('common:save')}</button>
                <Uyari tip="ok" mesaj={pOk} /><Uyari tip="err" mesaj={pErr} />
              </div>
            </form>
          } />

        {/* 2) Parola */}
        <Kart baslik={t('SettingsPage:password.title')} aciklama={t('SettingsPage:password.desc')}
          ikon={<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>}
          cocuk={
            <form onSubmit={parolaDegistir} className="space-y-4">
              <Girdi etiket={t('SettingsPage:password.current')} type="password" value={mevcut} onChange={e => setMevcut(e.target.value)} autoComplete="current-password" />
              <div className="grid sm:grid-cols-2 gap-4">
                <Girdi etiket={t('SettingsPage:password.new')} type="password" value={yeni} onChange={e => setYeni(e.target.value)} autoComplete="new-password" />
                <Girdi etiket={t('SettingsPage:password.new_repeat')} type="password" value={yeni2} onChange={e => setYeni2(e.target.value)} autoComplete="new-password" />
              </div>
              <div className="flex items-center gap-3 flex-wrap">
                <button type="submit" disabled={paYuk || !mevcut || !yeni} className={btn}>{paYuk ? t('SettingsPage:password.changing') : t('SettingsPage:password.change')}</button>
                <Uyari tip="ok" mesaj={paOk} /><Uyari tip="err" mesaj={paErr} />
              </div>
            </form>
          } />

        {/* 3) 2FA */}
        <Kart baslik={t('SettingsPage:twofa.title')} aciklama={t('SettingsPage:twofa.desc')}
          ikon={<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="m9 12 2 2 4-4"/></svg>}
          cocuk={
            <div className="space-y-4">
              <div className="flex items-center gap-3">
                <span className="text-sm text-slate-600 dark:text-slate-400">{t('SettingsPage:twofa.status')}</span>
                {ben?.iki_fa
                  ? <span className="text-xs font-semibold px-2.5 py-1 rounded-full bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300">{t('SettingsPage:twofa.active')}</span>
                  : <span className="text-xs font-semibold px-2.5 py-1 rounded-full bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300">{t('SettingsPage:twofa.off')}</span>}
              </div>

              {!ben?.iki_fa && !f2Kur && (
                <button onClick={f2Baslat} className={btn}>{t('SettingsPage:twofa.enable')}</button>
              )}

              {!ben?.iki_fa && f2Kur && (
                <form onSubmit={f2Etkinlestir} className="space-y-3 border border-slate-200 dark:border-slate-700 rounded-2xl p-4 bg-slate-50 dark:bg-slate-900">
                  <p className="text-sm text-slate-700 dark:text-slate-300">{t('SettingsPage:twofa.step1')}</p>
                  {f2Kur.qr_data_uri && (
                    <div className="flex flex-col items-center gap-2 py-1">
                      <img src={f2Kur.qr_data_uri} alt={t('SettingsPage:twofa.qr_alt')} width={256} height={256}
                        className="w-64 h-64 rounded-2xl bg-white p-3 border border-slate-200 dark:border-slate-700 shadow-sm" />
                      <p className="text-xs text-slate-500 dark:text-slate-500">{t('SettingsPage:twofa.scan_hint')}</p>
                    </div>
                  )}
                  <p className="text-xs text-slate-500 dark:text-slate-500">{t('SettingsPage:twofa.manual_hint')}</p>
                  <div className="flex items-center gap-2 flex-wrap">
                    <code className="font-mono text-sm px-3 py-2 rounded-lg bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-800 dark:text-slate-100 tracking-widest select-all">{secretGruplu}</code>
                    <button type="button" onClick={() => { navigator.clipboard?.writeText(f2Kur.secret) }} className="text-xs px-2.5 py-1.5 rounded border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700">{t('common:copy')}</button>
                  </div>
                  <p className="text-[11px] text-slate-500 dark:text-slate-500 break-all">{t('SettingsPage:twofa.or_link')} <span className="font-mono">{f2Kur.otpauth}</span></p>
                  <p className="text-sm text-slate-700 dark:text-slate-300">{t('SettingsPage:twofa.step2')}</p>
                  <div className="flex items-center gap-3 flex-wrap">
                    <input value={f2Kod} onChange={e => setF2Kod(e.target.value.replace(/\D/g, '').slice(0, 6))} placeholder="000000" inputMode="numeric"
                      className="w-32 px-3 py-2 text-center text-lg font-mono tracking-[0.3em] bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-600 rounded-lg text-slate-800 dark:text-slate-100 focus:border-brand-500 outline-none" />
                    <button type="submit" disabled={f2Yuk || f2Kod.length !== 6} className={btn}>{f2Yuk ? t('SettingsPage:twofa.verifying') : t('SettingsPage:twofa.verify_enable')}</button>
                    <button type="button" onClick={() => setF2Kur(null)} className="text-xs text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">{t('SettingsPage:twofa.cancel')}</button>
                  </div>
                  <Uyari tip="err" mesaj={f2Err} />
                </form>
              )}

              {ben?.iki_fa && !f2Kapat && (
                <button onClick={() => { setF2Kapat(true); setF2Err('') }} className="px-4 py-2 text-sm font-medium rounded-lg border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20">{t('SettingsPage:twofa.disable')}</button>
              )}
              {ben?.iki_fa && f2Kapat && (
                <form onSubmit={f2KapatOnay} className="space-y-3 border border-red-200 dark:border-red-800 rounded-2xl p-4 bg-red-50 dark:bg-red-900/10">
                  <p className="text-sm text-slate-700 dark:text-slate-300">{t('SettingsPage:twofa.disable_prompt')}</p>
                  <div className="flex items-center gap-3 flex-wrap">
                    <input value={kapatKod} onChange={e => setKapatKod(e.target.value.replace(/\D/g, '').slice(0, 6))} placeholder="000000" inputMode="numeric"
                      className="w-32 px-3 py-2 text-center text-lg font-mono tracking-[0.3em] bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-600 rounded-lg text-slate-800 dark:text-slate-100 outline-none" />
                    <button type="submit" disabled={f2Yuk || kapatKod.length !== 6} className="px-4 py-2 text-sm font-medium rounded-lg bg-red-600 hover:bg-red-700 text-white disabled:opacity-50">{t('common:close')}</button>
                    <button type="button" onClick={() => setF2Kapat(false)} className="text-xs text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">{t('SettingsPage:twofa.give_up')}</button>
                  </div>
                  <Uyari tip="err" mesaj={f2Err} />
                </form>
              )}
            </div>
          } />

        {/* 3.4) API token'ları — otomasyon için. Token sahibinin ROLÜYLE çalışır
            (bkz. internal/auth/apitoken.go), yani ayrı bir izin seti yoktur. */}
        <Kart baslik={t('SettingsPage:apitoken.title')} aciklama={t('SettingsPage:apitoken.desc')}
          ikon={<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>}
          cocuk={
            <div className="space-y-4">
              {yeniUretilen && (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-3 dark:border-amber-800 dark:bg-amber-900/20">
                  <p className="text-xs font-semibold text-amber-800 dark:text-amber-300">
                    {t('SettingsPage:apitoken.created_warning')}
                  </p>
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    <code className="min-w-0 flex-1 overflow-x-auto rounded-lg bg-slate-900 px-3 py-2 font-mono text-[11px] text-slate-100">
                      {yeniUretilen.token}
                    </code>
                    <button type="button"
                      onClick={() => { navigator.clipboard?.writeText(yeniUretilen.token); setKopyalandi(true) }}
                      className="shrink-0 rounded-lg border border-slate-300 px-3 py-2 text-xs font-medium hover:bg-white dark:border-slate-600 dark:hover:bg-slate-700">
                      {kopyalandi ? t('SettingsPage:apitoken.copied') : t('common:copy')}
                    </button>
                    <button type="button" onClick={() => { setYeniUretilen(null); setKopyalandi(false) }}
                      className="shrink-0 rounded-lg bg-amber-600 px-3 py-2 text-xs font-medium text-white hover:bg-amber-700">
                      {t('SettingsPage:apitoken.saved_close')}
                    </button>
                  </div>
                </div>
              )}

              <form onSubmit={tokenOlustur} className="flex flex-wrap items-end gap-3">
                <div className="min-w-[12rem] flex-1">
                  <Girdi etiket={t('SettingsPage:apitoken.name')} value={tkAd}
                    onChange={e => setTkAd(e.target.value)} maxLength={64}
                    placeholder={t('SettingsPage:apitoken.name_placeholder')} />
                </div>
                <label className="block">
                  <span className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">{t('SettingsPage:apitoken.expiry')}</span>
                  <select value={tkGun} onChange={e => setTkGun(e.target.value)} className="ta-input">
                    <option value="30">{t('SettingsPage:apitoken.expiry_30')}</option>
                    <option value="90">{t('SettingsPage:apitoken.expiry_90')}</option>
                    <option value="365">{t('SettingsPage:apitoken.expiry_365')}</option>
                    <option value="0">{t('SettingsPage:apitoken.expiry_never')}</option>
                  </select>
                </label>
                <button type="submit" disabled={tkYuk || !tkAd.trim()} className={btn}>
                  {tkYuk ? t('common:saving') : t('SettingsPage:apitoken.create')}
                </button>
              </form>
              <Uyari tip="err" mesaj={tkErr} />

              {tokenlar === null ? (
                <p className="text-xs text-slate-400">{t('common:loading')}</p>
              ) : tokenlar.length === 0 ? (
                <p className="text-xs text-slate-500 dark:text-slate-400">{t('SettingsPage:apitoken.empty')}</p>
              ) : (
                <ul className="divide-y divide-slate-100 dark:divide-slate-800">
                  {tokenlar.map(tk => (
                    <li key={tk.id} className="flex flex-wrap items-center justify-between gap-2 py-2.5">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium text-slate-800 dark:text-slate-100">{tk.ad}</span>
                          <code className="font-mono text-[11px] text-slate-400">{tk.onek}…</code>
                        </div>
                        <div className="mt-0.5 text-[11px] text-slate-500 dark:text-slate-400">
                          {tk.son_kullanim_at
                            ? t('SettingsPage:apitoken.last_used', { tarih: tk.son_kullanim_at, ip: tk.son_kullanim_ip || '—' })
                            : t('SettingsPage:apitoken.never_used')}
                          {tk.bitis_at
                            ? ` · ${t('SettingsPage:apitoken.expires', { tarih: tk.bitis_at })}`
                            : ` · ${t('SettingsPage:apitoken.no_expiry')}`}
                        </div>
                      </div>
                      <button type="button" onClick={() => tokenSil(tk)}
                        className="shrink-0 rounded-lg border border-red-300 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-900/20">
                        {t('SettingsPage:apitoken.revoke')}
                      </button>
                    </li>
                  ))}
                </ul>
              )}

              <p className="text-[11px] text-slate-500 dark:text-slate-400">
                {t('SettingsPage:apitoken.scope_note')}
              </p>
            </div>
          } />

        {/* 3.5) Nameserver — YALNIZ bayi. Admin'inki sunucu geneli bir ayar
            olduğu için Araçlar & Ayarlar sayfasındadır (NameserverAyari). */}
        {bayiMi && (
          <Kart baslik={t('SettingsPage:nameserver.title')} aciklama={t('SettingsPage:nameserver.desc')}
            ikon={<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="2" y="4" width="20" height="7" rx="2"/><rect x="2" y="13" width="20" height="7" rx="2"/><path d="M6 7.5h.01M6 16.5h.01"/></svg>}
            cocuk={
              <div className="space-y-4">
                <p className="text-xs text-slate-500 dark:text-slate-400">
                  {t('SettingsPage:nameserver.help_bayi')}
                </p>
                {ns?.uyari && <Uyari tip="err" mesaj={ns.uyari} />}
                {ns?.kaynak === 'yok' && ns?.oneri1 && (
                  <p className="text-xs text-slate-500 dark:text-slate-400">
                    {t('SettingsPage:nameserver.suggested')}
                  </p>
                )}
                <div className="grid sm:grid-cols-2 gap-4">
                  <Girdi etiket="NS1" value={ns1} onChange={e => setNS1(e.target.value)} placeholder="ns1.ornek.com" />
                  <Girdi etiket="NS2" value={ns2} onChange={e => setNS2(e.target.value)} placeholder="ns2.ornek.com" />
                </div>
                <div className="flex items-center gap-3 flex-wrap">
                  <button onClick={nsKaydet} disabled={nsYuk} className={btn}>{nsYuk ? t('common:saving') : t('common:save')}</button>
                </div>
                <Uyari tip="ok" mesaj={nsOk} />
                <Uyari tip="err" mesaj={nsErr} />
              </div>
            } />
        )}

        {/* 4) Tercihler */}
        <Kart baslik={t('SettingsPage:prefs.title')} aciklama={t('SettingsPage:prefs.desc')}
          ikon={<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>}
          cocuk={
            <div className="space-y-4">
              <div className="grid sm:grid-cols-2 gap-4">
                <label className="block">
                  <span className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('SettingsPage:prefs.theme')}</span>
                  <select value={tema} onChange={e => setTema(e.target.value)} className="w-full px-3 py-2 text-sm bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-600 rounded-lg text-slate-800 dark:text-slate-100 outline-none">
                    <option value="system">{t('SettingsPage:prefs.theme_system')}</option><option value="light">{t('SettingsPage:prefs.theme_light')}</option><option value="dark">{t('SettingsPage:prefs.theme_dark')}</option>
                  </select>
                </label>
                <label className="block">
                  <span className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('SettingsPage:prefs.language')}</span>
                  <select value={dil} onChange={e => setDil(e.target.value)} className="w-full px-3 py-2 text-sm bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-600 rounded-lg text-slate-800 dark:text-slate-100 outline-none">
                    <option value="tr">Türkçe</option><option value="en">English</option>
                  </select>
                </label>
              </div>
              <div className="flex items-center gap-3 flex-wrap">
                <button onClick={tercihKaydet} disabled={tYuk} className={btn}>{tYuk ? t('common:saving') : t('SettingsPage:prefs.save')}</button>
                <Uyari tip="ok" mesaj={tOk} />
              </div>
            </div>
          } />
      </div>
    </div>
  )
}
