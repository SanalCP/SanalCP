import { useTranslation } from 'react-i18next'
import { planFeatureKeys, type PlanFeatures } from '@/lib/planFeatures'

export default function PlanFeatureFields({ value, onChange, disabled = false }: { value: PlanFeatures; onChange?: (features: PlanFeatures) => void; disabled?: boolean }) {
  const { t } = useTranslation('common')
  return (
    <fieldset className="mt-4 mb-5 rounded-xl border border-slate-200 p-4 dark:border-slate-700" disabled={disabled || !onChange}>
      <legend className="px-1 text-sm font-semibold text-slate-900 dark:text-slate-100">{t('plan_features.title')}</legend>
      <p className="mb-3 text-xs text-slate-500 dark:text-slate-400">{t('plan_features.description')}</p>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {planFeatureKeys.map(key => <label key={key} className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
          <input type="checkbox" className="rounded" checked={value[key]} onChange={e => onChange?.({ ...value, [key]: e.target.checked })} />
          <span>{t(`plan_features.modules.${key}`)}</span>
          <span className="ml-auto text-xs text-slate-500">{t(value[key] ? 'plan_features.enabled' : 'plan_features.disabled')}</span>
        </label>)}
      </div>
    </fieldset>
  )
}
