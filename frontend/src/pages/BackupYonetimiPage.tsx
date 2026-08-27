import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ConfirmDialog from '@/components/ConfirmDialog'
import { T } from '@/lib/tablo'

type OzetSatir = {
  domain_id: number; alan_adi: string; sayi: number; toplam_b: number; son_yedek: string
  oto_sayi: number; manuel_sayi: number; freq: string; retention: number; manuel_retention: number
}
type Ozet = {
  domainler: OzetSatir[]; toplam_boyut_b: number; toplam_yedek: number
  toplam_oto: number; toplam_manuel: number; hedef_sayisi: number
  otomatik_domain: number; zamanlama_saat: number
  retention_min: number; retention_max: number
  manuel_ret_min: number; manuel_ret_max: number
}
type TopluDurum = {
  calisiyor: boolean; toplam: number; tamamlanan: number; basarisiz: number
  suanki: string; hatalar: string[]; baslangic: string; bitis: string; bende: boolean
}
type TemizlikMod = 'oto' | 'manuel' | 'gun'
type TemizlikSonuc = { sayi: number; boyut_b: number; domain: number; onizleme: boolean }
type Bekleyen = { mod: TemizlikMod; gun: number; sonuc: TemizlikSonuc }

export default function BackupYonetimiPage() {
  const { t } = useTranslation(['BackupYonetimiPage', 'common'])
  const [o, setO] = useState<Ozet | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [yedekliyor, setYedekliyor] = useState(false)
  const [toplu, setToplu] = useState<TopluDurum | null>(null)
  const [gun, setGun] = useState(3)
  const [temizleniyor, setTemizleniyor] = useState<TemizlikMod | null>(null)
  const [bekleyen, setBekleyen] = useState<Bekleyen | null>(null)

  function yukle() {
    setYuk(true)
    api.get<Ozet>('/admin/backups/ozet')
      .then(r => setO(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [])

  async function durumOku(): Promise<TopluDurum | null> {
    try {
      const { data } = await api.get<TopluDurum>('/admin/backups/toplu-durum')
      setToplu(data)
      return data
    } catch { return null }
  }

  // Sayfa açılışında bir kez: iş başka bir sekmede/kullanıcıda başlamış olabilir,
  // o zaman da ilerleme görünmeli.
  useEffect(() => { durumOku() }, [])

  // Yedekleme dakikalar sürer; iş bitene kadar durumu yoklarız. Buton eskiden
  // "tetiklendi" deyip susuyordu, kullanıcı işin bitip bitmediğini bilemiyordu.
  useEffect(() => {
    if (!toplu?.calisiyor) return
    const iv = setInterval(async () => {
      const d = await durumOku()
      if (d && !d.calisiyor) {
        yukle()
        setBasari(d.basarisiz
          ? t('BackupYonetimiPage:bulk.done_partial', { ok: d.tamamlanan, failed: d.basarisiz })
          : t('BackupYonetimiPage:bulk.done', { ok: d.tamamlanan }))
      }
    }, 2000)
    return () => clearInterval(iv)
  }, [toplu?.calisiyor, t])

  async function simdiYedekle() {
    setHata(null); setBasari(null); setYedekliyor(true)
    try {
      const { data } = await api.post<{ toplam: number }>('/admin/backups/hepsini-yedekle')
      if (!data.toplam) { setBasari(t('BackupYonetimiPage:bulk.no_domain')); return }
      setBasari(t('BackupYonetimiPage:bulk.started', { count: data.toplam }))
      durumOku() // yoklamayı hemen başlat
    } catch (e) { setHata(apiHata(e, t('BackupYonetimiPage:error_trigger_failed'))) }
    finally { setYedekliyor(false) }
  }

  // Silme iki adımlı: önce sunucudan kuru bir sayım alınır, onay kutusu gerçek
  // rakamı gösterir. Rakamsız bir "emin misiniz?" onay sayılmaz.
  async function temizlikOnizle(mod: TemizlikMod) {
    setHata(null); setBasari(null); setTemizleniyor(mod)
    try {
      const { data } = await api.post<TemizlikSonuc>('/admin/backups/temizle',
        { mod, gun, onizleme: true })
      if (!data.sayi) { setBasari(t('BackupYonetimiPage:cleanup.nothing')); return }
      setBekleyen({ mod, gun, sonuc: data })
    } catch (e) { setHata(apiHata(e, t('BackupYonetimiPage:cleanup.failed'))) }
    finally { setTemizleniyor(null) }
  }

  async function temizlikUygula() {
    if (!bekleyen) return
    try {
      const { data } = await api.post<TemizlikSonuc>('/admin/backups/temizle',
        { mod: bekleyen.mod, gun: bekleyen.gun, onizleme: false })
      setBekleyen(null)
      setBasari(t('BackupYonetimiPage:cleanup.done', { count: data.sayi, size: fmtByte(data.boyut_b) }))
      yukle()
    } catch (e) { setBekleyen(null); setHata(apiHata(e, t('BackupYonetimiPage:cleanup.failed'))) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('BackupYonetimiPage:breadcrumb.tools_settings'), href: '/araclar-ayarlar' },
        { etiket: t('BackupYonetimiPage:title') },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">💾</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('BackupYonetimiPage:title')}</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">{t('BackupYonetimiPage:subtitle')}</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {/* KPI */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
        <Kpi et={t('BackupYonetimiPage:kpi.total_size')} v={o ? fmtByte(o.toplam_boyut_b) : '—'} renk="sky" ikon="💽" />
        <Kpi et={t('BackupYonetimiPage:kpi.total_backups')} v={o ? String(o.toplam_yedek) : '—'} renk="violet" ikon="📦"
          alt={o ? t('BackupYonetimiPage:kpi.split', { auto: o.toplam_oto, manual: o.toplam_manuel }) : undefined} />
        <Kpi et={t('BackupYonetimiPage:kpi.domain_count')} v={o ? String(o.domainler.length) : '—'} renk="teal" ikon="🌐" />
        <Kpi et={t('BackupYonetimiPage:kpi.active_remote_target')} v={o ? String(o.hedef_sayisi) : '—'} renk="emerald" ikon="☁️" alt={t('BackupYonetimiPage:kpi.s3_sftp')} />
      </div>

      {/* Zamanlama + eylem */}
      <div className="mb-5 flex flex-wrap items-center gap-3 px-4 py-3 rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60">
        <span className="text-sm text-slate-600 dark:text-slate-300">
          {t('BackupYonetimiPage:schedule.label')} <strong>{o ? zamanlamaMetni(o, t) : '—'}</strong>
          {o && <> · {otoSaklamaMetni(o, t)} · {manuelSaklamaMetni(o, t)}</>}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button onClick={simdiYedekle} disabled={yedekliyor || !!toplu?.calisiyor}
            className="px-3.5 py-2 text-sm font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
            {toplu?.calisiyor
              ? t('BackupYonetimiPage:bulk.running_button', { done: toplu.tamamlanan + toplu.basarisiz, total: toplu.toplam })
              : yedekliyor ? t('BackupYonetimiPage:buttons.triggering') : t('BackupYonetimiPage:buttons.backup_now')}
          </button>
          <button onClick={yukle} disabled={yuk} className="px-3 py-2 text-sm border border-slate-200 dark:border-slate-700 rounded-lg text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">↻ {t('common:refresh')}</button>
        </div>
      </div>

      {/* Toplu yedekleme ilerlemesi */}
      {toplu?.calisiyor && (
        <div className="mb-5 px-4 py-3 rounded-2xl border border-sky-200 dark:border-sky-900/60 bg-sky-50 dark:bg-sky-900/20">
          <div className="flex items-center justify-between text-sm text-sky-800 dark:text-sky-200">
            <span>
              {toplu.bende
                ? t('BackupYonetimiPage:bulk.progress', { done: toplu.tamamlanan + toplu.basarisiz, total: toplu.toplam, domain: toplu.suanki })
                : t('BackupYonetimiPage:bulk.progress_other', { done: toplu.tamamlanan + toplu.basarisiz, total: toplu.toplam })}
            </span>
            {toplu.basarisiz > 0 && (
              <span className="text-red-700 dark:text-red-300">{t('BackupYonetimiPage:bulk.failed_count', { count: toplu.basarisiz })}</span>
            )}
          </div>
          <div className="mt-2 h-1.5 rounded-full bg-sky-200/70 dark:bg-sky-900/60 overflow-hidden">
            <div className="h-full bg-sky-600 dark:bg-sky-400 transition-all"
              style={{ width: `${toplu.toplam ? Math.round(((toplu.tamamlanan + toplu.basarisiz) / toplu.toplam) * 100) : 0}%` }} />
          </div>
        </div>
      )}

      {/* Biten işin hataları: sayfa yenilenene kadar görünür kalır */}
      {!toplu?.calisiyor && !!toplu?.hatalar?.length && (
        <div className="mb-5 px-4 py-3 rounded-2xl border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20">
          <div className="text-sm font-medium text-red-700 dark:text-red-300 mb-1">{t('BackupYonetimiPage:bulk.errors_title')}</div>
          <ul className="text-xs text-red-700 dark:text-red-300 font-mono space-y-0.5">
            {toplu.hatalar.map((h, i) => <li key={i} className="break-all">• {h}</li>)}
          </ul>
        </div>
      )}

      {/* Tablo */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
        <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('BackupYonetimiPage:table.title')}</h3>
        </div>
        <div className="lg:overflow-x-auto">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/50 border-b border-slate-200 dark:border-slate-700/60`}>
              <tr>
                <th className={T.baslik}>{t('common:domain')}</th>
                <th className={`${T.baslik} text-right`}>{t('BackupYonetimiPage:table.backup_count')}</th>
                <th className={`${T.baslik} text-right`}>{t('BackupYonetimiPage:table.total_size')}</th>
                <th className={T.baslik}>{t('BackupYonetimiPage:table.last_backup')}</th>
                <th className={`${T.baslik} text-right`}>{t('common:actions')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-700/60`}>
              {yuk ? (
                <tr><td colSpan={5} className={T.hucreDurum}>{t('common:loading')}</td></tr>
              ) : !o || o.domainler.length === 0 ? (
                <tr><td colSpan={5} className={T.hucreDurum}>{t('BackupYonetimiPage:table.no_domain')}</td></tr>
              ) : (
                o.domainler.map(d => (
                  <tr key={d.domain_id} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/40`}>
                    <td className={T.hucreBaslik}>{d.alan_adi}</td>
                    <td className={T.hucre} data-etiket={t('BackupYonetimiPage:table.backup_count')}>
                      <span className="font-mono text-xs text-slate-600 dark:text-slate-300">{d.sayi}</span>
                      {d.sayi > 0 && (
                        <span className="block text-[10px] text-slate-400 dark:text-slate-500">
                          {t('BackupYonetimiPage:table.split', { auto: d.oto_sayi, manual: d.manuel_sayi })}
                        </span>
                      )}
                    </td>
                    <td className={T.hucre} data-etiket={t('BackupYonetimiPage:table.total_size')}><span className="font-mono text-xs text-slate-600 dark:text-slate-300">{d.sayi ? fmtByte(d.toplam_b) : '—'}</span></td>
                    <td className={T.hucre} data-etiket={t('BackupYonetimiPage:table.last_backup')}><span className="font-mono text-xs text-slate-500 dark:text-slate-400">{d.son_yedek || <span className="text-slate-400">{t('BackupYonetimiPage:table.never')}</span>}</span></td>
                    <td className={T.hucreAksiyon}>
                      <Link to={`/abonelikler/${d.domain_id}/yedekler`} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-brand-600 dark:text-brand-400 hover:bg-slate-50 dark:hover:bg-slate-700">{t('BackupYonetimiPage:table.manage')}</Link>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
      <p className="text-xs text-slate-400 dark:text-slate-500 mt-3">
        {t('BackupYonetimiPage:info_note_before')}<span className="font-mono">/var/backups/sanalcp/&lt;domain&gt;/</span>{t('BackupYonetimiPage:info_note_after', { manage: t('BackupYonetimiPage:table.manage_text') })}
      </p>

      {/* Toplu temizlik — sayfanın en altı, kırmızı çerçeve: yıkıcı bölge */}
      <div className="mt-6 bg-white dark:bg-slate-800/60 border border-red-200 dark:border-red-900/50 rounded-2xl overflow-hidden">
        <div className="px-4 py-3 border-b border-red-100 dark:border-red-900/40">
          <h3 className="text-sm font-semibold text-red-700 dark:text-red-300">🗑 {t('BackupYonetimiPage:cleanup.title')}</h3>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{t('BackupYonetimiPage:cleanup.desc')}</p>
        </div>
        <div className="p-4 grid grid-cols-1 lg:grid-cols-3 gap-3">
          <TemizlikKutu
            baslik={t('BackupYonetimiPage:cleanup.auto.title')}
            aciklama={t('BackupYonetimiPage:cleanup.auto.desc')}
            sayi={o?.toplam_oto}
            etiket={t('BackupYonetimiPage:cleanup.count_label')}
            buton={t('BackupYonetimiPage:cleanup.auto.button')}
            mesgul={temizleniyor === 'oto'}
            kilit={!!temizleniyor || yuk}
            onTikla={() => temizlikOnizle('oto')}
          />
          <TemizlikKutu
            baslik={t('BackupYonetimiPage:cleanup.manual.title')}
            aciklama={t('BackupYonetimiPage:cleanup.manual.desc')}
            sayi={o?.toplam_manuel}
            etiket={t('BackupYonetimiPage:cleanup.count_label')}
            buton={t('BackupYonetimiPage:cleanup.manual.button')}
            mesgul={temizleniyor === 'manuel'}
            kilit={!!temizleniyor || yuk}
            onTikla={() => temizlikOnizle('manuel')}
          />
          <TemizlikKutu
            baslik={t('BackupYonetimiPage:cleanup.days.title')}
            aciklama={t('BackupYonetimiPage:cleanup.days.desc')}
            buton={t('BackupYonetimiPage:cleanup.days.button', { days: gun })}
            mesgul={temizleniyor === 'gun'}
            kilit={!!temizleniyor || yuk}
            onTikla={() => temizlikOnizle('gun')}
          >
            <label className="flex items-center gap-2 mt-2">
              <span className="text-[11px] text-slate-500 dark:text-slate-400">{t('BackupYonetimiPage:cleanup.days.keep_label')}</span>
              <input type="number" min={1} max={365} value={gun}
                onChange={e => setGun(Math.max(1, Math.min(365, Number(e.target.value) || 1)))}
                className="w-20 px-2 py-1 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono bg-white dark:bg-slate-900" />
            </label>
          </TemizlikKutu>
        </div>
      </div>

      <ConfirmDialog
        acik={!!bekleyen}
        baslik={t('BackupYonetimiPage:cleanup.confirm_title')}
        mesaj={bekleyen ? t(`BackupYonetimiPage:cleanup.confirm_${bekleyen.mod}`, {
          count: bekleyen.sonuc.sayi,
          size: fmtByte(bekleyen.sonuc.boyut_b),
          domains: bekleyen.sonuc.domain,
          days: bekleyen.gun,
        }) : ''}
        tehlikeli onayMetni={t('BackupYonetimiPage:cleanup.confirm_ok')}
        onOnay={temizlikUygula}
        onIptal={() => setBekleyen(null)}
      />
    </div>
  )
}

function TemizlikKutu({ baslik, aciklama, sayi, etiket, buton, mesgul, kilit, onTikla, children }: {
  baslik: string; aciklama: string; sayi?: number; etiket?: string
  buton: string; mesgul: boolean; kilit: boolean; onTikla: () => void
  children?: React.ReactNode
}) {
  return (
    <div className="flex flex-col p-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-slate-50/60 dark:bg-slate-900/40">
      <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">{baslik}</div>
      <p className="text-[11px] text-slate-500 dark:text-slate-400 mt-1 leading-snug flex-1">{aciklama}</p>
      {typeof sayi === 'number' && (
        <div className="mt-2 text-xs text-slate-600 dark:text-slate-300">
          <span className="font-mono font-semibold">{sayi}</span> {etiket}
        </div>
      )}
      {children}
      <button type="button" onClick={onTikla} disabled={kilit || sayi === 0}
        className="mt-3 w-full px-3 py-2 text-xs font-medium border border-red-300 dark:border-red-800 text-red-700 dark:text-red-300 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-lg disabled:opacity-40 disabled:cursor-not-allowed">
        {mesgul ? '…' : buton}
      </button>
    </div>
  )
}

type Ceviri = (k: string, o?: Record<string, unknown>) => string

// Banner metinleri sunucudan gelen GERÇEK ayarlardan kurulur. Eskiden sabit
// "7 günlük saklama" yazıyordu; retention domain başına ayarlanabildiği ve
// gün değil ADET olduğu için bu metin iki kez yanlıştı.
function zamanlamaMetni(o: Ozet, t: Ceviri): string {
  if (!o.otomatik_domain) return t('BackupYonetimiPage:schedule.off')
  if (o.zamanlama_saat < 0) return t('BackupYonetimiPage:schedule.time_mixed')
  return t('BackupYonetimiPage:schedule.time', { hour: String(o.zamanlama_saat).padStart(2, '0') })
}

function otoSaklamaMetni(o: Ozet, t: Ceviri): string {
  if (!o.otomatik_domain) return t('BackupYonetimiPage:schedule.auto_none')
  if (o.retention_min === o.retention_max) return t('BackupYonetimiPage:schedule.auto_retention', { n: o.retention_max })
  return t('BackupYonetimiPage:schedule.auto_retention_range', { min: o.retention_min, max: o.retention_max })
}

function manuelSaklamaMetni(o: Ozet, t: Ceviri): string {
  // 0 = sınırsız. min 0 ama max > 0 ise domainler karışık ayarda demektir.
  if (o.manuel_ret_max === 0) return t('BackupYonetimiPage:schedule.manual_unlimited')
  if (o.manuel_ret_min === 0) return t('BackupYonetimiPage:schedule.manual_mixed')
  if (o.manuel_ret_min === o.manuel_ret_max) return t('BackupYonetimiPage:schedule.manual_retention', { n: o.manuel_ret_max })
  return t('BackupYonetimiPage:schedule.manual_retention_range', { min: o.manuel_ret_min, max: o.manuel_ret_max })
}

function Kpi({ et, v, renk, ikon, alt }: { et: string; v: string; renk: string; ikon: string; alt?: string }) {
  const c: Record<string, string> = {
    sky: 'text-sky-600 dark:text-sky-400', violet: 'text-violet-600 dark:text-violet-400',
    teal: 'text-teal-600 dark:text-teal-400', emerald: 'text-emerald-600 dark:text-emerald-400',
  }
  return (
    <div className="rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60 p-4">
      <div className="flex items-center gap-2 text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{ikon} {et}</div>
      <div className={`text-2xl font-semibold mt-1 ${c[renk] || 'text-slate-700 dark:text-slate-200'}`}>{v}</div>
      {alt && <div className="text-[11px] text-slate-400 mt-0.5">{alt}</div>}
    </div>
  )
}

function fmtByte(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}
