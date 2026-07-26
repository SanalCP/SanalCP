// Sunucu geneli veritabanı bakışı. Boyutlar information_schema'dan gelir —
// hangi veritabanının diski yediğini görmek için buradaki tek yer.
import { Link } from 'react-router-dom'
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

function boyutYaz(kb: number): string {
  if (kb <= 0) return '—'
  if (kb < 1024) return `${kb} KB`
  if (kb < 1024 * 1024) return `${(kb / 1024).toFixed(1)} MB`
  return `${(kb / 1024 / 1024).toFixed(2)} GB`
}

const kolonlar: Kolon<Satir>[] = [
  {
    baslik: 'Veritabanı',
    dar: true,
    hucre: (s) => <span className="font-mono text-slate-900 dark:text-slate-100">{s.db_adi}</span>,
  },
  { baslik: 'Kullanıcı', dar: true, hucre: (s) => <span className="font-mono text-xs">{s.db_user}</span> },
  {
    baslik: 'Alan Adı',
    dar: true,
    hucre: (s) => (
      <Link to={`/abonelikler/${s.domain_id}/veritabanlari`} className="text-slate-700 dark:text-slate-300 hover:text-brand-600 dark:hover:text-brand-400 transition">
        {s.alan_adi}
      </Link>
    ),
  },
  { baslik: 'Sunucu', dar: true, hucre: (s) => <span className="text-xs text-slate-500">{s.db_host}</span> },
  {
    baslik: 'Boyut',
    dar: true,
    sinif: 'text-right',
    hucre: (s) => <span className={s.boyut_kb > 1024 * 1024 ? 'text-amber-600 dark:text-amber-400' : ''}>{boyutYaz(s.boyut_kb)}</span>,
  },
  { baslik: 'Oluşturma', dar: true, hucre: (s) => <span className="text-xs text-slate-500">{s.olusturma || '—'}</span> },
]

export default function VeritabanlariGenelPage() {
  return (
    <GenelListe<Satir>
      baslik="Veritabanları"
      aciklama="Sunucudaki tüm MySQL/MariaDB veritabanları ve disk kullanımları."
      uc="/genel/veritabanlari"
      kolonlar={kolonlar}
      araAlan={(s) => `${s.db_adi} ${s.db_user} ${s.alan_adi}`}
      satirAnahtar={(s) => s.id}
      bosMesaj="Henüz veritabanı yok"
      ozet={(l) => {
        const toplam = l.reduce((t, s) => t + s.boyut_kb, 0)
        return [
          { etiket: 'veritabanı', deger: l.length },
          { etiket: 'toplam boyut', deger: boyutYaz(toplam) },
          { etiket: 'farklı kullanıcı', deger: new Set(l.map((s) => s.db_user)).size },
        ]
      }}
    />
  )
}
