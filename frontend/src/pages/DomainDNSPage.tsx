// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'
import { T } from '@/lib/tablo'

type Kayit = {
  id: number
  domain_id: number
  ad: string
  tip: string
  deger: string
  ttl: number
  oncelik: number
  aktif: boolean
  olusturma: string
}

type Domain = { id: number; alan_adi: string; ipv4: string }

type DNSSEC = { aktif: boolean; imzali: boolean; ds: string[]; durum: string }

const TIPLER = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA', 'PTR', 'DS', 'TLSA', 'SSHFP', 'NAPTR']

type Kontrol = { anahtar: string; baslik: string; durum: 'ok' | 'uyari' | 'hata'; beklenen?: string; bulunan?: string; mesaj?: string }
type Dogrulama = { alan_adi: string; kontroller: Kontrol[]; ok_sayisi: number; uyari_sayisi: number; hata_sayisi: number }

export default function DomainDNSPage() {
  const { t } = useTranslation(['DomainDNSPage', 'common'])
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [kayitlar, setKayitlar] = useState<Kayit[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [duzenle, setDuzenle] = useState<Kayit | null>(null)
  const [silinecek, setSilinecek] = useState<Kayit | null>(null)
  const [secili, setSecili] = useState<Set<number>>(new Set())
  const [topluSilOnay, setTopluSilOnay] = useState(false)
  const [soa, setSoa] = useState<{ primary_ns: string; hostmaster: string; refresh: number; retry: number; expire: number; minimum: number; ttl: number } | null>(null)
  const [soaAcik, setSoaAcik] = useState(false)
  const [soaKaydediyor, setSoaKaydediyor] = useState(false)
  const [dnssec, setDnssec] = useState<DNSSEC | null>(null)
  const [dnssecIsliyor, setDnssecIsliyor] = useState(false)
  const [dnssecKapatOnay, setDnssecKapatOnay] = useState(false)
  const [dsKopyalandi, setDsKopyalandi] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true); setHata(null)
    api.get<Kayit[]>(`/domains/${id}/dns`)
      .then(r => { setKayitlar(r.data); setSecili(new Set()) })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }

  function secimDegistir(rid: number) {
    setSecili(prev => {
      const n = new Set(prev)
      if (n.has(rid)) n.delete(rid); else n.add(rid)
      return n
    })
  }
  function hepsiniSec() {
    setSecili(prev => prev.size === kayitlar.length ? new Set() : new Set(kayitlar.map(k => k.id)))
  }

  async function topluSil() {
    if (!id || secili.size === 0) return
    setHata(null); setBasari(null); setTopluSilOnay(false)
    try {
      const { data } = await api.post(`/domains/${id}/dns/toplu-sil`, { ids: [...secili] })
      setBasari(t('DomainDNSPage:success.bulk_deleted', { count: data.silinen }))
      yukle()
    } catch (e) { setHata(apiHata(e, t('DomainDNSPage:errors.bulk_delete_failed'))) }
  }
  async function topluDurum(aktif: boolean) {
    if (!id || secili.size === 0) return
    setHata(null); setBasari(null)
    try {
      const { data } = await api.post(`/domains/${id}/dns/toplu-durum`, { ids: [...secili], aktif })
      setBasari(t('DomainDNSPage:success.bulk_updated', { count: data.guncellenen, durum: aktif ? t('DomainDNSPage:status_active') : t('DomainDNSPage:status_inactive') }))
      yukle()
    } catch (e) { setHata(apiHata(e, t('DomainDNSPage:errors.bulk_update_failed'))) }
  }
  useEffect(() => {
    if (id) {
      api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
      api.get<typeof soa>(`/domains/${id}/dns/soa`).then(r => setSoa(r.data)).catch(() => {})
      api.get<DNSSEC>(`/domains/${id}/dns/dnssec`).then(r => setDnssec(r.data)).catch(() => {})
    }
    yukle()
  }, [id])

  async function dnssecDegistir(aktif: boolean) {
    if (!id) return
    setHata(null); setBasari(null); setDnssecKapatOnay(false); setDnssecIsliyor(true)
    try {
      const { data } = await api.post<DNSSEC>(`/domains/${id}/dns/dnssec`, { aktif })
      setDnssec(data)
      setBasari(aktif
        ? t('DomainDNSPage:dnssec.enabled_msg')
        : t('DomainDNSPage:dnssec.disabled_msg'))
    } catch (e) { setHata(apiHata(e, t('DomainDNSPage:dnssec.update_failed'))) }
    finally { setDnssecIsliyor(false) }
  }
  async function dnssecDurumYenile() {
    if (!id) return
    try { const { data } = await api.get<DNSSEC>(`/domains/${id}/dns/dnssec`); setDnssec(data) } catch { /* yut */ }
  }

  async function soaKaydet(e: React.FormEvent) {
    e.preventDefault()
    if (!id || !soa) return
    setHata(null); setBasari(null); setSoaKaydediyor(true)
    try {
      const { data } = await api.put(`/domains/${id}/dns/soa`, soa)
      setSoa(data)
      setBasari(t('DomainDNSPage:success.soa_saved'))
    } catch (e) { setHata(apiHata(e, t('DomainDNSPage:errors.soa_save_failed'))) }
    finally { setSoaKaydediyor(false) }
  }

  async function sablonUygula() {
    if (!id) return
    setHata(null); setBasari(null)
    try {
      const { data } = await api.post(`/domains/${id}/dns/sablon`)
      setBasari(t('DomainDNSPage:success.template_applied', { count: data.eklenen }))
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainDNSPage:errors.template_apply_failed')))
    }
  }

  async function sil() {
    if (!silinecek || !id) return
    try {
      await api.delete(`/domains/${id}/dns/${silinecek.id}`)
      setSilinecek(null); yukle()
    } catch (e) {
      alert(apiHata(e, t('DomainDNSPage:errors.delete_failed')))
    }
  }

  // Nameserver çifti + DNS doğrulama: ikisi de gerçek dünyadaki durumu gösterir.
  const [ns, setNS] = useState<{ ns1: string; ns2: string; kaynak?: string } | null>(null)
  const [dogrulama, setDogrulama] = useState<Dogrulama | null>(null)
  const [dogrulamaYuk, setDogrulamaYuk] = useState(false)

  useEffect(() => {
    if (!id) return
    api.get(`/domains/${id}/nameserver`).then(r => setNS(r.data)).catch(() => { /* yoksa eski metin */ })
  }, [id])

  async function dogrula() {
    setDogrulamaYuk(true)
    try {
      const { data } = await api.get<Dogrulama>(`/domains/${id}/dns/dogrula`)
      setDogrulama(data)
    } catch { /* sessiz */ } finally { setDogrulamaYuk(false) }
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('DomainDNSPage:breadcrumb.domains'), href: '/domainler' },
        { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainDNSPage:breadcrumb.dns_settings') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainDNSPage:title')}</h1>
      {domain && (
        <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
          <Link to={`/abonelikler/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300 font-medium">{domain.alan_adi}</Link>
          {' · '}{t('DomainDNSPage:ip_label')} <span className="font-mono">{domain.ipv4}</span>
        </p>
      )}

      <div className="bg-sky-50 dark:bg-sky-900/20 border border-sky-200 dark:border-sky-800 rounded-md px-3 py-2 text-xs text-sky-800 dark:text-sky-200 mb-4">
        <strong>{t('DomainDNSPage:info.label')}</strong> {t('DomainDNSPage:info.text1')}<strong>{t('DomainDNSPage:info.authoritative_dns')}</strong>{t('DomainDNSPage:info.text2')}<span className="font-mono">{ns?.ns1 || `ns1.${domain?.alan_adi || t('DomainDNSPage:info.your_domain')}`}</span> / <span className="font-mono">{ns?.ns2 || `ns2.${domain?.alan_adi || t('DomainDNSPage:info.your_domain')}`}</span>{t('DomainDNSPage:info.after')}
      </div>

      {/* DNS doğrulama — kayıtlar GERÇEKTEN yayında mı (public DNS'e sorulur). */}
      <div className="border border-slate-200 dark:border-slate-800 rounded-xl mb-4 overflow-hidden">
        <div className="flex items-center justify-between px-4 py-2.5">
          <div className="text-sm font-medium text-slate-700 dark:text-slate-200">
            {t('DomainDNSPage:verify.title')}
            <span className="ml-2 text-xs text-slate-400 font-normal">{t('DomainDNSPage:verify.hint')}</span>
          </div>
          <div className="flex items-center gap-3">
            {dogrulama && (
              <span className="text-xs text-slate-500">
                <span className="text-emerald-600 dark:text-emerald-400">{dogrulama.ok_sayisi} ✓</span>
                {dogrulama.uyari_sayisi > 0 && <span className="text-amber-600 dark:text-amber-400"> · {dogrulama.uyari_sayisi} !</span>}
                {dogrulama.hata_sayisi > 0 && <span className="text-red-600 dark:text-red-400"> · {dogrulama.hata_sayisi} ✗</span>}
              </span>
            )}
            <button onClick={dogrula} disabled={dogrulamaYuk}
              className="text-xs px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 disabled:opacity-50">
              {dogrulamaYuk ? t('common:loading') : t('DomainDNSPage:verify.button')}
            </button>
          </div>
        </div>
        {dogrulama && (
          <div className="border-t border-slate-100 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800">
            {dogrulama.kontroller.map(k => (
              <div key={k.anahtar} className="px-4 py-2.5 flex items-start gap-3">
                <span className={`mt-0.5 text-sm shrink-0 ${k.durum === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : k.durum === 'uyari' ? 'text-amber-600 dark:text-amber-400' : 'text-red-600 dark:text-red-400'}`}>
                  {k.durum === 'ok' ? '✓' : k.durum === 'uyari' ? '!' : '✗'}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="text-xs font-medium text-slate-700 dark:text-slate-200">{k.baslik}</div>
                  {k.bulunan && <div className="text-[11px] font-mono text-slate-500 dark:text-slate-400 break-all mt-0.5">{k.bulunan}</div>}
                  {k.durum !== 'ok' && k.beklenen && (
                    <div className="text-[11px] text-slate-400 break-all">{t('DomainDNSPage:verify.expected')} <span className="font-mono">{k.beklenen}</span></div>
                  )}
                  {k.mesaj && (
                    <div className={`text-[11px] mt-0.5 ${k.durum === 'hata' ? 'text-red-600 dark:text-red-400' : 'text-amber-700 dark:text-amber-400'}`}>{k.mesaj}</div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {soa && (
        <div className="border border-slate-200 dark:border-slate-800 rounded-xl mb-4 overflow-hidden">
          <button onClick={() => setSoaAcik(v => !v)} className="w-full flex items-center justify-between px-4 py-2.5 text-sm font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-800/50 transition">
            <span>{t('DomainDNSPage:soa.toggle_label')} <span className="text-xs text-slate-400 font-normal">{t('DomainDNSPage:soa.toggle_hint')}</span></span>
            <span className="text-slate-400 text-xs">{soaAcik ? t('DomainDNSPage:soa.hide') : t('DomainDNSPage:soa.edit')}</span>
          </button>
          {soaAcik && (
            <form onSubmit={soaKaydet} className="px-4 pb-4 pt-3 grid grid-cols-2 md:grid-cols-4 gap-3 border-t border-slate-100 dark:border-slate-800">
              <label className="col-span-2">
                <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{t('DomainDNSPage:soa.primary_ns')}</span>
                <input value={soa.primary_ns} onChange={e => setSoa({ ...soa, primary_ns: e.target.value })}
                  className="mt-1 w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono outline-none focus:border-brand-500" />
              </label>
              <label className="col-span-2">
                <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{t('DomainDNSPage:soa.hostmaster')}</span>
                <input value={soa.hostmaster} onChange={e => setSoa({ ...soa, hostmaster: e.target.value })} placeholder="admin@alan.com"
                  className="mt-1 w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono outline-none focus:border-brand-500" />
              </label>
              {(['refresh', 'retry', 'expire', 'minimum', 'ttl'] as const).map(f => (
                <label key={f}>
                  <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{f} {t('DomainDNSPage:soa.seconds_suffix')}</span>
                  <input type="number" min={0} value={soa[f]} onChange={e => setSoa({ ...soa, [f]: parseInt(e.target.value) || 0 })}
                    className="mt-1 w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono outline-none focus:border-brand-500" />
                </label>
              ))}
              <div className="col-span-2 md:col-span-4 flex justify-end">
                <button disabled={soaKaydediyor} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md disabled:opacity-50">
                  {soaKaydediyor ? t('common:saving') : t('DomainDNSPage:soa.save')}
                </button>
              </div>
            </form>
          )}
        </div>
      )}

      {dnssec && (
        <div className="border border-slate-200 dark:border-slate-800 rounded-xl mb-4 overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 gap-3 flex-wrap">
            <div>
              <div className="text-sm font-medium text-slate-700 dark:text-slate-200 flex items-center gap-2">
                🔐 DNSSEC
                {dnssec.aktif ? (
                  dnssec.imzali
                    ? <span className="text-xs px-1.5 py-0.5 rounded bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 font-medium">{t('DomainDNSPage:dnssec.status_signed')}</span>
                    : <span className="text-xs px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 font-medium">{t('DomainDNSPage:dnssec.status_signing')}</span>
                ) : (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 font-medium">{t('DomainDNSPage:dnssec.status_off')}</span>
                )}
              </div>
              <p className="text-xs text-slate-400 dark:text-slate-500 mt-0.5">{t('DomainDNSPage:dnssec.desc')}</p>
            </div>
            <div className="flex items-center gap-2">
              {dnssec.aktif && (
                <button onClick={dnssecDurumYenile} className="px-2.5 py-1.5 text-xs bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 rounded-md transition">{t('DomainDNSPage:dnssec.refresh_status')}</button>
              )}
              {dnssec.aktif ? (
                <button disabled={dnssecIsliyor} onClick={() => setDnssecKapatOnay(true)} className="px-3 py-1.5 text-sm bg-white dark:bg-slate-800 border border-red-200 dark:border-red-800 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 rounded-md transition disabled:opacity-50">{t('DomainDNSPage:dnssec.close')}</button>
              ) : (
                <button disabled={dnssecIsliyor} onClick={() => dnssecDegistir(true)} className="px-3 py-1.5 text-sm bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 font-medium rounded-md transition disabled:opacity-50">{dnssecIsliyor ? t('DomainDNSPage:dnssec.enabling') : t('DomainDNSPage:dnssec.enable')}</button>
              )}
            </div>
          </div>
          {dnssec.aktif && (
            <div className="px-4 pb-4 border-t border-slate-100 dark:border-slate-800 pt-3">
              {dnssec.ds && dnssec.ds.length > 0 ? (
                <div>
                  <div className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-1">{t('DomainDNSPage:dnssec.ds_title')}</div>
                  {dnssec.ds.map((d, i) => (
                    <div key={i} className="flex items-center gap-2 mb-1">
                      <code className="flex-1 text-xs font-mono bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded px-2 py-1 break-all text-slate-800 dark:text-slate-200">{d}</code>
                      <button onClick={() => { navigator.clipboard?.writeText(d); setDsKopyalandi(true); setTimeout(() => setDsKopyalandi(false), 1500) }}
                        className="px-2 py-1 text-xs bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 rounded transition whitespace-nowrap">{dsKopyalandi ? `✓ ${t('common:copied')}` : t('common:copy')}</button>
                    </div>
                  ))}
                  <p className="text-[11px] text-slate-400 dark:text-slate-500 mt-1">{t('DomainDNSPage:dnssec.ds_copy_hint')}</p>
                </div>
              ) : (
                <p className="text-xs text-amber-600 dark:text-amber-400">{t('DomainDNSPage:dnssec.ds_pending')}</p>
              )}
              {dnssec.durum && (
                <pre className="mt-2 text-[10px] font-mono text-slate-500 dark:text-slate-400 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded p-2 overflow-x-auto max-h-44">{dnssec.durum}</pre>
              )}
            </div>
          )}
        </div>
      )}

      <div className="flex items-center gap-2 mb-4">
        <button
          onClick={() => setDuzenle({} as Kayit)}
          className="inline-flex items-center gap-1.5 px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md shadow-sm transition"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          {t('DomainDNSPage:actions.add_record')}
        </button>
        <button
          onClick={sablonUygula}
          className="px-3 py-2 bg-white hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md transition"
          title={t('DomainDNSPage:actions.apply_template_tooltip')}
        >
          {t('DomainDNSPage:actions.apply_template')}
        </button>
        <button onClick={yukle} className="px-3 py-2 bg-white hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md transition">{t('DomainDNSPage:actions.refresh')}</button>
        <span className="ml-auto text-sm text-slate-500 dark:text-slate-500">{t('DomainDNSPage:actions.record_count', { count: kayitlar.length })}</span>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {secili.size > 0 && (
        <div className="mb-3 px-3 py-2 bg-brand-50 dark:bg-brand-900/20 border border-brand-200 dark:border-brand-800 rounded-md flex items-center gap-2 flex-wrap">
          <span className="text-sm font-medium text-brand-800 dark:text-brand-200">{t('DomainDNSPage:selection.selected_count', { count: secili.size })}</span>
          <div className="ml-auto flex items-center gap-2 flex-wrap">
            <button onClick={() => topluDurum(true)} className="px-3 py-1.5 text-sm bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-md text-emerald-700 dark:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/30 transition">{t('DomainDNSPage:selection.make_active')}</button>
            <button onClick={() => topluDurum(false)} className="px-3 py-1.5 text-sm bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700 transition">{t('DomainDNSPage:selection.make_inactive')}</button>
            <button onClick={() => setTopluSilOnay(true)} className="px-3 py-1.5 text-sm bg-red-600 hover:bg-red-700 text-white rounded-md transition">{t('DomainDNSPage:selection.delete_selected', { count: secili.size })}</button>
            <button onClick={() => setSecili(new Set())} className="px-2 py-1.5 text-sm text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 transition">{t('DomainDNSPage:selection.clear_selection')}</button>
          </div>
        </div>
      )}

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
        {yuk ? (
          <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div>
        ) : kayitlar.length === 0 ? (
          <div className="py-12 text-center">
            <p className="text-sm text-slate-500 dark:text-slate-500 mb-3">{t('DomainDNSPage:table.empty')}</p>
            <button onClick={sablonUygula} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md">
              {t('DomainDNSPage:actions.apply_template')}
            </button>
          </div>
        ) : (
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700`}>
              <tr>
                <th className={`${T.baslik} w-10`}>
                  <input type="checkbox" aria-label={t('DomainDNSPage:table.select_all')} checked={kayitlar.length > 0 && secili.size === kayitlar.length}
                    ref={el => { if (el) el.indeterminate = secili.size > 0 && secili.size < kayitlar.length }}
                    onChange={hepsiniSec} className="rounded border-slate-300 dark:border-slate-600 cursor-pointer" />
                </th>
                <th className={T.baslik}>{t('DomainDNSPage:table.name')}</th>
                <th className={T.baslik}>{t('DomainDNSPage:table.type')}</th>
                <th className={T.baslik}>{t('DomainDNSPage:table.value')}</th>
                <th className={T.baslik}>{t('DomainDNSPage:table.ttl')}</th>
                <th className={T.baslik}>{t('DomainDNSPage:table.priority')}</th>
                <th className={T.baslik}>{t('common:status')}</th>
                <th className={`${T.baslik} text-right`}>{t('common:actions')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
              {kayitlar.map(k => (
                <tr key={k.id} className={`${T.satir} ${secili.has(k.id) ? 'lg:bg-brand-50/60 dark:lg:bg-brand-900/10' : 'lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/60'}`}>
                  <td className={T.hucreSecim}>
                    <input type="checkbox" aria-label={t('DomainDNSPage:table.select_row', { ad: k.ad, tip: k.tip })} checked={secili.has(k.id)} onChange={() => secimDegistir(k.id)}
                      className="rounded border-slate-300 dark:border-slate-600 cursor-pointer" />
                  </td>
                  <td className={T.hucreBaslikSecimli}><span className="font-mono lg:text-sm text-base">{k.ad}</span></td>
                  <td className={T.hucre} data-etiket={t('DomainDNSPage:table.type')}>
                    <span className="text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 rounded font-mono font-semibold">{k.tip}</span>
                  </td>
                  <td className={T.hucre} data-etiket={t('DomainDNSPage:table.value')}><span className="font-mono text-sm text-slate-800 dark:text-slate-200 break-all">{k.deger}</span></td>
                  <td className={T.hucre} data-etiket={t('DomainDNSPage:table.ttl')}><span className="font-mono text-sm text-slate-600 dark:text-slate-400">{k.ttl}</span></td>
                  <td className={T.hucre} data-etiket={t('DomainDNSPage:table.priority')}><span className="font-mono text-sm text-slate-600 dark:text-slate-400">{k.tip === 'MX' || k.tip === 'SRV' ? k.oncelik : '—'}</span></td>
                  <td className={T.hucre} data-etiket={t('common:status')}>
                    {k.aktif ? (
                      <span className="text-xs text-emerald-700 dark:text-emerald-300 inline-flex items-center gap-1"><span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>{t('common:active')}</span>
                    ) : (
                      <span className="text-xs text-slate-500 dark:text-slate-500">{t('common:inactive')}</span>
                    )}
                  </td>
                  <td className={T.hucreAksiyon}>
                    <button onClick={() => setDuzenle(k)} className="text-sm text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100 px-2 py-1 rounded hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-800">{t('common:edit')}</button>
                    <button onClick={() => setSilinecek(k)} className="text-sm text-red-600 dark:text-red-400 hover:text-red-700 dark:text-red-300 px-2 py-1 rounded hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20">{t('common:delete')}</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {duzenle && (
        <KayitModal
          mevcut={duzenle}
          domainId={Number(id)}
          ipv4={domain?.ipv4 || ''}
          onKapat={() => setDuzenle(null)}
          onKayit={() => { setDuzenle(null); yukle() }}
        />
      )}

      <ConfirmDialog
        acik={!!silinecek}
        baslik={t('DomainDNSPage:confirm.delete_title')}
        mesaj={t('DomainDNSPage:confirm.delete_msg', { ad: silinecek?.ad, tip: silinecek?.tip, deger: silinecek?.deger.slice(0, 40) })}
        tehlikeli
        onayMetni={t('DomainDNSPage:confirm.delete_confirm')}
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />

      <ConfirmDialog
        acik={topluSilOnay}
        baslik={t('DomainDNSPage:confirm.bulk_delete_title')}
        mesaj={t('DomainDNSPage:confirm.bulk_delete_msg', { count: secili.size })}
        tehlikeli
        onayMetni={t('DomainDNSPage:confirm.bulk_delete_confirm', { count: secili.size })}
        onOnay={topluSil}
        onIptal={() => setTopluSilOnay(false)}
      />

      <ConfirmDialog
        acik={dnssecKapatOnay}
        baslik={t('DomainDNSPage:confirm.dnssec_disable_title')}
        mesaj={t('DomainDNSPage:confirm.dnssec_disable_msg')}
        tehlikeli
        onayMetni={t('DomainDNSPage:confirm.dnssec_disable_confirm')}
        onOnay={() => dnssecDegistir(false)}
        onIptal={() => setDnssecKapatOnay(false)}
      />
    </div>
  )
}

function KayitModal({ mevcut, domainId, ipv4, onKapat, onKayit }: {
  mevcut: Kayit; domainId: number; ipv4: string; onKapat: () => void; onKayit: () => void
}) {
  const { t } = useTranslation(['DomainDNSPage', 'common'])
  const yeni = !mevcut.id
  const [form, setForm] = useState<Kayit>({
    id: mevcut.id || 0,
    domain_id: domainId,
    ad: mevcut.ad || '@',
    tip: mevcut.tip || 'A',
    deger: mevcut.deger || ipv4,
    ttl: mevcut.ttl || 3600,
    oncelik: mevcut.oncelik || 0,
    aktif: mevcut.aktif !== false,
    olusturma: '',
  })
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  async function gonder(e: React.FormEvent) {
    e.preventDefault()
    setIsleniyor(true); setHata(null)
    try {
      if (yeni) await api.post(`/domains/${domainId}/dns`, form)
      else      await api.put(`/domains/${domainId}/dns/${form.id}`, form)
      onKayit()
    } catch (e) {
      setHata(apiHata(e, t('DomainDNSPage:errors.record_save_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={true} baslik={yeni ? t('DomainDNSPage:modal.new_title') : t('DomainDNSPage:modal.edit_title')} onKapat={onKapat} genislik="md">
      <form onSubmit={gonder} className="space-y-3">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div className="col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDNSPage:modal.name_label')}</label>
            <input type="text" value={form.ad} onChange={e => setForm({ ...form, ad: e.target.value })} required
              className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            <p className="text-[10px] text-slate-500 dark:text-slate-500 mt-0.5">{t('DomainDNSPage:modal.name_hint')}</p>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDNSPage:modal.type_label')}</label>
            <select value={form.tip} onChange={e => { const tip = e.target.value; setForm(f => ({ ...f, tip, oncelik: (tip === 'MX' || tip === 'SRV') ? (f.oncelik || 10) : 0 })) }}
              className="w-full px-2 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono bg-white dark:bg-slate-800">
              {TIPLER.map(tp => <option key={tp} value={tp}>{tp}</option>)}
            </select>
          </div>
        </div>

        <div>
          <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDNSPage:modal.value_label')}</label>
          <input type="text" value={form.deger} onChange={e => setForm({ ...form, deger: e.target.value })} required
            className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
          {t(`DomainDNSPage:value_hints.${form.tip}`, { defaultValue: '' }) && <p className="text-[10px] text-slate-500 dark:text-slate-500 mt-0.5">{t(`DomainDNSPage:value_hints.${form.tip}`)}</p>}
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDNSPage:modal.ttl_label')}</label>
            <input type="number" min={60} value={form.ttl} onChange={e => setForm({ ...form, ttl: parseInt(e.target.value) || 3600 })}
              className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono" />
          </div>
          {(form.tip === 'MX' || form.tip === 'SRV') && (
            <div>
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDNSPage:modal.priority_label')}</label>
              <input type="number" min={0} value={form.oncelik} onChange={e => setForm({ ...form, oncelik: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono" />
            </div>
          )}
        </div>

        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.aktif} onChange={e => setForm({ ...form, aktif: e.target.checked })} className="rounded" />
          {t('DomainDNSPage:modal.active_label')}
        </label>

        {hata && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{hata}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onKapat} className="px-4 py-2 border border-slate-200 dark:border-slate-700 rounded-md text-sm">{t('common:cancel')}</button>
          <button type="submit" disabled={isleniyor || !form.deger.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm rounded-md">{isleniyor ? t('common:saving') : (yeni ? t('common:add') : t('DomainDNSPage:modal.update'))}</button>
        </div>
      </form>
    </Modal>
  )
}
