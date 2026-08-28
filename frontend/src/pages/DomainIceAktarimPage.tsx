import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { AxiosProgressEvent } from 'axios'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type ArsivOzet = {
  stage_id: string
  dosya_adi: string
  boyut: number
  ozet: {
    uye_sayisi: number
    toplam_bayt: number
    kok_klasor: string
    kokler: string[]
    isaretler: Record<string, string[]>
  }
  uygulama: string
  config_yolu: string
  uyarilar: string[]
}

type DB = { id: number; db_adi: string; db_user: string }

type Domain = { id: number; alan_adi: string }

type ConfigSonuc = {
  db_adi: string
  guncellemeler: { yol: string; tur: string; alanlar: string[]; uygulandi: boolean; not?: string }[]
}
type ImportJob = { id:number; tur:'files'|'database'; durum:'queued'|'running'|'success'|'failed'|'rolled_back'; ilerleme:number; adim:string; mesaj:string; recovery_file:string; created_at:string; finished_at:string }
type Health = { ok:boolean; checks:{name:string;ok:boolean;detail:string}[] }

function mb(bayt: number) {
  if (bayt < 1024) return `${bayt} B`
  if (bayt < 1024 * 1024) return `${(bayt / 1024).toFixed(1)} KB`
  if (bayt < 1024 * 1024 * 1024) return `${(bayt / 1024 / 1024).toFixed(1)} MB`
  return `${(bayt / 1024 / 1024 / 1024).toFixed(2)} GB`
}

const kutu = 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl p-4 mb-4'
const dugme = 'px-3.5 py-2 rounded-lg text-sm font-medium bg-slate-900 text-white hover:bg-slate-700 disabled:opacity-50 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'
const girdi = 'w-full px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm text-slate-900 dark:text-slate-100'
const etiketSinif = 'block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1'

export default function DomainIceAktarimPage() {
  const { t } = useTranslation(['DomainIceAktarimPage', 'common'])
  const { id } = useParams()

  const [hata, setHata] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [domain, setDomain] = useState<Domain | null>(null)

  // --- 1. Site dosyaları ---
  const [arsiv, setArsiv] = useState<File | null>(null)
  const [ozet, setOzet] = useState<ArsivOzet | null>(null)
  const [yukleniyor, setYukleniyor] = useState(false)
  const [ilerleme, setIlerleme] = useState(0)
  const [hedef, setHedef] = useState('public_html')
  const [kokAtla, setKokAtla] = useState(true)
  const [temizle, setTemizle] = useState(false)
  const [cikariliyor, setCikariliyor] = useState(false)

  // --- 2. Veritabanı ---
  const [dbler, setDBler] = useState<DB[]>([])
  const [dbAdi, setDBAdi] = useState('')
  const [dump, setDump] = useState<File | null>(null)
  const [bosalt, setBosalt] = useState(false)
  const [dumpIlerleme, setDumpIlerleme] = useState(0)
  const [dumpYukleniyor, setDumpYukleniyor] = useState(false)

  // --- 3. Config ---
  const [configSonuc, setConfigSonuc] = useState<ConfigSonuc | null>(null)
  const [configCalisiyor, setConfigCalisiyor] = useState(false)
  const [isler, setIsler] = useState<ImportJob[]>([])
  const [saglik, setSaglik] = useState<Health|null>(null)

  const isleriYukle = useCallback(() => { if (id) api.get<ImportJob[]>(`/domains/${id}/ice-aktarim/isler`).then(r => setIsler(r.data || [])).catch(() => {}) }, [id])
  function saglikKontrol(){if(id)api.get<Health>(`/domains/${id}/ice-aktarim/saglik`).then(r=>setSaglik(r.data)).catch(e=>setHata(apiHata(e)))}

  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    api.get<DB[]>(`/domains/${id}/databases`)
      .then(r => {
        setDBler(r.data || [])
        if (r.data?.length) setDBAdi(r.data[0].db_adi)
      })
      .catch(() => { /* liste boş kalır; hata ilgili adımda gösterilir */ })
    isleriYukle()
  }, [id, isleriYukle])

  useEffect(() => {
    if (!isler.some(j => j.durum === 'queued' || j.durum === 'running')) return
    const timer = window.setInterval(isleriYukle, 2000)
    return () => window.clearInterval(timer)
  }, [isler, isleriYukle])

  function sifirlaMesaj() { setHata(null); setOk(null) }

  async function arsivYukle() {
    if (!arsiv) return
    sifirlaMesaj(); setOzet(null); setYukleniyor(true); setIlerleme(0)
    const form = new FormData()
    form.append('archive', arsiv)
    try {
      const r = await api.post<ArsivOzet>(`/domains/${id}/ice-aktarim/arsiv`, form, {
        timeout: 0,
        onUploadProgress: (e: AxiosProgressEvent) => {
          if (e.total) setIlerleme(Math.round((e.loaded / e.total) * 100))
        },
      })
      setOzet(r.data)
      setKokAtla(!!r.data.ozet.kok_klasor)
    } catch (e) {
      setHata(apiHata(e, t('DomainIceAktarimPage:errors.upload')))
    } finally {
      setYukleniyor(false)
    }
  }

  async function arsivUygula() {
    if (!ozet) return
    if (temizle && !confirm(t('DomainIceAktarimPage:files.confirm_wipe', { hedef }))) return
    sifirlaMesaj(); setCikariliyor(true)
    try {
      const { data } = await api.post(`/domains/${id}/ice-aktarim/arsiv-uygula`, {
        stage_id: ozet.stage_id, hedef, kok_atla: kokAtla, temizle,
      }, { timeout: 0 })
      setOk(t('DomainIceAktarimPage:jobs.queued', { id: data.job_id })); isleriYukle()
      setOzet(null); setArsiv(null)
    } catch (e) {
      setHata(apiHata(e, t('DomainIceAktarimPage:errors.extract')))
    } finally {
      setCikariliyor(false)
    }
  }

  async function dumpYukle() {
    if (!dump || !dbAdi) return
    if (bosalt && !confirm(t('DomainIceAktarimPage:db.confirm_wipe', { db: dbAdi }))) return
    sifirlaMesaj(); setDumpYukleniyor(true); setDumpIlerleme(0)
    const form = new FormData()
    form.append('dump', dump)
    form.append('db_name', dbAdi)
    form.append('bosalt', bosalt ? '1' : '0')
    try {
      const { data } = await api.post(`/domains/${id}/ice-aktarim/sql`, form, {
        timeout: 0,
        onUploadProgress: (e: AxiosProgressEvent) => {
          if (e.total) setDumpIlerleme(Math.round((e.loaded / e.total) * 100))
        },
      })
      setOk(t('DomainIceAktarimPage:jobs.queued', { id: data.job_id })); isleriYukle()
      setDump(null)
    } catch (e) {
      setHata(apiHata(e, t('DomainIceAktarimPage:errors.dump')))
    } finally {
      setDumpYukleniyor(false)
    }
  }

  async function configGuncelle() {
    if (!dbAdi) return
    sifirlaMesaj(); setConfigCalisiyor(true); setConfigSonuc(null)
    try {
      const { data } = await api.post<ConfigSonuc>(`/domains/${id}/ice-aktarim/config`, {
        db_name: dbAdi, dizin: hedef,
      })
      setConfigSonuc(data)
      if (!data.guncellemeler.length) setHata(t('DomainIceAktarimPage:config.none_found'))
    } catch (e) {
      setHata(apiHata(e, t('DomainIceAktarimPage:errors.config')))
    } finally {
      setConfigCalisiyor(false)
    }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('common:domain'), href: '/domainler' },
        { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainIceAktarimPage:title') },
      ]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainIceAktarimPage:title')}</h1>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">{t('DomainIceAktarimPage:desc')}</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

      {isler.length > 0 && <section className={kutu}>
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainIceAktarimPage:jobs.title')}</h2>
        <div className="space-y-3">{isler.map(j => <div key={j.id} className="border border-slate-200 dark:border-slate-800 rounded-xl p-3">
          <div className="flex items-center justify-between text-sm mb-2"><span className="font-medium">#{j.id} · {t(`DomainIceAktarimPage:jobs.types.${j.tur}`)}</span><span className={j.durum==='success'?'text-emerald-600':j.durum==='failed'?'text-red-600':j.durum==='rolled_back'?'text-amber-600':'text-blue-600'}>{t(`DomainIceAktarimPage:jobs.states.${j.durum}`)}</span></div>
          <div className="h-2 bg-slate-100 dark:bg-slate-800 rounded-full overflow-hidden"><div className="h-full bg-brand-600 transition-all" style={{width:`${j.ilerleme}%`}} /></div>
          <div className="mt-1 text-xs text-slate-500">{j.ilerleme}% · {j.adim}</div>{j.mesaj&&<div className="mt-2 text-xs text-red-600 dark:text-red-400">{j.mesaj}</div>}{j.recovery_file&&<div className="mt-1 text-xs text-slate-500">{t('DomainIceAktarimPage:jobs.recovery')}: <span className="font-mono">{j.recovery_file}</span></div>}
        </div>)}</div>
      </section>}

      <section className={kutu}><div className="flex items-center justify-between"><div><h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainIceAktarimPage:health.title')}</h2><p className="text-xs text-slate-500 mt-1">{t('DomainIceAktarimPage:health.desc')}</p></div><button onClick={saglikKontrol} className={dugme}>{t('DomainIceAktarimPage:health.button')}</button></div>
      {saglik&&<div className="grid grid-cols-1 sm:grid-cols-3 gap-2 mt-3">{saglik.checks.map(c=><div key={c.name} className={`p-3 rounded-lg border text-xs ${c.ok?'border-emerald-200 bg-emerald-50 text-emerald-700':'border-red-200 bg-red-50 text-red-700'}`}><div className="font-semibold">{c.ok?'✓':'✕'} {t(`DomainIceAktarimPage:health.${c.name}`)}</div><div className="mt-1 opacity-80">{c.detail}</div></div>)}</div>}</section>

      {/* 1 — Site dosyaları */}
      <section className={kutu}>
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainIceAktarimPage:files.title')}</h2>
        <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">{t('DomainIceAktarimPage:files.desc')}</p>

        <div className="flex flex-wrap items-end gap-3">
          <div className="flex-1 min-w-[240px]">
            <label className={etiketSinif}>{t('DomainIceAktarimPage:files.file')}</label>
            <input type="file" accept=".zip,.tar,.tar.gz,.tgz,.tar.bz2,.tbz2,.tar.xz,.txz,.rar"
              onChange={e => { setArsiv(e.target.files?.[0] || null); setOzet(null) }}
              className="block w-full text-sm text-slate-600 dark:text-slate-300 file:mr-3 file:px-3 file:py-1.5 file:rounded-lg file:border-0 file:text-sm file:bg-slate-100 dark:file:bg-slate-800 file:text-slate-700 dark:file:text-slate-200" />
          </div>
          <button onClick={arsivYukle} disabled={!arsiv || yukleniyor} className={dugme}>
            {yukleniyor ? t('DomainIceAktarimPage:files.uploading', { pct: ilerleme }) : t('DomainIceAktarimPage:files.analyze')}
          </button>
        </div>

        {ozet && (
          <div className="mt-4 border-t border-slate-200 dark:border-slate-800 pt-4">
            <dl className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-sm mb-3">
              <div><dt className="text-xs text-slate-500">{t('DomainIceAktarimPage:files.entries')}</dt><dd className="font-medium text-slate-900 dark:text-slate-100">{ozet.ozet.uye_sayisi.toLocaleString()}</dd></div>
              <div><dt className="text-xs text-slate-500">{t('DomainIceAktarimPage:files.expanded')}</dt><dd className="font-medium text-slate-900 dark:text-slate-100">{mb(ozet.ozet.toplam_bayt)}</dd></div>
              <div><dt className="text-xs text-slate-500">{t('DomainIceAktarimPage:files.root')}</dt><dd className="font-medium text-slate-900 dark:text-slate-100">{ozet.ozet.kok_klasor || '—'}</dd></div>
              <div><dt className="text-xs text-slate-500">{t('DomainIceAktarimPage:files.app')}</dt><dd className="font-medium text-slate-900 dark:text-slate-100">{ozet.uygulama || '—'}</dd></div>
            </dl>

            {ozet.uyarilar.map((u, i) => (
              <div key={i} className="mb-2 px-3 py-2 bg-amber-50 dark:bg-amber-900/10 border border-amber-200 dark:border-amber-800/40 rounded-lg text-xs text-amber-800 dark:text-amber-300">{u}</div>
            ))}

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
              <div>
                <label className={etiketSinif}>{t('DomainIceAktarimPage:files.target')}</label>
                <input value={hedef} onChange={e => setHedef(e.target.value)} className={girdi} placeholder="public_html" />
              </div>
              <div className="flex flex-col justify-end gap-2 text-sm">
                <label className={`flex items-center gap-2 ${ozet.ozet.kok_klasor ? '' : 'opacity-50'}`}>
                  <input type="checkbox" checked={kokAtla} disabled={!ozet.ozet.kok_klasor}
                    onChange={e => setKokAtla(e.target.checked)} />
                  <span className="text-slate-700 dark:text-slate-300">
                    {ozet.ozet.kok_klasor
                      ? t('DomainIceAktarimPage:files.strip_root', { kok: ozet.ozet.kok_klasor })
                      : t('DomainIceAktarimPage:files.strip_root_na')}
                  </span>
                </label>
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={temizle} onChange={e => setTemizle(e.target.checked)} />
                  <span className="text-rose-700 dark:text-rose-400">{t('DomainIceAktarimPage:files.wipe_first')}</span>
                </label>
              </div>
            </div>

            <button onClick={arsivUygula} disabled={cikariliyor} className={dugme}>
              {cikariliyor ? t('DomainIceAktarimPage:files.extracting') : t('DomainIceAktarimPage:files.apply')}
            </button>
          </div>
        )}
      </section>

      {/* 2 — Veritabanı */}
      <section className={kutu}>
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainIceAktarimPage:db.title')}</h2>
        <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">{t('DomainIceAktarimPage:db.desc')}</p>

        {dbler.length === 0 ? (
          <p className="text-sm text-slate-500 dark:text-slate-400">{t('DomainIceAktarimPage:db.no_db')}</p>
        ) : (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
              <div>
                <label className={etiketSinif}>{t('DomainIceAktarimPage:db.target')}</label>
                <select value={dbAdi} onChange={e => setDBAdi(e.target.value)} className={girdi}>
                  {dbler.map(d => <option key={d.id} value={d.db_adi}>{d.db_adi}</option>)}
                </select>
              </div>
              <div>
                <label className={etiketSinif}>{t('DomainIceAktarimPage:db.file')}</label>
                <input type="file" accept=".sql,.gz,.sql.gz"
                  onChange={e => setDump(e.target.files?.[0] || null)}
                  className="block w-full text-sm text-slate-600 dark:text-slate-300 file:mr-3 file:px-3 file:py-1.5 file:rounded-lg file:border-0 file:text-sm file:bg-slate-100 dark:file:bg-slate-800 file:text-slate-700 dark:file:text-slate-200" />
              </div>
            </div>
            <label className="flex items-center gap-2 text-sm mb-3">
              <input type="checkbox" checked={bosalt} onChange={e => setBosalt(e.target.checked)} />
              <span className="text-rose-700 dark:text-rose-400">{t('DomainIceAktarimPage:db.wipe_first')}</span>
            </label>
            <button onClick={dumpYukle} disabled={!dump || dumpYukleniyor} className={dugme}>
              {dumpYukleniyor ? t('DomainIceAktarimPage:db.importing', { pct: dumpIlerleme }) : t('DomainIceAktarimPage:db.apply')}
            </button>
          </>
        )}
      </section>

      {/* 3 — Bağlantı ayarları */}
      <section className={kutu}>
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainIceAktarimPage:config.title')}</h2>
        <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">{t('DomainIceAktarimPage:config.desc')}</p>
        <button onClick={configGuncelle} disabled={!dbAdi || configCalisiyor} className={dugme}>
          {configCalisiyor ? t('common:loading') : t('DomainIceAktarimPage:config.apply', { db: dbAdi, dizin: hedef })}
        </button>

        {configSonuc && configSonuc.guncellemeler.length > 0 && (
          <ul className="mt-3 space-y-2 text-sm">
            {configSonuc.guncellemeler.map((g, i) => (
              <li key={i} className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800">
                <span className="font-mono text-xs text-slate-700 dark:text-slate-300">{g.yol}</span>
                <span className="ml-2 text-xs uppercase text-slate-500">{g.tur}</span>
                {g.uygulandi
                  ? <span className="ml-2 text-xs text-emerald-600 dark:text-emerald-400">✓ {g.alanlar.join(', ')}</span>
                  : <span className="ml-2 text-xs text-amber-600 dark:text-amber-400">{g.not}</span>}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
