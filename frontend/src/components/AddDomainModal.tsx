// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Modal from './Modal'

const PHP_FALLBACK = ['7.4', '8.1', '8.2', '8.3', '8.4']

type Plan = { id: number; ad: string; php_surum: string; varsayilan: boolean }
type Surum = { surum: string; aciklama?: string }

export default function AddDomainModal({
  acik, onKapat, onEklendi,
}: {
  acik: boolean
  onKapat: () => void
  onEklendi: () => void
}) {
  const { t } = useTranslation(['AddDomainModal', 'common'])
  const [alanAdi, setAlanAdi] = useState('')
  const [phpSurum, setPhpSurum] = useState('8.3')
  const [planId, setPlanId] = useState<number | ''>('')
  const [planlar, setPlanlar] = useState<Plan[]>([])
  const [surumler, setSurumler] = useState<Surum[]>([])
  const [yukleniyor, setYukleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)

  // Modal açıldığında planları + kurulu PHP sürümlerini çek
  useEffect(() => {
    if (!acik) return
    api.get<Plan[]>('/plans').then(r => {
      const list = r.data || []
      setPlanlar(list)
      // Varsayılan plan varsa ön-seç + PHP'sini uygula
      const vars = list.find(p => p.varsayilan)
      if (vars) {
        setPlanId(vars.id)
        if (vars.php_surum) setPhpSurum(vars.php_surum)
      }
    }).catch(() => {})
    api.get<Surum[]>('/php/versions').then(r => setSurumler(r.data || [])).catch(() => {})
  }, [acik])

  function planDegis(v: string) {
    const idNum = v === '' ? '' : Number(v)
    setPlanId(idNum)
    if (idNum !== '') {
      const p = planlar.find(x => x.id === idNum)
      if (p?.php_surum) setPhpSurum(p.php_surum)
    }
  }

  const phpOpts = Array.from(new Set([
    ...(surumler.length ? surumler.map(s => s.surum) : PHP_FALLBACK),
    phpSurum,
  ].filter(Boolean)))

  const seciliPlan = planId === '' ? null : planlar.find(p => p.id === planId)
  const phpPlandan = !!seciliPlan && seciliPlan.php_surum === phpSurum

  async function gonder(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setBasari(null); setYukleniyor(true)
    try {
      const govde: Record<string, unknown> = {
        alan_adi: alanAdi.trim().toLowerCase(),
        php_surum: phpSurum,
      }
      if (planId !== '') govde.plan_id = planId
      const { data } = await api.post('/domains', govde)
      setBasari(t('AddDomainModal:created_success', { alan_adi: data.alan_adi, sistem_kullanici: data.sistem_kullanici }))
      setTimeout(() => {
        setAlanAdi('')
        setBasari(null)
        onEklendi()
        onKapat()
      }, 1500)
    } catch (e) {
      setHata(apiHata(e, t('AddDomainModal:create_failed')))
    } finally {
      setYukleniyor(false)
    }
  }

  return (
    <Modal acik={acik} baslik={t('AddDomainModal:title')} onKapat={onKapat} genislik="md">
      <form onSubmit={gonder} className="space-y-4">
        <div>
          <label className="ta-label">{t('AddDomainModal:domain_label')}</label>
          <input
            type="text"
            value={alanAdi}
            onChange={(e) => setAlanAdi(e.target.value)}
            placeholder={t('AddDomainModal:domain_placeholder')}
            autoFocus
            required
            className="ta-input w-full"
          />
          <p className="ta-hint">{t('AddDomainModal:domain_hint')} <code className="font-mono">site.com</code>, <code className="font-mono">musteri-1.org</code></p>
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label className="ta-label">{t('AddDomainModal:plan_label')}</label>
            <select
              value={planId}
              onChange={(e) => planDegis(e.target.value)}
              className="ta-input w-full"
            >
              <option value="">{t('AddDomainModal:plan_none')}</option>
              {planlar.map(p => (
                <option key={p.id} value={p.id}>{p.ad}{p.varsayilan ? t('AddDomainModal:plan_default_suffix') : ''}</option>
              ))}
            </select>
            <p className="ta-hint">{t('AddDomainModal:plan_hint')}</p>
          </div>
          <div>
            <label className="ta-label">{t('AddDomainModal:php_label')}</label>
            <select
              value={phpSurum}
              onChange={(e) => setPhpSurum(e.target.value)}
              className="ta-input w-full"
            >
              {phpOpts.map(v => <option key={v} value={v}>PHP {v}</option>)}
            </select>
            <p className="ta-hint">
              {phpPlandan ? <span className="text-brand-600 dark:text-brand-400">{t('AddDomainModal:php_from_plan', { ad: seciliPlan?.ad })}</span> : t('AddDomainModal:php_independent')}
            </p>
          </div>
        </div>

        <div className="bg-sky-50 dark:bg-sky-900/20 border border-sky-200 rounded-md p-3 text-xs text-sky-800">
          <strong>{t('AddDomainModal:auto_box_title')}</strong> {t('AddDomainModal:auto_box_user')} (<code className="font-mono">c_&lt;slug&gt;</code>) {t('AddDomainModal:auto_box_middle')} (<code className="font-mono">/home/c_&lt;slug&gt;/public_html</code>) {t('AddDomainModal:auto_box_end')}
        </div>

        {hata && <div className="ta-form-error" role="alert">{hata}</div>}
        {basari && <div className="ta-form-success" role="status">{basari}</div>}

        <div className="ta-form-actions">
          <button
            type="button"
            onClick={onKapat}
            disabled={yukleniyor}
            className="ta-secondary-button"
          >
            {t('common:cancel')}
          </button>
          <button
            type="submit"
            disabled={yukleniyor || !alanAdi.trim()}
            className="ta-primary-button"
          >
            {yukleniyor ? t('AddDomainModal:provisioning') : t('AddDomainModal:add_domain')}
          </button>
        </div>
      </form>
    </Modal>
  )
}
