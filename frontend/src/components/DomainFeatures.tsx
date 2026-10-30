import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { useAuth } from '@/store/auth'
import { domainPathFeature, isPlanFeatures, type PlanFeature, type PlanFeatures } from '@/lib/planFeatures'
import { DomainFeaturesContext, useDomainFeatures } from '@/lib/domainFeaturesContext'

type FeatureResponse = { features: PlanFeatures; enforced: boolean }
type FeatureState = { key: string; data?: FeatureResponse; failed?: boolean }

export function DomainFeaturesProvider({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const user = useAuth(s => s.kullanici)
  const routeDomain = location.pathname.match(/^\/abonelikler\/(\d+)(?:\/|$)/)?.[1] || ''
  const domainId = routeDomain || (user?.rol === 'user' ? localStorage.getItem('sanalcp.musteri.domain_id') || '' : '')
  const [attempt, setAttempt] = useState(0)
  // A navigation/domain/account change invalidates grants during render, before effects run.
  const key = `${location.key}:${location.pathname}:${domainId}:${user?.id}:${user?.rol}:${attempt}`
  const [state, setState] = useState<FeatureState>({ key: '' })
  useEffect(() => {
    if (!domainId) return
    const controller = new AbortController()
    api.get<FeatureResponse>(`/domains/${domainId}/features`, { signal: controller.signal })
      .then(({ data }) => {
        if (controller.signal.aborted) return
        if (!isPlanFeatures(data?.features) || typeof data?.enforced !== 'boolean') {
          setState({ key, failed: true })
          return
        }
        setState({ key, data })
      })
      .catch(() => { if (!controller.signal.aborted) setState({ key, failed: true }) })
    return () => controller.abort()
  }, [domainId, key])

  const data = state.key === key ? state.data : undefined
  const allows = (feature: PlanFeature) => !!data && (!data.enforced || data.features[feature])
  const allowsPath = (path: string) => {
    const feature = domainPathFeature(path)
    if (!feature) return true
    const targetId = path.match(/^\/abonelikler\/(\d+)/)?.[1]
    return targetId === domainId && allows(feature)
  }
  return <DomainFeaturesContext.Provider value={{ domainId, ready: !!data, failed: state.key === key && !!state.failed, allows, allowsPath, retry: () => setAttempt(v => v + 1) }}>{children}</DomainFeaturesContext.Provider>
}


export function DomainFeatureGate({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation('common')
  const location = useLocation()
  const { domainId, ready, failed, allowsPath, retry } = useDomainFeatures()
  if (!/^\/abonelikler\/\d+(?:\/|$)/.test(location.pathname)) return <>{children}</>
  if (ready && allowsPath(location.pathname)) return <>{children}</>
  return (
    <div className="ta-page-fluid">
      <div className="ta-card p-6" role={failed || ready ? 'alert' : 'status'}>
        <h1 className="text-lg font-semibold text-slate-900 dark:text-slate-100">{t(failed ? 'plan_features.load_failed' : ready ? 'plan_features.blocked_title' : 'plan_features.loading')}</h1>
        {(failed || ready) && <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">{t(failed ? 'plan_features.load_failed_hint' : 'plan_features.blocked_hint')}</p>}
        {(failed || ready) && <div className="mt-4 flex flex-wrap gap-3">
          <button type="button" className="ta-secondary-button" onClick={retry}>{t('plan_features.retry')}</button>
          <Link className="ta-secondary-button" to={`/abonelikler/${domainId}`}>{t('plan_features.overview')}</Link>
        </div>}
      </div>
    </div>
  )
}
