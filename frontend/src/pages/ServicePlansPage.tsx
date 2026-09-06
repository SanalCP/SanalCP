// sanal-dark-swept
// sanal-dark-swept-v2
import { modalUyari } from '@/lib/dialog'
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import { useAuth } from '@/store/auth'
import Breadcrumb from '@/components/Breadcrumb'
import ListToolbar from '@/components/ListToolbar'
import EmptyState from '@/components/EmptyState'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type Plan = {
  id: number
  ad: string
  aciklama: string
  disk_kota_mb: number
  trafik_kota_mb: number
  max_domain: number
  max_db: number
  max_email: number
  max_ftp: number
  php_surum: string
  fastcgi_cache: boolean
  client_max_body_mb: number
  nginx_ek_direktifler: string
  varsayilan: boolean
  olusturulma: string
}
type Surum = { surum: string; aciklama?: string }

export default function ServicePlansPage() {
  const { t, i18n } = useTranslation(['ServicePlansPage', 'common'])
  const locale = i18n.language === 'en' ? 'en-US' : 'tr-TR'
  const adminMi = useAuth((s) => s.kullanici?.rol) === 'admin'
  const [items, setItems] = useState<Plan[]>([])
  const [surumler, setSurumler] = useState<Surum[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [modal, setModal] = useState<Plan | null>(null)
  const [silinecek, setSilinecek] = useState<Plan | null>(null)

  function yukle() {
    setYuk(true); setHata(null)
    api.get<Plan[]>('/plans')
      .then(r => setItems(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [])
  useEffect(() => {
    api.get<Surum[]>('/php/versions').then(r => setSurumler(r.data || [])).catch(() => {})
  }, [])

  async function sil() {
    if (!silinecek) return
    try {
      await api.delete(`/plans/${silinecek.id}`)
      setSilinecek(null); yukle()
    } catch (e) {
      await modalUyari(apiHata(e, t('ServicePlansPage:errors.delete_failed')))
    }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('ServicePlansPage:breadcrumb.title') }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">{t('ServicePlansPage:title')}</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
        {t('ServicePlansPage:subtitle')}
      </p>

      <ListToolbar
        birincil={adminMi ? { etiket: t('ServicePlansPage:buttons.add_plan'), onClick: () => setModal({} as Plan) } : undefined}
        butonlar={[]}
      />

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('ServicePlansPage:loading')}</div>
      ) : items.length === 0 ? (
        <EmptyState
          baslik={t('ServicePlansPage:empty.title')}
          aciklama={t('ServicePlansPage:empty.description')}
          buton={{ etiket: t('ServicePlansPage:buttons.add_plan'), onClick: () => setModal({} as Plan) }}
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {items.map(p => (
            <div key={p.id} className={`bg-white dark:bg-slate-800 border rounded-2xl p-5 shadow-sm ${p.varsayilan ? 'border-brand-400 ring-2 ring-brand-100 dark:ring-brand-900/40' : 'border-slate-200 dark:border-slate-700'}`}>
              <div className="flex items-start justify-between mb-2">
                <div className="min-w-0">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-100 flex items-center gap-2">
                    {p.ad}
                    {p.varsayilan && <span className="text-[10px] uppercase tracking-wider bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 px-1.5 py-0.5 rounded font-semibold">{t('ServicePlansPage:card.default_badge')}</span>}
                  </h3>
                  {p.aciklama && <p className="text-sm text-slate-500 dark:text-slate-500 mt-0.5">{p.aciklama}</p>}
                </div>
                {p.php_surum && <span className="shrink-0 text-[11px] font-mono font-semibold bg-slate-100 dark:bg-slate-700/60 text-slate-600 dark:text-slate-300 px-2 py-0.5 rounded">PHP {p.php_surum}</span>}
              </div>

              <dl className="grid grid-cols-2 gap-y-1.5 text-sm mt-4">
                <Sat e={t('ServicePlansPage:card.labels.disk')} d={fmt(p.disk_kota_mb, 'mb', locale, t)} />
                <Sat e={t('ServicePlansPage:card.labels.traffic')} d={fmt(p.trafik_kota_mb, 'mb_per_month', locale, t)} />
                <Sat e={t('ServicePlansPage:card.labels.domain')} d={fmt(p.max_domain, 'count', locale, t)} />
                <Sat e={t('ServicePlansPage:card.labels.database')} d={fmt(p.max_db, 'count', locale, t)} />
                <Sat e={t('ServicePlansPage:card.labels.ftp')} d={fmt(p.max_ftp, 'ftp_account', locale, t)} />
              </dl>

              {/* Plan tanımı yöneticinin ürünüdür; bayi planları yalnız görür
                  (sunucu tarafında da /plans yazma uçları AdminOnly). */}
              {adminMi && (
                <div className="mt-4 flex gap-2">
                  <Link to={`/araclar/paketler/${p.id}`} className="flex-1 text-center text-sm px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md">
                    {t('ServicePlansPage:card.detail_button')}
                  </Link>
                  <button onClick={() => setSilinecek(p)} className="text-sm px-3 py-1.5 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 rounded-md">{t('ServicePlansPage:card.actions.delete')}</button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {modal && (
        <PlanModal
          plan={modal}
          surumler={surumler}
          onKapat={() => setModal(null)}
          onKayit={() => { setModal(null); yukle() }}
        />
      )}

      <ConfirmDialog
        acik={!!silinecek}
        baslik={t('ServicePlansPage:confirm.title')}
        mesaj={t('ServicePlansPage:confirm.message', { ad: silinecek?.ad || '' })}
        tehlikeli
        onayMetni={t('ServicePlansPage:confirm.yes')}
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}

function Sat({ e, d }: { e: string; d: string }) {
  return (
    <>
      <dt className="text-slate-500 dark:text-slate-500">{e}</dt>
      <dd className="text-slate-800 dark:text-slate-200 text-right font-mono">{d}</dd>
    </>
  )
}

type UnitKey = 'count' | 'mb' | 'mb_per_month' | 'ftp_account'

function fmt(n: number, birim: UnitKey, locale: string, t: (key: string) => string) {
  if (n <= 0) return t('ServicePlansPage:units.unlimited')
  if ((birim === 'mb' || birim === 'mb_per_month') && n >= 1024) {
    const suffix = birim === 'mb_per_month' ? '/' + t('ServicePlansPage:units.mb_per_month').split('/')[1] : ''
    return `${(n / 1024).toFixed(1)} GB${suffix}`
  }
  return `${n.toLocaleString(locale)} ${t(`ServicePlansPage:units.${birim}`)}`
}

function PlanModal({ plan, surumler, onKapat, onKayit }: { plan: Plan; surumler: Surum[]; onKapat: () => void; onKayit: () => void }) {
  const { t } = useTranslation(['ServicePlansPage', 'common'])
  const yeni = !plan.id
  const [form, setForm] = useState<Plan>({
    id: plan.id || 0,
    ad: plan.ad || '',
    aciklama: plan.aciklama || '',
    disk_kota_mb: plan.disk_kota_mb || 1024,
    trafik_kota_mb: plan.trafik_kota_mb || 10240,
    max_domain: plan.max_domain || 1,
    max_db: plan.max_db || 1,
    max_email: plan.max_email || 0,
    max_ftp: plan.max_ftp || 2,
    php_surum: plan.php_surum || '8.3',
    fastcgi_cache: plan.fastcgi_cache || false,
    client_max_body_mb: plan.client_max_body_mb || 64,
    nginx_ek_direktifler: plan.nginx_ek_direktifler || '',
    varsayilan: plan.varsayilan || false,
    olusturulma: '',
  })
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  const phpOpts = Array.from(new Set([
    ...surumler.map(s => s.surum),
    form.php_surum,
    ...(surumler.length === 0 ? ['7.4', '8.1', '8.2', '8.3', '8.4'] : []),
  ].filter(Boolean)))

  async function gonder(e: React.FormEvent) {
    e.preventDefault()
    setIsleniyor(true); setHata(null)
    try {
      if (yeni) await api.post('/plans', form)
      else await api.put(`/plans/${form.id}`, form)
      onKayit()
    } catch (e) {
      setHata(apiHata(e, t('ServicePlansPage:errors.save_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={true} baslik={yeni ? t('ServicePlansPage:modal.title_new') : t('ServicePlansPage:modal.title_edit')} onKapat={onKapat} genislik="lg">
      <form onSubmit={gonder} className="space-y-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Alan etiket={t('ServicePlansPage:modal.fields.plan_name')} value={form.ad} setVal={v => setForm({ ...form, ad: v })} required />
          <Alan etiket={t('ServicePlansPage:modal.fields.description')} value={form.aciklama} setVal={v => setForm({ ...form, aciklama: v })} />
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Sayi etiket={t('ServicePlansPage:modal.fields.disk_mb')} value={form.disk_kota_mb} setVal={v => setForm({ ...form, disk_kota_mb: v })} />
          <Sayi etiket={t('ServicePlansPage:modal.fields.traffic_mb')} value={form.trafik_kota_mb} setVal={v => setForm({ ...form, trafik_kota_mb: v })} />
          <div>
            <label className="ta-label-sm">{t('ServicePlansPage:modal.fields.php_version')}</label>
            <select value={form.php_surum} onChange={e => setForm({ ...form, php_surum: e.target.value })}
              className="ta-input ta-input-sm w-full">
              {phpOpts.map(v => <option key={v} value={v}>PHP {v}</option>)}
            </select>
          </div>
          <Sayi etiket={t('ServicePlansPage:modal.fields.max_domain')} value={form.max_domain} setVal={v => setForm({ ...form, max_domain: v })} />
          <Sayi etiket={t('ServicePlansPage:modal.fields.max_db')} value={form.max_db} setVal={v => setForm({ ...form, max_db: v })} />
          <Sayi etiket={t('ServicePlansPage:modal.fields.max_ftp')} value={form.max_ftp} setVal={v => setForm({ ...form, max_ftp: v })} />
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.varsayilan} onChange={e => setForm({ ...form, varsayilan: e.target.checked })} className="rounded" />
          {t('ServicePlansPage:modal.checkboxes.default_plan')}
        </label>
        <p className="ta-hint">{t('ServicePlansPage:modal.info_note')}</p>

        {hata && <div className="ta-form-error" role="alert">{hata}</div>}

        <div className="ta-form-actions">
          <button type="button" onClick={onKapat} className="ta-secondary-button">{t('ServicePlansPage:modal.buttons.cancel')}</button>
          <button type="submit" disabled={isleniyor || !form.ad.trim()} className="ta-primary-button">{isleniyor ? t('ServicePlansPage:modal.buttons.saving') : (yeni ? t('ServicePlansPage:modal.buttons.add') : t('ServicePlansPage:modal.buttons.update'))}</button>
        </div>
      </form>
    </Modal>
  )
}

function Alan({ etiket, value, setVal, required }: { etiket: string; value: string; setVal: (v: string) => void; required?: boolean }) {
  return (
    <div>
      <label className="ta-label-sm">{etiket}</label>
      <input type="text" value={value} onChange={e => setVal(e.target.value)} required={required}
        className="ta-input ta-input-sm w-full" />
    </div>
  )
}
function Sayi({ etiket, value, setVal }: { etiket: string; value: number; setVal: (v: number) => void }) {
  return (
    <div>
      <label className="ta-label-sm">{etiket}</label>
      <input type="number" min={0} value={value} onChange={e => setVal(parseInt(e.target.value) || 0)}
        className="ta-input ta-input-sm w-full font-mono" />
    </div>
  )
}
