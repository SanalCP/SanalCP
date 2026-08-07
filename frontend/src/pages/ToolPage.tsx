// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; alan_adi: string }

export default function ToolPage() {
  const { t } = useTranslation(['ToolPage', 'common'])
  const { id, slug } = useParams()
  const [d, setD] = useState<Domain | null>(null)
  const [hata, setHata] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(r => setD(r.data)).catch(e => setHata(apiHata(e)))
  }, [id])

  const titleKey = slug ? `ToolPage:tool_${slug}_title` : null
  const descKey = slug ? `ToolPage:tool_${slug}_desc` : null
  const phaseKey = slug ? `ToolPage:tool_${slug}_phase` : null
  const baslik = slug && t(titleKey!) !== titleKey ? t(titleKey!) : (slug || t('ToolPage:default_tool_name'))
  const aciklama = slug && t(descKey!) !== descKey ? t(descKey!) : t('ToolPage:not_yet')
  const faz = slug && phaseKey && t(phaseKey) !== phaseKey ? t(phaseKey) : undefined

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('ToolPage:breadcrumb_domains'), href: '/domainler' },
        { etiket: d?.alan_adi || t('ToolPage:loading'), href: `/abonelikler/${id}` },
        { etiket: baslik },
      ]} />

      <div className="flex items-center gap-3 mb-2">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{baslik}</h1>
        {faz && (
          <span className="text-[10px] font-semibold uppercase tracking-wider bg-amber-100 dark:bg-amber-900/30 text-amber-800 dark:text-amber-200 px-2 py-0.5 rounded">
            {faz} · {t('ToolPage:not_ready_suffix')}
          </span>
        )}
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-1">
        {d ? <>{t('ToolPage:domain_prefix')} <Link to={`/abonelikler/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300 font-medium">{d.alan_adi}</Link></> : t('ToolPage:loading')}
      </p>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">{aciklama}</p>
      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      <div className="bg-white dark:bg-slate-800 border-2 border-dashed border-slate-200 dark:border-slate-700 rounded-2xl p-12 text-center">
        <div className="w-16 h-16 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-3">
          <svg className="w-8 h-8 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h3 className="text-base font-semibold text-slate-700 dark:text-slate-300 mb-1">{t('ToolPage:wip_title')}</h3>
        <p className="text-sm text-slate-500 dark:text-slate-500">
          {faz ? t('ToolPage:wip_desc', { phase: <span className="font-mono text-brand-700 dark:text-brand-300">{faz}</span> }) : t('ToolPage:wip_desc_no_phase')}
        </p>
        <Link to={`/abonelikler/${id}`} className="inline-block mt-4 text-sm text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300 font-medium">
          {t('ToolPage:back_to_domain')}
        </Link>
      </div>
    </div>
  )
}
