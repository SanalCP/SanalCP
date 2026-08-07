// sanal-dark-swept
// sanal-dark-swept-v2
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

export type ToolbarButton = { etiket: string; onClick?: () => void; disabled?: boolean; ipucu?: string }

export default function ListToolbar({
  birincil, butonlar, arananSetter, aranan,
}: {
  birincil?: ToolbarButton
  butonlar?: ToolbarButton[]
  arananSetter?: (s: string) => void
  aranan?: string
}) {
  const { t } = useTranslation(['common'])
  return (
    <div className="flex items-stretch gap-2.5 mb-5 flex-wrap sm:items-center">
      {birincil && (
        <button
          onClick={birincil.onClick}
          disabled={birincil.disabled}
          title={birincil.ipucu}
          className="ta-primary-button"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          {birincil.etiket}
        </button>
      )}
      {(butonlar || []).map((b, i) => (
        <button
          key={i}
          onClick={b.onClick}
          disabled={b.disabled}
          title={b.ipucu}
          className="ta-secondary-button"
        >
          {b.etiket}
        </button>
      ))}
      {arananSetter !== undefined && (
        <div className="relative w-full sm:ml-auto sm:w-auto">
          <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            value={aranan || ''}
            onChange={(e) => arananSetter(e.target.value)}
            placeholder={t('common:search_placeholder')}
            className="ta-input w-full pl-9 sm:w-64"
          />
        </div>
      )}
    </div>
  )
}
