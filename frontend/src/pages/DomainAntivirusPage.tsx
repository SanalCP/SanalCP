import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { T } from '@/lib/tablo'

type Domain = { id: number; alan_adi: string }
type Bulgu = { id: number; dosya: string; imza: string; motor: string; karantina: number; puan: number; risk: string; gerekceler: string[]; sha256: string; istisna: number; karantina_yolu: string }
type Tarama = { id: number; durum: string; motor: string; taranan: number; enfekte: number; baslangic: string; bitis: string }
type Durum = { clamav: boolean; imza_tarihi: string; kullanici: string; son_tarama: Tarama | null; bulgular: Bulgu[] }

export default function DomainAntivirusPage() {
  const { t } = useTranslation(['DomainAntivirusPage', 'common'])
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [d, setD] = useState<Durum | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [tarariyor, setTarariyor] = useState(false)
  const [imzaYuk, setImzaYuk] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
  }, [id])

  const startPoll = useCallback((sid: number) => {
    setTarariyor(true)
    if (pollRef.current) clearInterval(pollRef.current)
    pollRef.current = setInterval(async () => {
      try {
        const { data } = await api.get<Tarama & { bulgular: Bulgu[] }>(`/domains/${id}/antivirus/tara/${sid}`)
        if (data.durum !== 'calisiyor') {
          if (pollRef.current) clearInterval(pollRef.current)
          setTarariyor(false)
          api.get<Durum>(`/domains/${id}/antivirus`).then(r => setD(r.data)).catch(() => {})
        }
      } catch { if (pollRef.current) clearInterval(pollRef.current); setTarariyor(false) }
    }, 2500)
  }, [id])

  const yukle = useCallback(() => {
    if (!id) return
    api.get<Durum>(`/domains/${id}/antivirus`).then(r => {
      setD(r.data)
      if (r.data.son_tarama?.durum === 'calisiyor') startPoll(r.data.son_tarama.id)
    }).catch(e => setHata(apiHata(e))).finally(() => setYuk(false))
  }, [id, startPoll])
  useEffect(() => { yukle(); return () => { if (pollRef.current) clearInterval(pollRef.current) } }, [yukle])

  async function tara() {
    setHata(null); setTarariyor(true)
    try {
      const { data } = await api.post(`/domains/${id}/antivirus/tara`, {})
      startPoll(data.scan_id)
    } catch (e) { setHata(apiHata(e, t('DomainAntivirusPage:scan_start_failed'))); setTarariyor(false) }
  }

  async function karantina(b: Bulgu) {
    if (!confirm(t('DomainAntivirusPage:quarantine_confirm', { file: b.dosya }))) return
    setHata(null)
    try { await api.post(`/domains/${id}/antivirus/karantina`, { bulgu_id: b.id }); yukle() }
    catch (e) { setHata(apiHata(e, t('DomainAntivirusPage:quarantine_failed'))) }
  }

  async function geriAl(b: Bulgu) {
    if (!confirm(t('DomainAntivirusPage:restore_confirm', { file: b.dosya }))) return
    setHata(null)
    try { await api.post(`/domains/${id}/antivirus/karantina-geri-al`, { bulgu_id: b.id }); yukle() }
    catch (e) { setHata(apiHata(e, t('DomainAntivirusPage:restore_failed'))) }
  }

  async function istisnaDegistir(b: Bulgu) {
    setHata(null)
    try {
      if (b.istisna) await api.delete(`/domains/${id}/antivirus/istisna/${b.id}`)
      else {
        if (!confirm(t('DomainAntivirusPage:exception_confirm', { file: b.dosya }))) return
        await api.post(`/domains/${id}/antivirus/istisna`, { bulgu_id: b.id })
      }
      yukle()
    } catch (e) { setHata(apiHata(e, t('DomainAntivirusPage:exception_failed'))) }
  }

  async function imzaGuncelle() {
    setImzaYuk(true); setHata(null)
    try { await api.post(`/domains/${id}/antivirus/imza-guncelle`, {}); yukle() }
    catch (e) { setHata(apiHata(e, t('DomainAntivirusPage:signature_update_failed'))) }
    finally { setImzaYuk(false) }
  }

  if (yuk) return <div className="px-6 py-5 text-slate-400">{t('common:loading')}</div>
  if (!d) return <div className="px-6 py-5"><div className="text-sm text-red-600">{hata || t('DomainAntivirusPage:not_found')}</div></div>

  const aktif = d.bulgular.filter(b => !b.karantina && !b.istisna)

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { etiket: t('common:home'), href: '/' },
          { etiket: t('DomainAntivirusPage:breadcrumb.domains'), href: '/domainler' },
          { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
          { etiket: t('DomainAntivirusPage:breadcrumb.antivirus') },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainAntivirusPage:title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          <span className="font-mono">public_html</span> {t('DomainAntivirusPage:desc_suffix')}
        </p>

        {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}

        {/* Durum + eylemler */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="text-sm space-y-0.5">
              <div className="flex items-center gap-2">
                <span className={`w-2 h-2 rounded-full ${d.clamav ? 'bg-emerald-500' : 'bg-amber-500'}`} />
                <span className="text-slate-700 dark:text-slate-200">{t('DomainAntivirusPage:engine_label')} <span className="font-medium">{d.clamav ? t('DomainAntivirusPage:engine_clamav') : t('DomainAntivirusPage:engine_heuristic_only')}</span></span>
              </div>
              {d.clamav && <div className="text-xs text-slate-400 ml-4">{t('DomainAntivirusPage:signature_db', { date: d.imza_tarihi || '—' })}</div>}
              {d.son_tarama && <div className="text-xs text-slate-400 ml-4">
                {t('DomainAntivirusPage:last_scan', { when: d.son_tarama.bitis || d.son_tarama.baslangic, scanned: d.son_tarama.taranan, found: d.son_tarama.enfekte })}
              </div>}
            </div>
            <div className="flex gap-2">
              {d.clamav && <button onClick={imzaGuncelle} disabled={imzaYuk || tarariyor}
                className="px-3 py-2 text-sm border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50">
                {imzaYuk ? t('DomainAntivirusPage:updating_signatures') : t('DomainAntivirusPage:update_signatures')}</button>}
              <button onClick={tara} disabled={tarariyor}
                className="px-4 py-2 text-sm font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
                {tarariyor ? t('DomainAntivirusPage:scanning') : t('DomainAntivirusPage:scan_now')}</button>
            </div>
          </div>
          {tarariyor ? (
            <div className="mt-3 flex items-center gap-2 text-sm text-brand-600 dark:text-brand-400">
              <span className="inline-block w-4 h-4 border-2 border-brand-500 border-t-transparent rounded-full animate-spin" />
              {t('DomainAntivirusPage:scan_in_progress')}
            </div>
          ) : (
            <div className="flex items-start gap-2 mt-3 px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
              <span className="text-amber-500 dark:text-amber-400 text-sm leading-none mt-0.5">⚠</span>
              <span className="text-xs text-amber-800 dark:text-amber-300">{t('DomainAntivirusPage:resource_warning')}</span>
            </div>
          )}
        </div>

        {/* Bulgular */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">
            {t('DomainAntivirusPage:findings_title')} {d.son_tarama && <span className="text-xs font-normal text-slate-400">{t('DomainAntivirusPage:findings_last_scan_note')}</span>}
          </h3>
          {!d.son_tarama ? (
            <div className="text-center py-8 text-sm text-slate-500 dark:text-slate-400">{t('DomainAntivirusPage:no_scan_yet')}</div>
          ) : aktif.length === 0 && d.bulgular.length === 0 ? (
            <div className="text-center py-8">
              <div className="text-3xl mb-2">✅</div>
              <p className="text-sm text-emerald-600 dark:text-emerald-400 font-medium">{t('DomainAntivirusPage:clean')}</p>
            </div>
          ) : (
            <div className="lg:overflow-x-auto">
              <table className={T.tablo}>
                <thead className={T.baslikGrubu}>
                  <tr className="text-left border-b border-slate-100 dark:border-slate-700">
                    <th className={T.baslik}>{t('DomainAntivirusPage:table.file')}</th><th className={T.baslik}>{t('DomainAntivirusPage:table.signature')}</th><th className={T.baslik}>{t('DomainAntivirusPage:table.score')}</th><th className={T.baslik}>{t('DomainAntivirusPage:table.status')}</th><th className={T.baslik}></th>
                  </tr>
                </thead>
                <tbody className={T.govde}>
                  {d.bulgular.map((b, i) => (
                    <tr key={i} className={`${T.satir} lg:border-b lg:border-slate-50 dark:lg:border-slate-800`}>
                      <td className={T.hucreBaslik}><span className="font-mono text-xs lg:text-xs text-sm break-all">{b.dosya}</span></td>
                      <td className={T.hucre} data-etiket={t('DomainAntivirusPage:table.signature')}><span className="text-slate-700 dark:text-slate-200">{b.imza}</span>{b.gerekceler?.length > 0 && <details className="mt-1 text-xs text-slate-500"><summary>{t('DomainAntivirusPage:reasons')}</summary>{b.gerekceler.map((g, j) => <div key={j}>• {g}</div>)}</details>}</td>
                      <td className={T.hucre} data-etiket={t('DomainAntivirusPage:table.score')}><span className={`text-xs px-2 py-1 rounded font-semibold ${b.puan >= 90 ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'}`}>{b.puan || 100}/100 · {t(`DomainAntivirusPage:risk.${b.risk || 'kritik'}`)}</span><div className="mt-1 text-xs text-slate-400">{b.motor}</div></td>
                      <td className={T.hucre} data-etiket={t('DomainAntivirusPage:table.status')}>
                        {b.karantina ? <span className="text-xs text-amber-600 dark:text-amber-400">🔒 {t('DomainAntivirusPage:quarantined')}</span>
                          : b.istisna ? <span className="text-xs text-slate-500">✓ {t('DomainAntivirusPage:excepted')}</span>
                          : <span className="text-xs text-red-600 dark:text-red-400">⚠ {t('DomainAntivirusPage:active_finding')}</span>}
                      </td>
                      <td className={T.hucreAksiyon}>
                        <div className="flex flex-col items-end gap-1">
                          {b.karantina ? <button onClick={() => geriAl(b)} className="text-xs text-amber-700 dark:text-amber-300 hover:underline whitespace-nowrap">{t('DomainAntivirusPage:restore_action')}</button>
                            : !b.istisna && <button onClick={() => karantina(b)} className="text-xs text-red-600 dark:text-red-400 hover:underline whitespace-nowrap">{t('DomainAntivirusPage:quarantine_action')}</button>}
                          {!b.karantina && <button onClick={() => istisnaDegistir(b)} className="text-xs text-slate-500 hover:underline whitespace-nowrap">{b.istisna ? t('DomainAntivirusPage:exception_remove') : t('DomainAntivirusPage:exception_action')}</button>}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">{t('DomainAntivirusPage:back_to_subscription')}</Link></div>
      </div>
    </div>
  )
}
