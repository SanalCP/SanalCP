// Sunucu geneli DNS bakışı — hangi zone'da kaç kayıt var, MX/TXT eksiği olan
// domainler hangileri. Düzenleme hâlâ domain sayfasında (satırdan gidilir).
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import GenelListe, { type Kolon } from '@/components/GenelListe'

type Satir = {
  domain_id: number
  alan_adi: string
  durum: string
  kayit_sayisi: number
  a_sayisi: number
  mx_sayisi: number
  txt_sayisi: number
  pasif_sayisi: number
  dnssec_aktif: boolean
}

function Sayi({ n, uyariSifir, bosTitle }: { n: number; uyariSifir?: boolean; bosTitle?: string }) {
  if (n === 0 && uyariSifir) {
    return <span className="text-amber-600 dark:text-amber-400" title={bosTitle}>0</span>
  }
  return <span className={n === 0 ? 'text-slate-400' : ''}>{n}</span>
}

export default function DNSGenelPage() {
  const { t } = useTranslation(['DNSGenelPage', 'common'])
  const kolonlar: Kolon<Satir>[] = [
    {
      baslik: t('DNSGenelPage:col_domain'),
      dar: true,
      hucre: (s) => (
        <Link to={`/abonelikler/${s.domain_id}/dns`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
          {s.alan_adi}
        </Link>
      ),
    },
    { baslik: t('DNSGenelPage:col_records'), dar: true, hucre: (s) => <Sayi n={s.kayit_sayisi} uyariSifir bosTitle={t('DNSGenelPage:no_records_title')} /> },
    { baslik: t('DNSGenelPage:col_a'), dar: true, hucre: (s) => <Sayi n={s.a_sayisi} uyariSifir bosTitle={t('DNSGenelPage:no_records_title')} /> },
    { baslik: t('DNSGenelPage:col_mx'), dar: true, hucre: (s) => <Sayi n={s.mx_sayisi} /> },
    { baslik: t('DNSGenelPage:col_txt'), dar: true, hucre: (s) => <Sayi n={s.txt_sayisi} /> },
    {
      baslik: t('DNSGenelPage:col_passive'),
      dar: true,
      hucre: (s) => (s.pasif_sayisi > 0
        ? <span className="text-amber-600 dark:text-amber-400">{s.pasif_sayisi}</span>
        : <span className="text-slate-400">—</span>),
    },
    {
      baslik: t('DNSGenelPage:col_dnssec'),
      dar: true,
      hucre: (s) => (s.dnssec_aktif
        ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('DNSGenelPage:dnssec_on')}</span>
        : <span className="text-slate-400 text-xs">{t('DNSGenelPage:dnssec_off')}</span>),
    },
    {
      baslik: t('DNSGenelPage:col_domain_status'),
      dar: true,
      hucre: (s) => (s.durum === 'aktif'
        ? <span className="text-xs text-slate-500">{t('DNSGenelPage:domain_active')}</span>
        : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{t('DNSGenelPage:domain_passive')}</span>),
    },
  ]

  return (
    <GenelListe<Satir>
      baslik={t('DNSGenelPage:title')}
      aciklama={t('DNSGenelPage:subtitle')}
      uc="/genel/dns"
      kolonlar={kolonlar}
      araAlan={(s) => s.alan_adi}
      satirAnahtar={(s) => s.domain_id}
      bosMesaj={t('DNSGenelPage:empty')}
      ozet={(l) => [
        { etiket: t('DNSGenelPage:summary_zones'), deger: l.length },
        { etiket: t('DNSGenelPage:summary_total_records'), deger: l.reduce((tt, s) => tt + s.kayit_sayisi, 0) },
        ...(l.filter((s) => s.mx_sayisi === 0).length > 0
          ? [{ etiket: t('DNSGenelPage:summary_no_mx'), deger: l.filter((s) => s.mx_sayisi === 0).length, vurgu: 'uyari' as const }]
          : []),
        ...(l.filter((s) => s.dnssec_aktif).length > 0
          ? [{ etiket: t('DNSGenelPage:summary_dnssec_on'), deger: l.filter((s) => s.dnssec_aktif).length }]
          : []),
      ]}
    />
  )
}