// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import DomainList, { type Domain } from '@/components/DomainList'
import DomainPano from '@/components/DomainPano'
import ResourceCard from '@/components/ResourceCard'
import { useAuth } from '@/store/auth'

export default function DashboardPage() {
  const { t } = useTranslation(['DashboardPage', 'common'])
  const kullanici = useAuth((s) => s.kullanici)
  const [params, setParams] = useSearchParams()
  const [domainler, setDomainler] = useState<Domain[]>([])
  const [yukleniyor, setYukleniyor] = useState(true)
  const [hata, setHata] = useState<string | null>(null)

  useEffect(() => {
    setYukleniyor(true)
    api.get<Domain[]>('/domains')
      .then((r) => {
        setDomainler(r.data)
        if (!params.get('domain') && r.data.length > 0) {
          setParams({ domain: String(r.data[0].id) }, { replace: true })
        }
      })
      .catch((e) => setHata(apiHata(e)))
      .finally(() => setYukleniyor(false))
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const seciliId = Number(params.get('domain')) || domainler[0]?.id
  const secili = domainler.find((d) => d.id === seciliId) || domainler[0]

  function secimYap(id: number) {
    setParams({ domain: String(id) })
  }

  return (
    <div className="w-full px-4 sm:px-6 lg:px-8 py-7">
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="ta-eyebrow mb-2">SanalCP Overview</div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-900 dark:text-white">{t('DashboardPage:title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1.5">
            {t('DashboardPage:welcome')} <span className="text-slate-700 dark:text-slate-300 font-medium">{kullanici?.ad_soyad || kullanici?.adi}</span>
          </p>
        </div>
        {secili && (
          <div className="ta-card px-4 py-3 text-left sm:text-right text-xs text-slate-500 dark:text-slate-400">
            <span className="block">{t('DashboardPage:selected_domain')}</span>
            <span className="text-brand-600 dark:text-brand-300 font-mono font-semibold text-sm">{secili.alan_adi}</span>
          </div>
        )}
      </div>

      {hata && (
        <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">
          {hata}
        </div>
      )}

      <div className="grid grid-cols-12 gap-6">
        <aside className="col-span-12 lg:col-span-3">
          <DomainList items={domainler} seciliId={secili?.id} onSec={secimYap} yukleniyor={yukleniyor} />
        </aside>

        <section className="col-span-12 lg:col-span-6">
          {secili ? (
            <DomainPano domain={secili} />
          ) : (
            <div className="bg-white dark:bg-slate-800 border-2 border-dashed border-slate-200 dark:border-slate-700 rounded-2xl p-12 text-center text-slate-500 dark:text-slate-500">
              {yukleniyor ? t('common:loading') : t('DashboardPage:empty')}
            </div>
          )}
        </section>

        <aside className="col-span-12 lg:col-span-3">
          <ResourceCard />
        </aside>
      </div>
    </div>
  )
}
