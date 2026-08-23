import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { uretGucluParola } from '@/lib/parola'

// Şifre belirlenen tüm ekranlarda ortak: göster/gizle + güvenli üreteç.
export default function ParolaGirdisi({
  value, onChange, onUret, placeholder, autoComplete, className, inputId, required,
}: {
  value: string
  onChange: (v: string) => void
  onUret?: (v: string) => void
  placeholder?: string
  autoComplete?: string
  className?: string
  inputId?: string
  required?: boolean
}) {
  const { t } = useTranslation(['common'])
  const [gizli, setGizli] = useState(true)

  function uret() {
    const p = uretGucluParola()
    setGizli(false)
    if (onUret) onUret(p)
    else onChange(p)
  }

  return (
    <div className="flex items-center gap-1 flex-wrap">
      <input
        id={inputId}
        type={gizli ? 'password' : 'text'}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        required={required}
        className={className ?? 'ta-input w-full font-mono'}
      />
      <button
        type="button"
        onClick={() => setGizli(g => !g)}
        title={gizli ? t('common:show_password') : t('common:hide_password')}
        className="whitespace-nowrap px-2 py-2 bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 text-sm rounded-md"
      >
        {gizli ? t('common:show_password') : t('common:hide_password')}
      </button>
      <button
        type="button"
        onClick={uret}
        title={t('common:generate_password')}
        className="whitespace-nowrap px-3 py-2 bg-white dark:bg-slate-800 border border-brand-600 text-brand-700 dark:text-brand-300 hover:bg-brand-50 dark:hover:bg-brand-900/30 text-sm rounded-md"
      >
        {t('common:generate_password')}
      </button>
    </div>
  )
}
