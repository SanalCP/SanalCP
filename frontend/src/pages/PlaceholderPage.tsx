// sanal-dark-swept
// sanal-dark-swept-v2
import { useTranslation } from 'react-i18next'
import Breadcrumb from '@/components/Breadcrumb'

export default function PlaceholderPage({
  baslik, faz, aciklama, parent,
}: {
  baslik: string; faz?: string; aciklama: string
  parent?: { etiket: string; href: string }
}) {
  const { t } = useTranslation(['PlaceholderPage', 'common'])
  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        ...(parent ? [parent] : []),
        { etiket: baslik },
      ]} />
      <div className="flex items-center gap-3 mb-2">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{baslik}</h1>
        {faz && (
          <span className="text-[10px] font-semibold uppercase tracking-wider bg-amber-100 dark:bg-amber-900/30 text-amber-800 dark:text-amber-200 px-2 py-0.5 rounded">
            {t('PlaceholderPage:phase_badge', { phase: faz })}
          </span>
        )}
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">{aciklama}</p>

      <div className="bg-white dark:bg-slate-800 border-2 border-dashed border-slate-200 dark:border-slate-700 rounded-2xl p-12 text-center">
        <div className="w-16 h-16 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-3">
          <svg className="w-8 h-8 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h3 className="text-base font-semibold text-slate-700 dark:text-slate-300 mb-1">{t('PlaceholderPage:wip_title')}</h3>
        <p className="text-sm text-slate-500 dark:text-slate-500">{t('PlaceholderPage:wip_prefix')} {faz ? <span className="font-mono text-brand-700 dark:text-brand-300">{faz}</span> : t('PlaceholderPage:wip_next_phases')} {t('PlaceholderPage:wip_suffix')}</p>
      </div>
    </div>
  )
}