// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Mod = 'devral' | 'kapali' | 'engelle' | 'denetle'
type Ayar = { mod: Mod; paranoya: number }
type ModBilgi = { aktif: boolean; mod: string; paranoya: number; ad?: string }
type Efektif = { aktif: boolean; engine: string; paranoya: number }
type Yanit = {
  alan_adi: string
  ayar: Ayar
  plan: ModBilgi
  efektif: Efektif
  modul_yuklu: boolean
}

function modlar(t: TFunction): { key: Mod; ad: string; ikon: string; aciklama: string; renk: string }[] {
  return [
    { key: 'devral', ad: t('DomainWafPage:modes.devral.title'), ikon: '↩︎',
      aciklama: t('DomainWafPage:modes.devral.desc'), renk: 'slate' },
    { key: 'engelle', ad: t('DomainWafPage:modes.engelle.title'), ikon: '🛡️',
      aciklama: t('DomainWafPage:modes.engelle.desc'), renk: 'emerald' },
    { key: 'denetle', ad: t('DomainWafPage:modes.denetle.title'), ikon: '👁️',
      aciklama: t('DomainWafPage:modes.denetle.desc'), renk: 'indigo' },
    { key: 'kapali', ad: t('DomainWafPage:modes.kapali.title'), ikon: '⛔',
      aciklama: t('DomainWafPage:modes.kapali.desc'), renk: 'rose' },
  ]
}

function paranoyaAciklama(t: TFunction): Record<number, string> {
  return {
    0: t('DomainWafPage:paranoia.hint0'),
    1: t('DomainWafPage:paranoia.hint1'),
    2: t('DomainWafPage:paranoia.hint2'),
    3: t('DomainWafPage:paranoia.hint3'),
    4: t('DomainWafPage:paranoia.hint4'),
  }
}

export default function DomainWafPage() {
  const { t } = useTranslation(['DomainWafPage', 'common'])
  const { id } = useParams()
  const [y, setY] = useState<Yanit | null>(null)
  const [ayar, setAyar] = useState<Ayar | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [isleniyor, setIsleniyor] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true); setHata(null)
    api.get<Yanit>(`/domains/${id}/waf`)
      .then(r => { setY(r.data); setAyar(r.data.ayar) })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [id])

  async function kaydet() {
    if (!ayar) return
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      const r = await api.put<{ efektif: Efektif; modul_yuklu: boolean }>(`/domains/${id}/waf`, { ayar })
      const ef = r.data.efektif
      setBasari(ef.aktif
        ? t('DomainWafPage:applied_msg', { mode: ef.engine === 'On' ? t('DomainWafPage:modes.engelle.title') : t('DomainWafPage:modes.denetle.title'), level: ef.paranoya })
        : t('DomainWafPage:saved_inactive_msg'))
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainWafPage:save_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' }, { etiket: t('DomainWafPage:breadcrumb.domains'), href: '/domainler' },
        { etiket: y?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainWafPage:breadcrumb.waf') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainWafPage:title')}</h1>
      {y && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        <Link to={`/abonelikler/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 font-medium">{y.alan_adi}</Link>
        {' · '}{t('DomainWafPage:domain_info')}
      </p>}

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {y && !y.modul_yuklu && (
        <div className="mb-5 px-3 py-2.5 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-md text-xs text-amber-800 dark:text-amber-200">
          <strong>{t('DomainWafPage:module_not_installed.title')}</strong>{' '}
          {t('DomainWafPage:module_not_installed.desc', { cmd: 'sanalcp-waf-setup' }).replace('sanalcp-waf-setup', '')}
          <code className="font-mono">sanalcp-waf-setup</code>
        </div>
      )}

      {yuk || !ayar || !y ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div>
      ) : (
        <>
          {/* Efektif durum + plan bilgisi */}
          <div className="mb-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainWafPage:effective_status.label')}</span>
              {y.efektif.aktif ? (
                <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold ${
                  y.efektif.engine === 'On'
                    ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300'
                    : 'bg-indigo-100 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-300'
                }`}>
                  ● {y.efektif.engine === 'On' ? t('DomainWafPage:effective_status.active_block') : t('DomainWafPage:effective_status.active_detect')} · Paranoya {y.efektif.paranoya}
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400">○ {t('DomainWafPage:effective_status.inactive')}</span>
              )}
              <span className="text-xs text-slate-400 dark:text-slate-500 ml-auto">
                {t('DomainWafPage:effective_status.plan_default', { name: y.plan.ad || '—' })}{' '}
                {y.plan.aktif ? `${y.plan.mod === 'denetle' ? t('DomainWafPage:modes.denetle.title') : t('DomainWafPage:modes.engelle.title')} · PL${y.plan.paranoya}` : t('DomainWafPage:effective_status.plan_off')}
              </span>
            </div>
          </div>

          {/* Mod seçici */}
          <Kart baslik={t('DomainWafPage:modes.title')}>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {modlar(t).map(m => {
                const aktif = ayar.mod === m.key
                const renk: Record<string, string> = {
                  slate:   aktif ? 'border-slate-500 bg-slate-100 dark:bg-slate-700/40 ring-2 ring-slate-400/20' : 'border-slate-200 dark:border-slate-700 hover:border-slate-400',
                  emerald: aktif ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 ring-2 ring-emerald-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-emerald-300',
                  indigo:  aktif ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20 ring-2 ring-indigo-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-indigo-300',
                  rose:    aktif ? 'border-rose-500 bg-rose-50 dark:bg-rose-900/20 ring-2 ring-rose-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-rose-300',
                }
                return (
                  <button key={m.key} type="button" onClick={() => setAyar({ ...ayar, mod: m.key })}
                    className={`text-left p-4 border rounded-xl transition ${renk[m.renk]}`}>
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{m.ikon} {m.ad}</span>
                      {aktif && <span className="text-[10px] uppercase tracking-wider font-semibold text-slate-500 dark:text-slate-400">● Seçili</span>}
                    </div>
                    <div className="text-[11px] text-slate-600 dark:text-slate-400 leading-snug">{m.aciklama}</div>
                  </button>
                )
              })}
            </div>
          </Kart>

          {/* Paranoya */}
          <Kart baslik="Paranoya Seviyesi (CRS)">
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-3">
              Daha yüksek seviye = daha çok kural + daha güçlü koruma, ancak yanlış-pozitif olasılığı artar.
              Yalnızca WAF <strong>Engelle</strong> veya <strong>Denetle</strong> modundayken etkilidir.
            </p>
            <div className="flex items-center gap-3">
              <select
                value={ayar.paranoya}
                onChange={e => setAyar({ ...ayar, paranoya: parseInt(e.target.value) })}
                disabled={ayar.mod === 'devral' || ayar.mod === 'kapali'}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 dark:bg-slate-800 rounded text-sm font-mono disabled:opacity-50">
                <option value={0}>Plandan devral</option>
                <option value={1}>Seviye 1 (Düşük)</option>
                <option value={2}>Seviye 2 (Orta)</option>
                <option value={3}>Seviye 3 (Yüksek)</option>
                <option value={4}>Seviye 4 (Sıkı)</option>
              </select>
              <span className="text-xs text-slate-500 dark:text-slate-400">{paranoyaAciklama(t)[ayar.paranoya]}</span>
            </div>
          </Kart>

          <div className="flex gap-3 mt-6">
            <button onClick={kaydet} disabled={isleniyor}
              className="px-6 py-2.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">
              {isleniyor ? 'Uygulanıyor…' : '💾 Kaydet ve Uygula'}
            </button>
            <button onClick={yukle} disabled={isleniyor}
              className="px-4 py-2.5 border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 text-sm rounded-md">
              Yeniden Yükle
            </button>
          </div>
        </>
      )}
    </div>
  )
}

function Kart({ baslik, children }: { baslik: string; children: any }) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4">
      <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-3 pb-2 border-b border-slate-100 dark:border-slate-800">{baslik}</h3>
      {children}
    </div>
  )
}
