import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { T } from '@/lib/tablo'

type Plan = {
  id: number; ad: string; aciklama: string
  disk_kota_mb: number; trafik_kota_mb: number
  max_domain: number; max_db: number; max_email: number; max_ftp: number
  cpu_yuzde: number; ram_mb: number; max_process: number
  inode_kota: number; io_agirlik: number; mysql_max_baglanti: number
  pm_max_children: number
  io_read_mbps: number; io_write_mbps: number; io_read_iops: number; io_write_iops: number
  db_max_queries_per_hour: number; db_max_updates_per_hour: number; db_max_query_seconds: number
  php_surum: string
  fastcgi_cache: boolean; client_max_body_mb: number; nginx_ek_direktifler: string
  waf_enabled: boolean; waf_mode: string; waf_paranoia: number
  varsayilan: boolean; olusturulma: string
}
type Domain = { id: number; alan_adi: string; sistem_kullanici: string; durum: string; olusturulma: string }
type GetResp = { plan: Plan; domain_sayisi: number }
type Surum = { surum: string; aciklama?: string }

export default function PaketDetayPage() {
  const { t } = useTranslation(['PaketDetayPage', 'common'])
  const { id } = useParams()
  const [plan, setPlan] = useState<Plan | null>(null)
  const [domainSayisi, setDomainSayisi] = useState(0)
  const [domainler, setDomainler] = useState<Domain[]>([])
  const [surumler, setSurumler] = useState<Surum[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [isleniyor, setIsleniyor] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true); setHata(null)
    Promise.all([
      api.get<GetResp>(`/plans/${id}`),
      api.get<Domain[]>(`/plans/${id}/domains`),
    ]).then(([g, d]) => {
      setPlan(g.data.plan)
      setDomainSayisi(g.data.domain_sayisi)
      setDomainler(d.data || [])
    }).catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [id])
  useEffect(() => {
    api.get<Surum[]>('/php/versions').then(r => setSurumler(r.data || [])).catch(() => {})
  }, [])

  async function kaydet() {
    if (!plan) return
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      await api.put(`/plans/${id}`, plan)
      setBasari(t('PaketDetayPage:save_success', { name: plan.ad }))
      setTimeout(() => setBasari(null), 6000)
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('PaketDetayPage:save_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  async function domainicinYenidenUygula(domID: number) {
    if (!plan) return
    setIsleniyor(true)
    try {
      await api.put(`/domains/${domID}/plan`, { plan_id: plan.id })
      setBasari(t('PaketDetayPage:reapply_success', { domain: domainler.find(d => d.id === domID)?.alan_adi }))
      setTimeout(() => setBasari(null), 4000)
    } catch (e) {
      setHata(apiHata(e))
    } finally { setIsleniyor(false) }
  }

  function P<K extends keyof Plan>(k: K, v: Plan[K]) {
    if (!plan) return
    setPlan({ ...plan, [k]: v })
  }

  if (yuk) return <div className="px-6 py-5 text-slate-400">{t('PaketDetayPage:loading')}</div>
  if (!plan) return <div className="px-6 py-5"><div className="text-sm text-red-600">{hata || t('PaketDetayPage:not_found')}</div></div>

  // Kurulu PHP sürümleri + planın mevcut değeri (kurulu olmasa da görünsün)
  const phpOpts = Array.from(new Set([
    ...surumler.map(s => s.surum),
    plan.php_surum,
    ...(surumler.length === 0 ? ['7.4', '8.1', '8.2', '8.3', '8.4'] : []),
  ].filter(Boolean)))

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { etiket: t('common:home'), href: '/' },
          { etiket: t('PaketDetayPage:breadcrumb.tools_settings'), href: '/araclar-ayarlar' },
          { etiket: t('PaketDetayPage:breadcrumb.service_plans'), href: '/araclar/paketler' },
          { etiket: plan.ad },
        ]} />

        {/* Başlık + kaydet (yapışkan) */}
        <div className="sticky top-0 z-10 -mx-2 px-2 py-3 mb-4 bg-slate-50/85 dark:bg-slate-900/85 backdrop-blur border-b border-slate-200/70 dark:border-slate-800 flex items-center justify-between gap-4">
          <div className="min-w-0">
            <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100 flex items-center gap-2 truncate">
              {plan.ad}
              {plan.varsayilan && <span className="shrink-0 text-[10px] uppercase font-semibold tracking-wider bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 px-1.5 py-0.5 rounded">{t('PaketDetayPage:default_badge')}</span>}
            </h1>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 truncate">
              {plan.aciklama || t('PaketDetayPage:no_description')} · <span className="font-mono">{domainSayisi}</span> {t('PaketDetayPage:used_in_domains_inline')}
            </p>
          </div>
          <button onClick={kaydet} disabled={isleniyor}
            className="shrink-0 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-lg shadow-sm">
            {isleniyor ? t('PaketDetayPage:save_button_saving') : t('PaketDetayPage:save_button')}
          </button>
        </div>

        {hata && <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
        {basari && <div className="mb-4 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

        {/* Genel */}
        <Kart baslik={t('PaketDetayPage:general.title')} ikon="⚙️">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Alan etiket={t('PaketDetayPage:general.plan_name')}>
              <input value={plan.ad} onChange={e => P('ad', e.target.value)} className={inp} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:general.default_plan')}>
              <label className="flex items-center gap-2 h-[38px] px-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-slate-50/60 dark:bg-slate-900/40 cursor-pointer">
                <input type="checkbox" checked={plan.varsayilan} onChange={e => P('varsayilan', e.target.checked)} className="rounded" />
                <span className="text-sm text-slate-700 dark:text-slate-300">{t('PaketDetayPage:general.default_plan_hint')}</span>
              </label>
            </Alan>
            <Alan etiket={t('PaketDetayPage:general.description')} span={2}>
              <textarea value={plan.aciklama} onChange={e => P('aciklama', e.target.value)} rows={2} className={inp} />
            </Alan>
          </div>
        </Kart>

        {/* Varsayılanlar — yeni domainler bu değerleri miras alır */}
        <Kart baslik={t('PaketDetayPage:defaults.title')} ikon="🧩" alt={t('PaketDetayPage:defaults.desc')}>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Alan etiket={t('PaketDetayPage:defaults.php_version')} ipucu={t('PaketDetayPage:defaults.php_version_hint')}>
              <select value={plan.php_surum} onChange={e => P('php_surum', e.target.value)} className={inp}>
                {phpOpts.map(v => <option key={v} value={v}>PHP {v}</option>)}
              </select>
            </Alan>
          </div>
        </Kart>

        {/* Kaynak Limitleri */}
        <Kart baslik={t('PaketDetayPage:resource_limits.title')} ikon="📊" alt={t('PaketDetayPage:resource_limits.desc')}>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <Alan etiket={t('PaketDetayPage:resource_limits.cpu')} ipucu={t('PaketDetayPage:resource_limits.cpu_hint')}>
              <input type="number" min={10} max={2000} value={plan.cpu_yuzde} onChange={e => P('cpu_yuzde', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.ram')} ipucu={t('PaketDetayPage:resource_limits.ram_hint')}>
              <input type="number" min={64} value={plan.ram_mb} onChange={e => P('ram_mb', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.max_process')} ipucu={t('PaketDetayPage:resource_limits.max_process_hint')}>
              <input type="number" min={5} value={plan.max_process} onChange={e => P('max_process', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.mysql_connections')} ipucu={t('PaketDetayPage:resource_limits.mysql_connections_hint')}>
              <input type="number" min={1} value={plan.mysql_max_baglanti} onChange={e => P('mysql_max_baglanti', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.disk')} ipucu={t('PaketDetayPage:resource_limits.disk_hint')}>
              <input type="number" min={0} value={plan.disk_kota_mb} onChange={e => P('disk_kota_mb', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.traffic')} ipucu={t('PaketDetayPage:resource_limits.traffic_hint')}>
              <input type="number" min={0} value={plan.trafik_kota_mb} onChange={e => P('trafik_kota_mb', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.inode_quota')} ipucu={t('PaketDetayPage:resource_limits.inode_quota_hint')}>
              <input type="number" min={1000} value={plan.inode_kota} onChange={e => P('inode_kota', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.io_weight')} ipucu={t('PaketDetayPage:resource_limits.io_weight_hint')}>
              <input type="number" min={1} max={1000} value={plan.io_agirlik} onChange={e => P('io_agirlik', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.pm_max_children')} ipucu={t('PaketDetayPage:resource_limits.pm_max_children_hint')}>
              <input type="number" min={0} value={plan.pm_max_children} onChange={e => P('pm_max_children', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.pm_max_children_placeholder')} />
            </Alan>
          </div>
          <div className="mt-4 text-xs font-medium text-slate-500">{t('PaketDetayPage:resource_limits.io_section_label')}</div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mt-2">
            <Alan etiket={t('PaketDetayPage:resource_limits.io_read_mbps')} ipucu={t('PaketDetayPage:resource_limits.io_read_mbps_hint')}>
              <input type="number" min={0} value={plan.io_read_mbps} onChange={e => P('io_read_mbps', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.io_read_mbps_placeholder')} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.io_write_mbps')} ipucu={t('PaketDetayPage:resource_limits.io_write_mbps_hint')}>
              <input type="number" min={0} value={plan.io_write_mbps} onChange={e => P('io_write_mbps', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.io_write_mbps_placeholder')} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.io_read_iops')} ipucu={t('PaketDetayPage:resource_limits.io_read_iops_hint')}>
              <input type="number" min={0} value={plan.io_read_iops} onChange={e => P('io_read_iops', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.io_read_iops_placeholder')} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.io_write_iops')} ipucu={t('PaketDetayPage:resource_limits.io_write_iops_hint')}>
              <input type="number" min={0} value={plan.io_write_iops} onChange={e => P('io_write_iops', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.io_write_iops_placeholder')} />
            </Alan>
          </div>
          <div className="mt-4 text-xs font-medium text-slate-500">{t('PaketDetayPage:resource_limits.db_section_label')}</div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mt-2">
            <Alan etiket={t('PaketDetayPage:resource_limits.db_max_queries')} ipucu={t('PaketDetayPage:resource_limits.db_max_queries_hint')}>
              <input type="number" min={0} value={plan.db_max_queries_per_hour} onChange={e => P('db_max_queries_per_hour', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.db_max_queries_placeholder')} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.db_max_updates')} ipucu={t('PaketDetayPage:resource_limits.db_max_updates_hint')}>
              <input type="number" min={0} value={plan.db_max_updates_per_hour} onChange={e => P('db_max_updates_per_hour', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.db_max_updates_placeholder')} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:resource_limits.db_max_query_seconds')} ipucu={t('PaketDetayPage:resource_limits.db_max_query_seconds_hint')}>
              <input type="number" min={0} value={plan.db_max_query_seconds} onChange={e => P('db_max_query_seconds', Number(e.target.value) || 0)} className={inpNum} placeholder={t('PaketDetayPage:resource_limits.db_max_query_seconds_placeholder')} />
            </Alan>
          </div>
        </Kart>

        {/* Sayısal Sınırlar (E-posta kaldırıldı) */}
        <Kart baslik={t('PaketDetayPage:numeric_limits.title')} ikon="🔢" alt={t('PaketDetayPage:numeric_limits.desc')}>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            <Alan etiket={t('PaketDetayPage:numeric_limits.domain')}>
              <input type="number" min={0} value={plan.max_domain} onChange={e => P('max_domain', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:numeric_limits.database')}>
              <input type="number" min={0} value={plan.max_db} onChange={e => P('max_db', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
            <Alan etiket={t('PaketDetayPage:numeric_limits.ftp')}>
              <input type="number" min={0} value={plan.max_ftp} onChange={e => P('max_ftp', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
          </div>
        </Kart>

        {/* Web Sunucusu (nginx) */}
        <Kart baslik={t('PaketDetayPage:web_server.title')} ikon="🛠️" alt={t('PaketDetayPage:web_server.desc')}>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
            <Alan etiket={t('PaketDetayPage:web_server.fastcgi_cache')} ipucu={t('PaketDetayPage:web_server.fastcgi_cache_hint')}>
              <label className="flex items-center gap-2 h-[38px] px-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-slate-50/60 dark:bg-slate-900/40 cursor-pointer">
                <input type="checkbox" checked={plan.fastcgi_cache} onChange={e => P('fastcgi_cache', e.target.checked)} className="rounded" />
                <span className="text-sm text-slate-700 dark:text-slate-300">{t('PaketDetayPage:web_server.fastcgi_cache_label')}</span>
              </label>
            </Alan>
            <Alan etiket={t('PaketDetayPage:web_server.upload_size')} ipucu={t('PaketDetayPage:web_server.upload_size_hint')}>
              <input type="number" min={1} max={4096} value={plan.client_max_body_mb} onChange={e => P('client_max_body_mb', Number(e.target.value) || 0)} className={inpNum} />
            </Alan>
          </div>
          <Alan etiket={t('PaketDetayPage:web_server.extra_directives')} ipucu={t('PaketDetayPage:web_server.extra_directives_hint')}>
            <textarea
              value={plan.nginx_ek_direktifler || ''}
              onChange={e => P('nginx_ek_direktifler', e.target.value)}
              rows={6}
              spellCheck={false}
              placeholder={t('PaketDetayPage:web_server.extra_directives_placeholder')}
              className={inp + ' font-mono text-xs leading-relaxed'}
            />
          </Alan>
          <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
            {t('PaketDetayPage:web_server.extra_directives_note')}
          </p>
        </Kart>

        {/* WAF (ModSecurity + OWASP CRS) plan varsayılanı */}
        <Kart baslik={t('PaketDetayPage:waf.title')} ikon="🛡️" alt={t('PaketDetayPage:waf.desc')}>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Alan etiket={t('PaketDetayPage:waf.waf_default')} ipucu={t('PaketDetayPage:waf.waf_default_hint')}>
              <label className="flex items-center gap-2 h-[38px] px-3 border border-slate-200 dark:border-slate-700 rounded-lg bg-slate-50/60 dark:bg-slate-900/40 cursor-pointer">
                <input type="checkbox" checked={plan.waf_enabled} onChange={e => P('waf_enabled', e.target.checked)} className="rounded" />
                <span className="text-sm text-slate-700 dark:text-slate-300">{t('PaketDetayPage:waf.waf_default_label')}</span>
              </label>
            </Alan>
            <Alan etiket={t('PaketDetayPage:waf.mode')} ipucu={t('PaketDetayPage:waf.mode_hint')}>
              <select value={plan.waf_mode} onChange={e => P('waf_mode', e.target.value)} className={inp} disabled={!plan.waf_enabled}>
                <option value="on">{t('PaketDetayPage:waf.mode_block')}</option>
                <option value="detect">{t('PaketDetayPage:waf.mode_detect')}</option>
              </select>
            </Alan>
            <Alan etiket={t('PaketDetayPage:waf.paranoia')} ipucu={t('PaketDetayPage:waf.paranoia_hint')}>
              <select value={plan.waf_paranoia} onChange={e => P('waf_paranoia', Number(e.target.value) || 1)} className={inp} disabled={!plan.waf_enabled}>
                <option value={1}>{t('PaketDetayPage:waf.paranoia_level_1')}</option>
                <option value={2}>{t('PaketDetayPage:waf.paranoia_level_2')}</option>
                <option value={3}>{t('PaketDetayPage:waf.paranoia_level_3')}</option>
                <option value={4}>{t('PaketDetayPage:waf.paranoia_level_4')}</option>
              </select>
            </Alan>
          </div>
          <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
            {t('PaketDetayPage:waf.note')}
          </p>
        </Kart>

        {/* Atanmış domainler */}
        <Kart baslik={t('PaketDetayPage:assigned_domains.title', { count: domainler.length })} ikon="🌐" alt={t('PaketDetayPage:assigned_domains.desc')}>
          {domainler.length === 0 ? (
            <div className="text-sm text-slate-400 py-6 text-center">{t('PaketDetayPage:assigned_domains.empty')}</div>
          ) : (
            <div className="lg:overflow-x-auto">
              <table className={T.tablo}>
                <thead className={`${T.baslikGrubu} border-b border-slate-200 dark:border-slate-700`}>
                  <tr>
                    <th className={T.baslik}>{t('PaketDetayPage:assigned_domains.col_domain')}</th>
                    <th className={T.baslik}>{t('PaketDetayPage:assigned_domains.col_system_user')}</th>
                    <th className={T.baslik}>{t('PaketDetayPage:assigned_domains.col_status')}</th>
                    <th className={T.baslik}>{t('PaketDetayPage:assigned_domains.col_created')}</th>
                    <th className={`${T.baslik} text-right`}>{t('PaketDetayPage:assigned_domains.col_action')}</th>
                  </tr>
                </thead>
                <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
                  {domainler.map(d => (
                    <tr key={d.id} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/60`}>
                      <td className={T.hucreBaslik}><Link to={`/abonelikler/${d.id}`} className="text-brand-600 dark:text-brand-400 font-medium">{d.alan_adi}</Link></td>
                      <td className={T.hucre} data-etiket={t('PaketDetayPage:assigned_domains.col_system_user')}><span className="font-mono text-xs">{d.sistem_kullanici}</span></td>
                      <td className={T.hucre} data-etiket={t('PaketDetayPage:assigned_domains.col_status')}>
                        <span className={`text-[10px] uppercase tracking-wider px-2 py-0.5 rounded font-semibold ${
                          d.durum === 'aktif' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-100 dark:bg-slate-700 text-slate-500'
                        }`}>{d.durum}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('PaketDetayPage:assigned_domains.col_created')}><span className="font-mono text-xs text-slate-500">{d.olusturulma}</span></td>
                      <td className={T.hucreAksiyon}>
                        <button onClick={() => domainicinYenidenUygula(d.id)} disabled={isleniyor}
                          className="text-xs px-2 py-1 border border-slate-300 dark:border-slate-600 rounded-md hover:bg-slate-50 dark:hover:bg-slate-800">
                          {t('PaketDetayPage:assigned_domains.reapply')}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Kart>
      </div>
    </div>
  )
}

const inp = 'w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-800 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none'
const inpNum = inp + ' font-mono'

function Kart({ baslik, alt, ikon, children }: { baslik: string; alt?: string; ikon?: string; children: React.ReactNode }) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
      <div className="flex items-center gap-2 mb-1">
        {ikon && <span className="text-base leading-none" aria-hidden>{ikon}</span>}
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{baslik}</h3>
      </div>
      {alt && <p className="text-xs text-slate-500 dark:text-slate-400 mb-4 max-w-2xl">{alt}</p>}
      {children}
    </div>
  )
}

function Alan({ etiket, ipucu, span, children }: { etiket: string; ipucu?: string; span?: number; children: React.ReactNode }) {
  return (
    <label className={`block ${span === 2 ? 'sm:col-span-2' : ''}`}>
      <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{etiket}</span>
      {ipucu && <span className="text-[10px] text-slate-400 dark:text-slate-500 ml-1 cursor-help" title={ipucu}>ⓘ</span>}
      <div className="mt-1.5">{children}</div>
    </label>
  )
}