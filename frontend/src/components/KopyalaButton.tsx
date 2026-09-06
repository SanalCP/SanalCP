import { useEffect, useRef, useState, type ButtonHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import { panoYaz } from '@/lib/pano'

type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onClick' | 'type'> & {
  metin: string
  /** IP/parola gibi doğrudan tıklanan değerleri geri bildirim sırasında korur. */
  icerigiKoru?: boolean
}

export default function KopyalaButton({ metin, icerigiKoru = false, children, className = 'ta-secondary-button', disabled, ...props }: Props) {
  const { t } = useTranslation('common')
  const [durum, setDurum] = useState<'hazir' | 'kopyalaniyor' | 'kopyalandi' | 'elle' | 'hata'>('hazir')
  const zamanlayici = useRef<ReturnType<typeof setTimeout>>()
  const nesil = useRef(0)
  const mesgul = useRef(false)

  useEffect(() => {
    const etkinNesil = nesil.current
    setDurum('hazir')
    mesgul.current = false
    return () => {
      nesil.current = etkinNesil + 1
      clearTimeout(zamanlayici.current)
    }
  }, [metin])

  async function kopyala() {
    if (mesgul.current) return
    mesgul.current = true
    const istek = nesil.current
    clearTimeout(zamanlayici.current)
    setDurum('kopyalaniyor')
    let sonuc: 'kopyalandi' | 'elle' | 'hata'
    try {
      sonuc = await panoYaz(metin, t('copy_manual_hint')) ? 'kopyalandi' : 'elle'
    } catch {
      sonuc = 'hata'
    }
    // Değer değiştiyse veya buton kaldırıldıysa eski sonuç yeni değere ait değildir.
    if (istek !== nesil.current) return
    mesgul.current = false
    setDurum(sonuc)
    zamanlayici.current = setTimeout(() => setDurum('hazir'), sonuc === 'kopyalandi' ? 2000 : 4000)
  }

  const etiket = durum === 'kopyalandi' ? t('copied')
    : durum === 'kopyalaniyor' ? t('copying')
      : durum === 'elle' ? t('copy_manual') : durum === 'hata' ? t('copy_failed') : ''

  return <button
    {...props}
    type="button"
    onClick={() => { void kopyala() }}
    disabled={disabled || durum === 'kopyalaniyor'}
    aria-busy={durum === 'kopyalaniyor'}
    className={`${className} transition duration-150 active:scale-95 motion-reduce:transform-none motion-reduce:transition-none disabled:cursor-wait ${durum === 'kopyalandi' ? 'ring-2 ring-emerald-400/70' : ''}`}
  >
    {(icerigiKoru || durum === 'hazir') && (children ?? t('copy'))}
    <span role="status" aria-live="polite" aria-atomic="true" className={`${icerigiKoru && etiket ? 'ml-2 ' : ''}inline-flex items-center gap-1 ${durum === 'kopyalandi' ? 'text-emerald-600 dark:text-emerald-400' : ''}`}>
      {durum === 'kopyalandi' && <span aria-hidden="true">✓</span>}
      {durum === 'kopyalaniyor' && <span aria-hidden="true" className="h-3 w-3 animate-spin rounded-full border-2 border-current border-t-transparent motion-reduce:animate-none" />}
      {etiket}
    </span>
  </button>
}
