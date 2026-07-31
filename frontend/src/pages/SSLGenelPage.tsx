// Sunucu geneli sertifika bakışı. Asıl amacı süresi dolmak üzere olanı
// gözden kaçırmamak: liste en yakın bitiş tarihine göre sıralı gelir.
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import GenelListe, { type Kolon } from '@/components/GenelListe'

type Satir = {
  domain_id: number
  alan_adi: string
  durum: string
  ssl_aktif: boolean
  ssl_bitis: string
  kalan_gun: number | null
}

// Let's Encrypt 90 günlük sertifika veriyor ve 30 gün kala yeniler; 14 günün
// altı "yenileme çalışmamış" demektir, o yüzden ayrı eşik.
function KalanRozet({ gun, t }: { gun: number | null; t: (k: string, opts?: Record<string, unknown>) => string }) {
  if (gun === null) return <span className="text-slate-400">—</span>
  if (gun < 0) {
    return <span className="px-2 py-0.5 rounded text-xs font-medium bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">{t('SSLGenelPage:days_ago_expired', { n: Math.abs(gun) })}</span>
  }
  if (gun <= 14) {
    return <span className="px-2 py-0.5 rounded text-xs font-medium bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">{t('SSLGenelPage:days_remaining', { n: gun })}</span>
  }
  if (gun <= 30) {
    return <span className="px-2 py-0.5 rounded text-xs font-medium bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{t('SSLGenelPage:days_remaining', { n: gun })}</span>
  }
  return <span className="text-slate-600 dark:text-slate-400">{t('SSLGenelPage:days_remaining', { n: gun })}</span>
}

export default function SSLGenelPage() {
  const { t } = useTranslation(['SSLGenelPage', 'common'])
  const kolonlar: Kolon<Satir>[] = [
    {
      baslik: t('SSLGenelPage:col_domain'),
      dar: true,
      hucre: (s) => (
        <Link to={`/abonelikler/${s.domain_id}/ssl`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
          {s.alan_adi}
        </Link>
      ),
    },
    {
      baslik: t('SSLGenelPage:col_ssl'),
      dar: true,
      hucre: (s) => (s.ssl_aktif
        ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('SSLGenelPage:ssl_active')}</span>
        : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{t('SSLGenelPage:ssl_none')}</span>),
    },
    { baslik: t('SSLGenelPage:col_expires'), dar: true, hucre: (s) => (s.ssl_bitis || <span className="text-slate-400">—</span>) },
    { baslik: t('SSLGenelPage:col_remaining'), dar: true, hucre: (s) => <KalanRozet gun={s.kalan_gun} t={t} /> },
    {
      baslik: t('SSLGenelPage:col_domain_status'),
      dar: true,
      hucre: (s) => (s.durum === 'aktif'
        ? <span className="text-xs text-slate-500">{t('SSLGenelPage:domain_active')}</span>
        : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{t('SSLGenelPage:domain_passive')}</span>),
    },
    {
      baslik: '',
      dar: true,
      sinif: 'text-right',
      hucre: (s) => (
        <Link to={`/abonelikler/${s.domain_id}/ssl`} className="text-xs text-brand-600 dark:text-brand-400 hover:underline">
          {s.ssl_aktif ? t('SSLGenelPage:manage_link') : t('SSLGenelPage:install_link')}
        </Link>
      ),
    },
  ]

  return (
    <GenelListe<Satir>
      baslik={t('SSLGenelPage:title')}
      aciklama={t('SSLGenelPage:subtitle')}
      uc="/genel/ssl"
      kolonlar={kolonlar}
      araAlan={(s) => s.alan_adi}
      satirAnahtar={(s) => s.domain_id}
      bosMesaj={t('SSLGenelPage:empty')}
      ozet={(l) => {
        const dolmus = l.filter((s) => s.kalan_gun !== null && s.kalan_gun < 0).length
        const yakin = l.filter((s) => s.kalan_gun !== null && s.kalan_gun >= 0 && s.kalan_gun <= 30).length
        const yok = l.filter((s) => !s.ssl_aktif).length
        return [
          { etiket: t('SSLGenelPage:summary_active'), deger: l.filter((s) => s.ssl_aktif).length },
          ...(dolmus > 0 ? [{ etiket: t('SSLGenelPage:summary_expired'), deger: dolmus, vurgu: 'tehlike' as const }] : []),
          ...(yakin > 0 ? [{ etiket: t('SSLGenelPage:summary_expiring_soon'), deger: yakin, vurgu: 'uyari' as const }] : []),
          ...(yok > 0 ? [{ etiket: t('SSLGenelPage:summary_no_ssl'), deger: yok, vurgu: 'uyari' as const }] : []),
        ]
      }}
    />
  )
}