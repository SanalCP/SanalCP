import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'

// Alt alan adı göçü — alt alan adı sistemi kaldırıldıktan sonra kalan TEK
// yönetim yüzeyi (bkz. internal/altalangoc, migrations/0084).
//
// Bu sayfa, göçünü çalıştırmamış kurulumlarda hâlâ `subdomanlar` satırı
// olabileceği için var. Kayıt yoksa "yapacak bir şey yok" der ve kimseyi
// rahatsız etmez; kayıt varsa göçün tek çalıştırma yolu burasıdır.
type Kayit = {
  id: number
  tam_ad: string
  alt_ad: string
  php_surum: string
  ana_alan_adi: string
  eski_sk: string
  hedef_sk: string
  kaynak_dizin: string
  boyut_kb: number
  dosya_sayisi: number
  sertifika_var: boolean
  symlink_var: boolean
  sorun?: string
}
type Envanter = { kayitlar: Kayit[]; toplam: number; goc_edilebilir: number }
type Sonuc = {
  id: number
  tam_ad: string
  basarili: boolean
  domain_id?: number
  hedef_sk?: string
  hata?: string
  adimlar?: string[]
}
type GocYanit = { dry_run: boolean; sonuclar: Sonuc[]; basarili: number; basarisiz: number; hata?: string }

function fmtKB(kb: number) {
  if (kb < 1024) return kb + ' KB'
  if (kb < 1024 * 1024) return (kb / 1024).toFixed(1) + ' MB'
  return (kb / 1024 / 1024).toFixed(2) + ' GB'
}

export default function AltAlanGocPage() {
  const { t } = useTranslation(['AltAlanGocPage', 'common'])
  const [env, setEnv] = useState<Envanter | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [calisiyor, setCalisiyor] = useState(false)
  const [yanit, setYanit] = useState<GocYanit | null>(null)
  const [secili, setSecili] = useState<Set<number>>(new Set())

  function yukle() {
    setYuk(true); setHata(null)
    api.get<Envanter>('/altalan-goc')
      .then(r => {
        setEnv(r.data)
        // Göç edilebilir olanlar varsayılan olarak seçili gelir; sorunlu
        // kayıtlar seçilemez, zaten sunucu da onları atlar.
        setSecili(new Set((r.data.kayitlar || []).filter(k => !k.sorun).map(k => k.id)))
      })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [])

  async function calistir(dryRun: boolean) {
    const ids = Array.from(secili)
    if (ids.length === 0) return
    if (!dryRun) {
      // Gerçek göç dosya taşır, nginx conf yazar ve DNS kaydı siler. Kuru
      // çalıştırmadan sonra bile açık bir onay isteniyor.
      if (!confirm(t('AltAlanGocPage:confirm_run', { count: ids.length }))) return
    }
    setCalisiyor(true); setHata(null); setYanit(null)
    try {
      const { data } = await api.post<GocYanit>('/altalan-goc', { ids, dry_run: dryRun })
      setYanit(data)
      if (!dryRun) yukle()
    } catch (e) {
      // Sunucu nginx doğrulaması düşerse 500 + gövde döner; gövdedeki sonuç
      // listesi hangi kayıtta kalındığını gösteriyor, atmak yerine gösteriyoruz.
      const govde = (e as { response?: { data?: GocYanit } })?.response?.data
      if (govde?.sonuclar) setYanit(govde)
      setHata(apiHata(e, t('AltAlanGocPage:run_failed')))
    } finally { setCalisiyor(false) }
  }

  function toggle(id: number) {
    setSecili(s => {
      const y = new Set(s)
      y.has(id) ? y.delete(id) : y.add(id)
      return y
    })
  }

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('AltAlanGocPage:title') }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">{t('AltAlanGocPage:title')}</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5 max-w-3xl">{t('AltAlanGocPage:subtitle')}</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {yuk ? (
        <div className="text-sm text-slate-400 py-6">{t('common:loading')}</div>
      ) : !env || env.toplam === 0 ? (
        <EmptyState
          baslik={t('AltAlanGocPage:empty_title')}
          aciklama={t('AltAlanGocPage:empty_desc')}
        />
      ) : (
        <>
          <div className="mb-4 px-4 py-3 bg-amber-50 dark:bg-amber-900/20 border border-amber-300 dark:border-amber-800 rounded-2xl text-sm text-amber-900 dark:text-amber-200">
            <div className="font-semibold mb-1">⚠ {t('AltAlanGocPage:warn_title')}</div>
            <p className="text-[13px] leading-relaxed">{t('AltAlanGocPage:warn_body')}</p>
          </div>

          <div className="flex flex-wrap items-center gap-2 mb-3">
            <button onClick={() => calistir(true)} disabled={calisiyor || secili.size === 0}
              className="text-sm px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 disabled:opacity-50">
              {calisiyor ? t('AltAlanGocPage:running') : t('AltAlanGocPage:dry_run_button', { count: secili.size })}
            </button>
            <button onClick={() => calistir(false)} disabled={calisiyor || secili.size === 0}
              className="text-sm px-3 py-1.5 rounded-lg bg-brand-600 hover:bg-brand-700 text-white disabled:opacity-50">
              {t('AltAlanGocPage:run_button', { count: secili.size })}
            </button>
            <span className="text-xs text-slate-500 dark:text-slate-400">
              {t('AltAlanGocPage:counts', { total: env.toplam, ok: env.goc_edilebilir })}
            </span>
          </div>

          <div className="ta-card overflow-x-auto mb-5">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[11px] uppercase tracking-wide text-slate-400">
                  <th className="px-3 py-2 w-8"></th>
                  <th className="px-3 py-2">{t('AltAlanGocPage:col_name')}</th>
                  <th className="px-3 py-2">{t('AltAlanGocPage:col_target')}</th>
                  <th className="px-3 py-2">{t('AltAlanGocPage:col_content')}</th>
                  <th className="px-3 py-2">{t('AltAlanGocPage:col_state')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-700/60">
                {env.kayitlar.map(k => (
                  <tr key={k.id} className={k.sorun ? 'opacity-60' : ''}>
                    <td className="px-3 py-2">
                      <input type="checkbox" checked={secili.has(k.id)} disabled={!!k.sorun}
                        onChange={() => toggle(k.id)} />
                    </td>
                    <td className="px-3 py-2">
                      <div className="font-mono text-sm text-slate-800 dark:text-slate-200">{k.tam_ad}</div>
                      <div className="text-[11px] text-slate-400">
                        {t('AltAlanGocPage:parent', { domain: k.ana_alan_adi })} · PHP {k.php_surum}
                      </div>
                    </td>
                    <td className="px-3 py-2 font-mono text-xs text-slate-600 dark:text-slate-400">
                      {k.eski_sk} → {k.hedef_sk}
                    </td>
                    <td className="px-3 py-2 text-xs text-slate-600 dark:text-slate-400 whitespace-nowrap">
                      {t('AltAlanGocPage:files', { count: k.dosya_sayisi })} · {fmtKB(k.boyut_kb)}
                    </td>
                    <td className="px-3 py-2 text-xs">
                      {k.sorun
                        ? <span className="text-red-600 dark:text-red-400">{k.sorun}</span>
                        : (
                          <span className="inline-flex flex-col gap-0.5">
                            <span className={k.sertifika_var ? 'text-emerald-600 dark:text-emerald-400' : 'text-slate-400'}>
                              {k.sertifika_var ? t('AltAlanGocPage:cert_yes') : t('AltAlanGocPage:cert_no')}
                            </span>
                            {k.symlink_var && (
                              <span className="text-amber-600 dark:text-amber-400">{t('AltAlanGocPage:symlink_warn')}</span>
                            )}
                          </span>
                        )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {yanit && (
        <div className="ta-card p-4">
          <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">
            {yanit.dry_run ? t('AltAlanGocPage:result_dry') : t('AltAlanGocPage:result_real')}
            {' · '}
            {t('AltAlanGocPage:result_counts', { ok: yanit.basarili, fail: yanit.basarisiz })}
          </h3>
          {yanit.hata && (
            <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">
              {yanit.hata}
            </div>
          )}
          <ul className="space-y-3">
            {yanit.sonuclar.map(s => (
              <li key={s.id}>
                <div className="flex items-center gap-2">
                  <span>{s.basarili ? '✓' : '✕'}</span>
                  <span className="font-mono text-sm">{s.tam_ad}</span>
                  {s.hedef_sk && <span className="text-[11px] text-slate-400 font-mono">→ {s.hedef_sk}</span>}
                </div>
                {s.hata && <div className="text-xs text-red-600 dark:text-red-400 ml-6">{s.hata}</div>}
                {!!s.adimlar?.length && (
                  <ul className="ml-6 mt-1 space-y-0.5 text-[11px] text-slate-500 dark:text-slate-400">
                    {s.adimlar.map((a, i) => <li key={i}>· {a}</li>)}
                  </ul>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
