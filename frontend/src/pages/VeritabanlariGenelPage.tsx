// Sunucu geneli veritabanı bakışı. Boyutlar information_schema'dan gelir —
// hangi veritabanının diski yediğini görmek için buradaki tek yer.
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import GenelListe, { type Kolon } from '@/components/GenelListe'

type Satir = {
  id: number
  domain_id: number
  alan_adi: string
  db_adi: string
  db_user: string
  db_host: string
  boyut_kb: number
  olusturma: string
}

function boyutYaz(kb: number, t: (k: string, opts?: Record<string, unknown>) => string): string {
  if (kb <= 0) return t('VeritabanlariGenelPage:size_empty')
  if (kb < 1024) return t('VeritabanlariGenelPage:size_kb', { n: kb })
  if (kb < 1024 * 1024) return t('VeritabanlariGenelPage:size_mb', { n: (kb / 1024).toFixed(1) })
  return t('VeritabanlariGenelPage:size_gb', { n: (kb / 1024 / 1024).toFixed(2) })
}

export default function VeritabanlariGenelPage() {
  const { t } = useTranslation(['VeritabanlariGenelPage', 'common'])
  const kolonlar: Kolon<Satir>[] = [
    {
      baslik: t('VeritabanlariGenelPage:col_db'),
      dar: true,
      hucre: (s) => <span className="font-mono text-slate-900 dark:text-slate-100">{s.db_adi}</span>,
    },
    { baslik: t('VeritabanlariGenelPage:col_user'), dar: true, hucre: (s) => <span className="font-mono text-xs">{s.db_user}</span> },
    {
      baslik: t('VeritabanlariGenelPage:col_domain'),
      dar: true,
      hucre: (s) => (
        <Link to={`/abonelikler/${s.domain_id}/veritabanlari`} className="text-slate-700 dark:text-slate-300 hover:text-brand-600 dark:hover:text-brand-400 transition">
          {s.alan_adi}
        </Link>
      ),
    },
    { baslik: t('VeritabanlariGenelPage:col_server'), dar: true, hucre: (s) => <span className="text-xs text-slate-500">{s.db_host}</span> },
    {
      baslik: t('VeritabanlariGenelPage:col_size'),
      dar: true,
      sinif: 'text-right',
      hucre: (s) => <span className={s.boyut_kb > 1024 * 1024 ? 'text-amber-600 dark:text-amber-400' : ''}>{boyutYaz(s.boyut_kb, t)}</span>,
    },
    { baslik: t('VeritabanlariGenelPage:col_created'), dar: true, hucre: (s) => <span className="text-xs text-slate-500">{s.olusturma || t('VeritabanlariGenelPage:size_empty')}</span> },
  ]

  return (
    <GenelListe<Satir>
      baslik={t('VeritabanlariGenelPage:title')}
      aciklama={t('VeritabanlariGenelPage:subtitle')}
      uc="/genel/veritabanlari"
      kolonlar={kolonlar}
      araAlan={(s) => `${s.db_adi} ${s.db_user} ${s.alan_adi}`}
      satirAnahtar={(s) => s.id}
      bosMesaj={t('VeritabanlariGenelPage:empty')}
      ozet={(l) => {
        const toplam = l.reduce((t, s) => t + s.boyut_kb, 0)
        return [
          { etiket: t('VeritabanlariGenelPage:summary_count'), deger: l.length },
          { etiket: t('VeritabanlariGenelPage:summary_total_size'), deger: boyutYaz(toplam, t) },
          { etiket: t('VeritabanlariGenelPage:summary_unique_users'), deger: new Set(l.map((s) => s.db_user)).size },
        ]
      }}
    />
  )
}