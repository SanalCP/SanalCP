import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { T } from '@/lib/tablo'

type Kural = {
  id: number; tip: 'ban' | 'whitelist' | 'kapat'; ip: string; port: number
  protokol: string; aciklama: string; aktif: boolean; created_at: string
  kaynak: 'elle' | 'otomatik'; servis: string; bitis_at: string
}
type ListeResp = { kurallar: Kural[]; korumali_portlar: number[] }
type OtoBan = { aktif: boolean; esik: number; pencere_dk: number; sure_dk: number; aktif_ban_sayisi: number }

function sablonlar(t: TFunction) {
  return [
    { key: 'mysql_kapat', ikon: '🗄️', ad: t('FirewallPage:template_mysql_title'), portlar: '3306',
      aciklama: t('FirewallPage:template_mysql_desc') },
    { key: 'ftp_kapat', ikon: '📁', ad: t('FirewallPage:template_ftp_title'), portlar: '21',
      aciklama: t('FirewallPage:template_ftp_desc') },
    { key: 'mail_kapat', ikon: '📧', ad: t('FirewallPage:template_mail_title'), portlar: '25, 465, 587, 110, 143',
      aciklama: t('FirewallPage:template_mail_desc') },
    { key: 'rpc_kapat', ikon: '🔗', ad: t('FirewallPage:template_rpc_title'), portlar: '111, 2049',
      aciklama: t('FirewallPage:template_rpc_desc') },
  ] as const
}

function modlar(t: TFunction) {
  return {
    ban: { ikon: '🚫', ad: t('FirewallPage:mode_ban_title'), aktifRenk: 'bg-red-600 border-red-600',
      aciklama: t('FirewallPage:mode_ban_desc'),
      ornek: t('FirewallPage:mode_ban_example') },
    whitelist: { ikon: '✅', ad: t('FirewallPage:mode_whitelist_title'), aktifRenk: 'bg-emerald-600 border-emerald-600',
      aciklama: t('FirewallPage:mode_whitelist_desc'),
      ornek: t('FirewallPage:mode_whitelist_example') },
    kapat: { ikon: '🔒', ad: t('FirewallPage:mode_close_title'), aktifRenk: 'bg-amber-600 border-amber-600',
      aciklama: t('FirewallPage:mode_close_desc'),
      ornek: t('FirewallPage:mode_close_example') },
  } as const
}

export default function FirewallPage() {
  const { t } = useTranslation(['FirewallPage', 'common'])
  const SABLONLAR = sablonlar(t)
  const MODLAR = modlar(t)
  const [kurallar, setKurallar] = useState<Kural[]>([])
  const [korumali, setKorumali] = useState<number[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [mesgul, setMesgul] = useState<string | null>(null)

  const [tip, setTip] = useState<'ban' | 'whitelist' | 'kapat'>('ban')
  const [ip, setIp] = useState('')
  const [port, setPort] = useState('')
  const [protokol, setProtokol] = useState<'tcp' | 'udp'>('tcp')
  const [aciklama, setAciklama] = useState('')

  // Otomatik saldırı engelleme ayarları (varsayılan KAPALI — bkz. migrations/0067).
  const [otoban, setOtoban] = useState<OtoBan | null>(null)

  function otobanYukle() {
    api.get<OtoBan>('/firewall/otoban').then(r => setOtoban(r.data)).catch(() => {})
  }

  function yukle() {
    setYuk(true)
    api.get<ListeResp>('/firewall')
      .then(r => { setKurallar(r.data.kurallar || []); setKorumali(r.data.korumali_portlar || []) })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
    otobanYukle()
  }
  useEffect(yukle, [])

  async function otobanKaydet(yeni: OtoBan) {
    setHata(null); setBasari(null); setMesgul('otoban')
    try {
      await api.put('/firewall/otoban', {
        aktif: yeni.aktif, esik: yeni.esik, pencere_dk: yeni.pencere_dk, sure_dk: yeni.sure_dk,
      })
      setOtoban(yeni)
      setBasari(yeni.aktif ? t('FirewallPage:autoban.saved_on') : t('FirewallPage:autoban.saved_off'))
      otobanYukle()
    } catch (err) { setHata(apiHata(err, t('FirewallPage:autoban.save_failed'))); otobanYukle() }
    finally { setMesgul(null) }
  }

  async function otobanTemizle() {
    if (!confirm(t('FirewallPage:autoban.confirm_clear'))) return
    setHata(null); setBasari(null); setMesgul('otoban-temizle')
    try {
      const { data } = await api.post('/firewall/otoban/temizle')
      setBasari(t('FirewallPage:autoban.cleared', { count: data.silinen }))
      yukle()
    } catch (err) { setHata(apiHata(err, t('FirewallPage:autoban.clear_failed'))) }
    finally { setMesgul(null) }
  }

  async function sablonUygula(s: typeof SABLONLAR[number]) {
    if (!confirm(t('FirewallPage:confirm_apply', { ad: s.ad, portlar: s.portlar }))) return
    setHata(null); setBasari(null); setMesgul('sablon:' + s.key)
    try {
      const { data } = await api.post('/firewall/sablon', { sablon: s.key })
      setBasari(data.eklenen > 0 ? t('FirewallPage:applied_count', { ad: s.ad, count: data.eklenen }) : t('FirewallPage:applied_none', { ad: s.ad }))
      yukle()
    } catch (err) { setHata(apiHata(err, t('FirewallPage:template_failed'))) }
    finally { setMesgul(null) }
  }

  async function ekle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setBasari(null); setMesgul('manuel')
    try {
      await api.post('/firewall', {
        tip, ip: tip === 'kapat' ? '' : ip.trim(),
        port: port.trim() ? parseInt(port, 10) : 0, protokol, aciklama: aciklama.trim(),
      })
      setBasari(t('FirewallPage:added'))
      setIp(''); setPort(''); setAciklama('')
      yukle()
    } catch (err) { setHata(apiHata(err, t('FirewallPage:add_failed'))) }
    finally { setMesgul(null) }
  }

  async function sil(k: Kural) {
    const ozet = k.tip === 'kapat' ? t('FirewallPage:port_close_label', { port: k.port }) : `${k.ip}${k.port ? ':' + k.port : ''} ${k.tip}`
    if (!confirm(t('FirewallPage:confirm_delete', { ozet }))) return
    setHata(null); setBasari(null); setMesgul('sil:' + k.id)
    try { await api.delete(`/firewall/${k.id}`); yukle() }
    catch (err) { setHata(apiHata(err, t('FirewallPage:delete_failed'))) }
    finally { setMesgul(null) }
  }

  const ipGerekli = tip !== 'kapat'
  const mod = MODLAR[tip]
  const korumaliMetin = useMemo(() => korumali.slice().sort((a, b) => a - b).join(', '), [korumali])
  const protectedPorts = korumaliMetin || t('FirewallPage:protected_ports_default')

  // canlı önizleme cümlesi
  const onizleme = useMemo(() => {
    if (tip === 'kapat') return port ? t('FirewallPage:preview_close_with_port', { port }) : t('FirewallPage:preview_close_no_port')
    const kim = ip.trim() || t('FirewallPage:preview_ban_ip_default')
    if (tip === 'ban') {
      return port ? t('FirewallPage:preview_ban_with_port', { ip: kim, port }) : t('FirewallPage:preview_ban_no_port', { ip: kim })
    }
    return port ? t('FirewallPage:preview_whitelist_with_port', { ip: kim, port }) : t('FirewallPage:preview_whitelist_no_port', { ip: kim })
  }, [tip, ip, port, t])

  const kisitUyari = tip === 'whitelist' && port.trim() !== ''

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('FirewallPage:breadcrumb_title') }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🛡️</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('FirewallPage:title')}</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
        {t('FirewallPage:subtitle')}
      </p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      <div className="mb-5 px-4 py-2.5 rounded-lg bg-sky-50 dark:bg-sky-900/20 border border-sky-200 dark:border-sky-800 text-xs text-sky-800 dark:text-sky-200">
        {t('FirewallPage:protected_ports_notice', { ports: protectedPorts })}
      </div>

      {/* ---------- OTOMATİK SALDIRI ENGELLEME ---------- */}
      {otoban && (
        <div className="mb-6 rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-700/60 dark:bg-slate-800/60">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="text-lg">🛰️</span>
                <h2 className="text-sm font-semibold text-slate-800 dark:text-slate-100">{t('FirewallPage:autoban.title')}</h2>
                <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${
                  otoban.aktif
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                    : 'bg-slate-100 text-slate-500 dark:bg-slate-700 dark:text-slate-300'
                }`}>
                  {otoban.aktif ? t('FirewallPage:autoban.on') : t('FirewallPage:autoban.off')}
                </span>
              </div>
              <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t('FirewallPage:autoban.desc')}</p>
            </div>
            <button
              type="button"
              onClick={() => otobanKaydet({ ...otoban, aktif: !otoban.aktif })}
              disabled={!!mesgul}
              className={`shrink-0 rounded-lg px-4 py-2 text-sm font-medium disabled:opacity-50 ${
                otoban.aktif
                  ? 'border border-red-300 text-red-600 hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-900/20'
                  : 'bg-slate-900 text-white hover:bg-slate-800 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100'
              }`}
            >
              {mesgul === 'otoban' ? '…' : otoban.aktif ? t('FirewallPage:autoban.turn_off') : t('FirewallPage:autoban.turn_on')}
            </button>
          </div>

          {otoban.aktif && (
            <>
              <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
                <SayiAlan etiket={t('FirewallPage:autoban.threshold')} ipucu={t('FirewallPage:autoban.threshold_hint')}
                  deger={otoban.esik} min={3} max={100}
                  degistir={v => setOtoban({ ...otoban, esik: v })} />
                <SayiAlan etiket={t('FirewallPage:autoban.window')} ipucu={t('FirewallPage:autoban.window_hint')}
                  deger={otoban.pencere_dk} min={1} max={1440}
                  degistir={v => setOtoban({ ...otoban, pencere_dk: v })} />
                <SayiAlan etiket={t('FirewallPage:autoban.duration')} ipucu={t('FirewallPage:autoban.duration_hint')}
                  deger={otoban.sure_dk} min={1} max={43200}
                  degistir={v => setOtoban({ ...otoban, sure_dk: v })} />
              </div>
              <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                <p className="text-xs text-slate-500 dark:text-slate-400">
                  {t('FirewallPage:autoban.active_count', { count: otoban.aktif_ban_sayisi })}
                  {' · '}
                  {t('FirewallPage:autoban.whitelist_note')}
                </p>
                <div className="flex gap-2">
                  {otoban.aktif_ban_sayisi > 0 && (
                    <button type="button" onClick={otobanTemizle} disabled={!!mesgul}
                      className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-700">
                      {mesgul === 'otoban-temizle' ? '…' : t('FirewallPage:autoban.clear_all')}
                    </button>
                  )}
                  <button type="button" onClick={() => otobanKaydet(otoban)} disabled={!!mesgul}
                    className="rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100">
                    {mesgul === 'otoban' ? '…' : t('FirewallPage:autoban.save')}
                  </button>
                </div>
              </div>
            </>
          )}
        </div>
      )}

      {/* ---------- HAZIR ŞABLONLAR ---------- */}
      <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-2 flex items-center gap-2">{t('FirewallPage:templates_title')} <span className="text-xs font-normal text-slate-400">{t('FirewallPage:templates_subtitle')}</span></h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-6">
        {SABLONLAR.map(s => (
          <div key={s.key} className="flex items-start gap-3 p-4 rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60">
            <div className="w-10 h-10 rounded-lg bg-slate-100 dark:bg-slate-700 flex items-center justify-center text-xl shrink-0">{s.ikon}</div>
            <div className="flex-1 min-w-0">
              <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">{s.ad}</div>
              <div className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{s.aciklama}</div>
              <div className="text-[11px] font-mono text-slate-400 mt-1">{t('FirewallPage:port_label')} {s.portlar}</div>
            </div>
            <button onClick={() => sablonUygula(s)} disabled={!!mesgul}
              className="shrink-0 self-center px-3 py-1.5 text-xs font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
              {mesgul === 'sablon:' + s.key ? '…' : t('FirewallPage:apply')}
            </button>
          </div>
        ))}
      </div>

      {/* ---------- MANUEL KURAL ---------- */}
      <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-2">{t('FirewallPage:manual_title')}</h2>
      <form onSubmit={ekle} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 mb-6">
        {/* 1) ne yapmak istiyorsun */}
        <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">{t('FirewallPage:step1_label')}</div>
        <div className="grid grid-cols-3 gap-2 mb-3">
          {(['ban', 'whitelist', 'kapat'] as const).map(tt => (
            <button key={tt} type="button" onClick={() => setTip(tt)}
              className={`px-3 py-3 text-sm font-medium rounded-lg border text-center transition ${
                tip === tt ? MODLAR[tt].aktifRenk + ' text-white'
                  : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'
              }`}>
              <div className="text-lg leading-none mb-1">{MODLAR[tt].ikon}</div>
              {MODLAR[tt].ad}
            </button>
          ))}
        </div>
        <div className="mb-4 px-3 py-2 rounded-lg bg-slate-50 dark:bg-slate-900/40 text-xs text-slate-600 dark:text-slate-300">
          {mod.aciklama}<br /><span className="text-slate-400">{mod.ornek}</span>
        </div>

        {/* 2) detaylar */}
        <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-2">{t('FirewallPage:step2_label')}</div>
        <div className="grid grid-cols-1 sm:grid-cols-4 gap-3">
          {ipGerekli && (
            <label className="block sm:col-span-2">
              <span className="text-[11px] text-slate-500 dark:text-slate-400">{t('FirewallPage:ip_label')}</span>
              <input value={ip} onChange={e => setIp(e.target.value)} required placeholder={t('FirewallPage:ip_placeholder')}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
          )}
          <label className="block">
            <span className="text-[11px] text-slate-500 dark:text-slate-400">{ipGerekli ? t('FirewallPage:port_optional') : t('FirewallPage:port_required')}</span>
            <input value={port} onChange={e => setPort(e.target.value.replace(/[^0-9]/g, ''))} required={tip === 'kapat'} placeholder={tip === 'kapat' ? t('FirewallPage:port_placeholder_close') : t('FirewallPage:port_placeholder_other')}
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
          <label className="block">
            <span className="text-[11px] text-slate-500 dark:text-slate-400">{t('FirewallPage:protocol_label')}</span>
            <select value={protokol} onChange={e => setProtokol(e.target.value as 'tcp' | 'udp')}
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none">
              <option value="tcp">TCP</option><option value="udp">UDP</option>
            </select>
          </label>
          <label className="block sm:col-span-4">
            <span className="text-[11px] text-slate-500 dark:text-slate-400">{t('FirewallPage:note_label')}</span>
            <input value={aciklama} onChange={e => setAciklama(e.target.value)} placeholder={t('FirewallPage:note_placeholder')}
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          </label>
        </div>

        {/* canlı önizleme */}
        <div className="mt-3 flex items-center gap-2 px-3 py-2 rounded-lg bg-slate-100 dark:bg-slate-900/60 text-xs">
          <span className="text-slate-400">{t('FirewallPage:preview_label')}</span>
          <span className="font-medium text-slate-700 dark:text-slate-200">{onizleme}</span>
        </div>

        {kisitUyari && (
          <div className="mt-2 px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 text-xs text-amber-800 dark:text-amber-200">
            {t('FirewallPage:dynamic_ip_warning')}
          </div>
        )}

        <button disabled={mesgul === 'manuel'} className="mt-3 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
          {mesgul === 'manuel' ? t('FirewallPage:applying') : t('FirewallPage:add_apply')}
        </button>
      </form>

      {/* ---------- AKTİF KURALLAR ---------- */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('FirewallPage:active_rules_title')} {!yuk && <span className="text-slate-400 font-normal">· {kurallar.length}</span>}</h3>
          <button onClick={yukle} disabled={yuk} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">{t('FirewallPage:refresh')}</button>
        </div>
        <div className="lg:overflow-x-auto">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/50 border-b border-slate-200 dark:border-slate-700/60`}>
              <tr>
                <th className={T.baslik}>{t('FirewallPage:col_type')}</th>
                <th className={T.baslik}>{t('FirewallPage:col_ip')}</th>
                <th className={T.baslik}>{t('FirewallPage:col_port')}</th>
                <th className={T.baslik}>{t('FirewallPage:col_proto')}</th>
                <th className={`${T.baslik} w-full`}>{t('FirewallPage:col_note')}</th>
                <th className={`${T.baslik} text-right`}>{t('FirewallPage:col_actions')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-700/60`}>
              {yuk ? (
                <tr><td colSpan={6} className={T.hucreDurum}>{t('FirewallPage:loading')}</td></tr>
              ) : kurallar.length === 0 ? (
                <tr><td colSpan={6} className={T.hucreDurum}>
                  <div className="text-2xl mb-1">🛡️</div>
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('FirewallPage:no_rules')}</p>
                  <p className="text-xs text-slate-400 mt-1">{t('FirewallPage:no_rules_hint')}</p>
                </td></tr>
              ) : (
                kurallar.map(k => (
                  <tr key={k.id} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/40`}>
                    <td className={T.hucreBaslik}>
                      <div className="flex flex-wrap items-center gap-1.5">
                        <TurRozet tip={k.tip} t={t} />
                        {k.kaynak === 'otomatik' && (
                          <span className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300"
                            title={t('FirewallPage:autoban.badge_title')}>
                            {t('FirewallPage:autoban.badge')}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className={T.hucre} data-etiket={t('FirewallPage:col_ip')}><span className="font-mono text-xs text-slate-700 dark:text-slate-200">{k.ip || <span className="text-slate-400">{t('FirewallPage:everyone')}</span>}</span></td>
                    <td className={T.hucre} data-etiket={t('FirewallPage:col_port')}><span className="font-mono text-xs text-slate-600 dark:text-slate-300">{k.port || <span className="text-slate-400">{t('FirewallPage:all_ports')}</span>}</span></td>
                    <td className={T.hucre} data-etiket={t('FirewallPage:col_proto')}><span className="font-mono text-[11px] text-slate-500 uppercase">{k.protokol}</span></td>
                    <td className={T.hucre} data-etiket={t('FirewallPage:col_note')}>
                      <span className="text-xs text-slate-500 dark:text-slate-400">{k.aciklama || t('FirewallPage:empty_note')}</span>
                      {k.bitis_at && (
                        <span className="ml-2 whitespace-nowrap text-[11px] text-slate-400">
                          {t('FirewallPage:autoban.expires_at', { tarih: k.bitis_at })}
                        </span>
                      )}
                    </td>
                    <td className={T.hucreAksiyon}>
                      <button disabled={!!mesgul} onClick={() => sil(k)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">{mesgul === 'sil:' + k.id ? '…' : t('FirewallPage:delete')}</button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

// SayiAlan: otomatik ban eşik/pencere/süre girdileri. Boş bırakılırsa min'e
// düşer — sunucu tarafı da aynı sınırları doğrular (bkz. otoban_handlers.go).
function SayiAlan({ etiket, ipucu, deger, min, max, degistir }: {
  etiket: string; ipucu: string; deger: number; min: number; max: number; degistir: (v: number) => void
}) {
  return (
    <div>
      <label className="block text-xs font-medium text-slate-600 dark:text-slate-300">{etiket}</label>
      <input
        type="number"
        min={min}
        max={max}
        value={deger}
        onChange={e => {
          const v = parseInt(e.target.value, 10)
          degistir(Number.isNaN(v) ? min : Math.min(max, Math.max(min, v)))
        }}
        className="mt-1 w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100"
      />
      <p className="mt-1 text-[11px] text-slate-400">{ipucu}</p>
    </div>
  )
}

function TurRozet({ tip, t }: { tip: Kural['tip']; t: (k: string) => string }) {
  const m = {
    ban: [t('FirewallPage:badge_ban'), 'bg-red-100 dark:bg-red-900/40 text-red-700 dark:text-red-300'],
    whitelist: [t('FirewallPage:badge_whitelist'), 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300'],
    kapat: [t('FirewallPage:badge_close'), 'bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-200'],
  }[tip]
  return <span className={`inline-block text-xs px-2 py-0.5 rounded-full font-medium ${m[1]}`}>{m[0]}</span>
}