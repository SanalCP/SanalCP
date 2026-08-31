// sanal-dark-swept
// sanal-dark-swept-v2
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ResourceCard from '@/components/ResourceCard'
import DomainKaynakKart from '@/components/DomainKaynakKart'
import DomainPano from "@/components/DomainPano"
import type { Domain } from '@/components/DomainList'
import { useAuth } from '@/store/auth'

type Plan = {
  id: number
  ad: string
}

const ICONS = {
  baglanti:  'M13.828 10.172a4 4 0 015.656 5.656l-3 3a4 4 0 01-5.656-5.656m.172-5.172a4 4 0 00-5.656 5.656l-3 3a4 4 0 005.656 5.656',
  dosyalar:  'M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V7z',
  db:        'M4 7c0-1.657 3.582-3 8-3s8 1.343 8 3-3.582 3-8 3-8-1.343-8-3zm0 0v10c0 1.657 3.582 3 8 3s8-1.343 8-3V7M4 12c0 1.657 3.582 3 8 3s8-1.343 8-3',
  yedek:     'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1M16 12l-4 4-4-4M12 16V4',
  posta:     'M3 8l9 6 9-6m-9 6V4m0 0v16',
  dns:       'M21 12a9 9 0 11-18 0 9 9 0 0118 0zM3 12h18M12 3a14 14 0 010 18M12 3a14 14 0 000 18',
}

export default function SubscriptionDetailPage() {
  const { t } = useTranslation(['SubscriptionDetailPage', 'common'])
  const { id } = useParams()
  const navigate = useNavigate()
  const adminMi = useAuth((s) => s.kullanici?.rol) === 'admin'
  const [domain, setDomain] = useState<Domain | null>(null)
  const [planlar, setPlanlar] = useState<Plan[]>([])
  const [seciliPlanID, setSeciliPlanID] = useState<number | ''>('')
  const [hata, setHata] = useState<string | null>(null)
  const [diskMB, setDiskMB] = useState<number | null>(null)
  const [menuAcik, setMenuAcik] = useState(false)
  const [isleniyor, setIsleniyor] = useState(false)
  const [bildirim, setBildirim] = useState<string | null>(null)

  const domainYukle = useCallback(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`)
      .then(r => setDomain(r.data))
      .catch(e => setHata(apiHata(e, t('SubscriptionDetailPage:subscription_load_failed'))))
  }, [id, t])

  useEffect(() => {
    if (!id) return
    domainYukle()
    api.get<{ disk_mb: { kullanim: number } }>(`/domains/${id}/kaynak`)
      .then(r => setDiskMB(r.data.disk_mb.kullanim))
      .catch(() => {})
  }, [id, domainYukle])

  useEffect(() => {
    if (!adminMi) return
    api.get<Plan[]>('/plans')
      .then(r => setPlanlar(r.data || []))
      .catch(e => setHata(apiHata(e, t('SubscriptionDetailPage:plans_load_failed'))))
  }, [adminMi, t])

  useEffect(() => {
    setSeciliPlanID(domain?.plan_id ?? '')
  }, [domain?.plan_id])

  async function askiToggle() {
    if (!id || !domain) return
    const askiyaAl = !domain.askida
    if (askiyaAl && !window.confirm(t('SubscriptionDetailPage:suspend_confirm', { domain: domain.alan_adi }))) return
    setMenuAcik(false); setIsleniyor(true); setHata(null); setBildirim(null)
    try {
      await api.post(`/domains/${id}/${askiyaAl ? 'askiya-al' : 'askidan-al'}`)
      setBildirim(askiyaAl ? t('SubscriptionDetailPage:suspended_success') : t('SubscriptionDetailPage:unsuspended_success'))
      setTimeout(() => setBildirim(null), 6000)
      domainYukle()
    } catch (e) { setHata(apiHata(e, t('SubscriptionDetailPage:operation_failed'))) }
    finally { setIsleniyor(false) }
  }

  async function paketDegistir() {
    if (!id || !domain || seciliPlanID === '' || seciliPlanID === domain.plan_id) return
    const yeniPlan = planlar.find(p => p.id === seciliPlanID)
    if (!yeniPlan || !window.confirm(t('SubscriptionDetailPage:change_plan_confirm', {
      domain: domain.alan_adi,
      current: domain.plan_ad || t('SubscriptionDetailPage:no_plan'),
      next: yeniPlan.ad,
    }))) return

    setIsleniyor(true); setHata(null); setBildirim(null)
    try {
      await api.put(`/domains/${id}/plan`, { plan_id: yeniPlan.id })
      setBildirim(t('SubscriptionDetailPage:change_plan_success', { plan: yeniPlan.ad }))
      setTimeout(() => setBildirim(null), 6000)
      domainYukle()
    } catch (e) {
      setHata(apiHata(e, t('SubscriptionDetailPage:change_plan_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  if (hata && !domain) return (
    <div className="ta-page-fluid">
      <Breadcrumb items={[{ etiket: 'Anasayfa', href: '/' }, { etiket: 'Domainler', href: '/domainler' }, { etiket: 'Hata' }]} />
      <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md p-4 text-sm text-red-700 dark:text-red-300">{hata}</div>
    </div>
  )

  if (!domain) return (
    <div className="ta-page-fluid">
      <Breadcrumb items={[{ etiket: 'Anasayfa', href: '/' }, { etiket: 'Domainler', href: '/domainler' }]} />
      <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('SubscriptionDetailPage:loading')}</div>
    </div>
  )

  return (
    <div className="ta-page-fluid">
      <Breadcrumb items={[
        { etiket: 'Anasayfa', href: '/' },
        { etiket: 'Domainler', href: '/domainler' },
        { etiket: domain.alan_adi },
      ]} />

      <p className="ta-eyebrow">Domain yönetimi</p>
      <div className="mb-5 flex flex-wrap items-center gap-3">
        <h1 className="ta-page-title text-brand-700 dark:text-brand-300">{domain.alan_adi}</h1>
        <button
          onClick={() => navigate('/abonelikler')}
          className="ta-icon-button"
          title={t('SubscriptionDetailPage:switch_subscription')}
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {domain.askida ? (
          <span className="text-[10px] px-2 py-0.5 rounded uppercase font-semibold tracking-wider flex items-center gap-1 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300">
            <span className="w-1.5 h-1.5 rounded-full bg-red-500"></span>
            {t('SubscriptionDetailPage:suspended')}
          </span>
        ) : (
          <span className={`text-[10px] px-2 py-0.5 rounded uppercase font-semibold tracking-wider flex items-center gap-1 ${
            domain.durum === 'aktif' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-200 text-slate-600 dark:text-slate-400'
          }`}>
            <span className={`w-1.5 h-1.5 rounded-full ${domain.durum === 'aktif' ? 'bg-emerald-500' : 'bg-slate-400'}`}></span>
            {domain.durum}
          </span>
        )}
        <div className="relative ml-1">
          <button
            onClick={() => setMenuAcik(v => !v)}
            disabled={isleniyor}
            className="ta-icon-button disabled:opacity-50"
            title={t('SubscriptionDetailPage:more_actions')}>
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
              <circle cx="12" cy="5" r="1.5" /><circle cx="12" cy="12" r="1.5" /><circle cx="12" cy="19" r="1.5" />
            </svg>
          </button>
          {menuAcik && (
            <>
              {/* Backdrop: dropdown'ı dışarı tıklayınca kapatır, dekoratif — gerçek menü
                  öğeleri aşağıda native <button>, zaten klavye erişilebilir. */}
              {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
              <div className="fixed inset-0 z-10" onClick={() => setMenuAcik(false)} />
              <div className="absolute left-0 mt-1 z-20 w-56 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-xl shadow-lg py-1 text-sm">
                <button
                  onClick={askiToggle}
                  className={`w-full text-left px-3 py-2 flex items-center gap-2 hover:bg-slate-50 dark:hover:bg-slate-700/60 ${domain.askida ? 'text-emerald-700 dark:text-emerald-300' : 'text-red-600 dark:text-red-400'}`}>
                  {domain.askida ? (
                    <>
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M5 3l14 9-14 9V3z" /></svg>
                      {t('SubscriptionDetailPage:suspended')}n Al (Geri Getir)
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M10 9v6m4-6v6M9 3h6a2 2 0 012 2v0H7v0a2 2 0 012-2z" /></svg>
                      {t('SubscriptionDetailPage:suspend')}
                    </>
                  )}
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {bildirim && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{bildirim}</div>}
      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      <div className="mb-6 rounded-xl border border-slate-200 bg-white p-3 shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <h2 className="mb-2 px-1 text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-500">{t('SubscriptionDetailPage:quick_links')}</h2>
        <div className="flex items-center gap-2 overflow-x-auto">
          <QuickLink to={`/abonelikler/${domain.id}/baglanti`} ikon={ICONS.baglanti}>{t('SubscriptionDetailPage:quick_link_connection')}</QuickLink>
          <QuickLink to={`/abonelikler/${domain.id}/dns`} ikon={ICONS.dns}>{t('SubscriptionDetailPage:quick_link_dns')}</QuickLink>
          <QuickLink to={`/abonelikler/${domain.id}/mail`} ikon={ICONS.posta}>{t('SubscriptionDetailPage:quick_link_mail')}</QuickLink>
          <QuickLink to={`/abonelikler/${domain.id}/dosyalar`} ikon={ICONS.dosyalar}>{t('SubscriptionDetailPage:quick_link_files')}</QuickLink>
          <QuickLink to={`/abonelikler/${domain.id}/veritabanlari`} ikon={ICONS.db}>{t('SubscriptionDetailPage:quick_link_databases')}</QuickLink>
          <QuickLink to={`/abonelikler/${domain.id}/yedekler`} ikon={ICONS.yedek}>{t('SubscriptionDetailPage:quick_link_backup')}</QuickLink>
        </div>
      </div>

      <div className="grid grid-cols-12 gap-5">
        <aside className="order-2 col-span-12 space-y-4 md:col-span-6 xl:order-1 xl:col-span-3">
          <WebSitePreview alanAdi={domain.alan_adi} ssl={domain.ssl} />

          <div className="ta-card p-5">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('SubscriptionDetailPage:statistics')}</h3>
              <button type="button" aria-label={t('SubscriptionDetailPage:refresh')} className="ta-icon-button !h-8 !w-8 !rounded-lg" title={t('SubscriptionDetailPage:refresh')}>
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
              </button>
            </div>
            <div className="space-y-2.5 text-sm">
              <Stat e={t('SubscriptionDetailPage:disk_space')}    d={diskMB != null ? `${diskMB} MB` : '…'} />
              <Stat e={t('SubscriptionDetailPage:monthly_traffic')}  d={`${Math.round(domain.trafik_kb / 1024)} MB`} />
              <Stat e={t('SubscriptionDetailPage:created')}   d={domain.olusturulma} />
              <Stat e={t('SubscriptionDetailPage:php_version')}    d={domain.php_surum} />
            </div>

            {adminMi && (
              <div className="mt-4 border-t border-slate-100 pt-4 dark:border-slate-800">
                <label htmlFor="domain-plan" className="ta-label-sm">{t('SubscriptionDetailPage:service_plan')}</label>
                <div className="mt-1.5 flex gap-2">
                  <select
                    id="domain-plan"
                    value={seciliPlanID}
                    onChange={e => setSeciliPlanID(e.target.value === '' ? '' : Number(e.target.value))}
                    disabled={isleniyor}
                    className="ta-input ta-input-sm min-w-0 flex-1"
                  >
                    <option value="" disabled>{t('SubscriptionDetailPage:no_plan')}</option>
                    {planlar.map(p => <option key={p.id} value={p.id}>{p.ad}</option>)}
                  </select>
                  <button
                    type="button"
                    onClick={paketDegistir}
                    disabled={isleniyor || seciliPlanID === '' || seciliPlanID === domain.plan_id}
                    className="ta-primary-button whitespace-nowrap disabled:opacity-50"
                  >
                    {isleniyor ? t('SubscriptionDetailPage:changing_plan') : t('SubscriptionDetailPage:change_plan')}
                  </button>
                </div>
                <p className="ta-hint mt-1.5">{t('SubscriptionDetailPage:change_plan_hint')}</p>
              </div>
            )}
          </div>
        </aside>

        <section className="order-1 col-span-12 xl:order-2 xl:col-span-6">
          <DomainPano domain={domain} />

          <div className="mt-5 pt-3 border-t border-slate-100 dark:border-slate-800 flex items-center justify-between text-xs text-slate-500 dark:text-slate-500 flex-wrap gap-2">
            <div className="flex items-center gap-4">
              <span>Web sitesi: <span className="font-mono text-slate-700 dark:text-slate-300">httpdocs</span></span>
              <span>IP: <span className="font-mono text-slate-700 dark:text-slate-300">{domain.ipv4}</span></span>
              <span>{t('SubscriptionDetailPage:system_user')}: <span className="font-mono text-slate-700 dark:text-slate-300">{domain.sistem_kullanici}</span></span>
            </div>
            <button className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300">{t('SubscriptionDetailPage:add_description')}</button>
          </div>
        </section>

        <aside className="order-3 col-span-12 md:col-span-6 xl:col-span-3">
          <DomainKaynakKart domainId={domain.id} />
        </aside>
      </div>
    </div>
  )
}

function WebSitePreview({ alanAdi, ssl }: { alanAdi: string; ssl: boolean }) {
  const { t } = useTranslation('SubscriptionDetailPage')
  const url = `${ssl ? 'https' : 'http'}://${alanAdi}`
  const [previewVersion, setPreviewVersion] = useState(() => Date.now())

  // Domain sayfasına her girişte ve hedef değiştiğinde iframe'i taze bir URL ile
  // yeniden yükle. Cache-buster yalnız önizlemeye aittir; gerçek site URL'sini değiştirmez.
  useEffect(() => {
    setPreviewVersion(Date.now())
  }, [alanAdi, ssl])

  const previewURL = `${url}/?sanalcp_preview=${previewVersion}`
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
      <div className="relative aspect-[4/3] bg-gradient-to-br from-slate-800 to-slate-900 overflow-hidden">
        {ssl ? (
          <div className="absolute inset-0 overflow-hidden pointer-events-none">
            <iframe
              key={previewVersion}
              src={previewURL}
              title={t('SubscriptionDetailPage:preview', { domain: alanAdi })}
              loading="lazy"
              sandbox="allow-scripts allow-same-origin"
              tabIndex={-1}
              aria-hidden
              className="origin-top-left"
              style={{ width: '400%', height: '400%', transform: 'scale(0.25)', border: 0, background: '#fff' }}
            />
          </div>
        ) : (
          <div className="absolute inset-0 flex flex-col items-center justify-center text-center px-4">
            <svg className="w-9 h-9 text-white/40 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.542 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" /></svg>
            <div className="text-[11px] text-white/60">{t('SubscriptionDetailPage:https_only')}</div>
            <div className="text-[10px] text-white/40 mt-0.5">{t('SubscriptionDetailPage:ssl_preview')}</div>
          </div>
        )}
        <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent p-3 flex items-center justify-between gap-2">
          <div className="min-w-0">
            <div className="text-[9px] uppercase tracking-wider text-white/60">Web Sitesi</div>
            <div className="text-xs font-semibold text-white truncate">{alanAdi}</div>
          </div>
          <div className="shrink-0 flex items-center gap-1.5">
            <button type="button" onClick={() => setPreviewVersion(Date.now())} disabled={!ssl}
              title={ssl ? t('SubscriptionDetailPage:refresh_preview') : t('SubscriptionDetailPage:ssl_required')}
              className="inline-flex items-center gap-1 text-[11px] bg-white/15 hover:bg-white/25 text-white px-2 py-1 rounded-md font-medium transition disabled:opacity-40 disabled:cursor-not-allowed">
              <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h5M20 20v-5h-5M5.5 15a7 7 0 0011.9 2M18.5 9A7 7 0 006.6 7" />
              </svg>
              Yenile
            </button>
            <a href={url} target="_blank" rel="noreferrer"
              className="inline-flex items-center gap-1 text-[11px] bg-white/90 hover:bg-white text-slate-900 px-2 py-1 rounded-md font-medium transition">
            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
            {t('SubscriptionDetailPage:open')}
            </a>
          </div>
        </div>
      </div>
    </div>
  )
}

function Stat({ e, d }: { e: string; d: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-slate-500 dark:text-slate-500">{e}</span>
      <span className="text-slate-800 dark:text-slate-200 font-medium font-mono">{d}</span>
    </div>
  )
}

function QuickLink({ to, ikon, children }: { to: string; ikon: string; children: React.ReactNode }) {
  return (
    <Link
      to={to}
      className="inline-flex shrink-0 items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm font-medium text-slate-700 transition hover:border-brand-300 hover:bg-brand-50 hover:text-brand-700 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 dark:hover:border-brand-700 dark:hover:bg-brand-900/20 dark:hover:text-brand-300"
    >
      <svg className="h-4 w-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d={ikon} />
      </svg>
      {children}
    </Link>
  )
}
