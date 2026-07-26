// Sunucu geneli sertifika bakışı. Asıl amacı süresi dolmak üzere olanı
// gözden kaçırmamak: liste en yakın bitiş tarihine göre sıralı gelir.
import { Link } from 'react-router-dom'
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
function KalanRozet({ gun }: { gun: number | null }) {
  if (gun === null) return <span className="text-slate-400">—</span>
  if (gun < 0) {
    return <span className="px-2 py-0.5 rounded text-xs font-medium bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">{Math.abs(gun)} gün önce doldu</span>
  }
  if (gun <= 14) {
    return <span className="px-2 py-0.5 rounded text-xs font-medium bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">{gun} gün</span>
  }
  if (gun <= 30) {
    return <span className="px-2 py-0.5 rounded text-xs font-medium bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{gun} gün</span>
  }
  return <span className="text-slate-600 dark:text-slate-400">{gun} gün</span>
}

const kolonlar: Kolon<Satir>[] = [
  {
    baslik: 'Alan Adı',
    dar: true,
    hucre: (s) => (
      <Link to={`/abonelikler/${s.domain_id}/ssl`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
        {s.alan_adi}
      </Link>
    ),
  },
  {
    baslik: 'SSL',
    dar: true,
    hucre: (s) => (s.ssl_aktif
      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">Aktif</span>
      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">Yok</span>),
  },
  { baslik: 'Bitiş', dar: true, hucre: (s) => (s.ssl_bitis || <span className="text-slate-400">—</span>) },
  { baslik: 'Kalan', dar: true, hucre: (s) => <KalanRozet gun={s.kalan_gun} /> },
  {
    baslik: 'Domain',
    dar: true,
    hucre: (s) => (s.durum === 'aktif'
      ? <span className="text-xs text-slate-500">Aktif</span>
      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">Pasif</span>),
  },
  {
    baslik: '',
    dar: true,
    sinif: 'text-right',
    hucre: (s) => (
      <Link to={`/abonelikler/${s.domain_id}/ssl`} className="text-xs text-brand-600 dark:text-brand-400 hover:underline">
        {s.ssl_aktif ? 'Yönet' : 'Kur'}
      </Link>
    ),
  },
]

export default function SSLGenelPage() {
  return (
    <GenelListe<Satir>
      baslik="SSL Sertifikaları"
      aciklama="Tüm alan adlarının sertifika durumu ve bitiş tarihleri. En yakın bitiş en üstte."
      uc="/genel/ssl"
      kolonlar={kolonlar}
      araAlan={(s) => s.alan_adi}
      satirAnahtar={(s) => s.domain_id}
      bosMesaj="Henüz alan adı yok"
      ozet={(l) => {
        const dolmus = l.filter((s) => s.kalan_gun !== null && s.kalan_gun < 0).length
        const yakin = l.filter((s) => s.kalan_gun !== null && s.kalan_gun >= 0 && s.kalan_gun <= 30).length
        const yok = l.filter((s) => !s.ssl_aktif).length
        return [
          { etiket: 'sertifika aktif', deger: l.filter((s) => s.ssl_aktif).length },
          ...(dolmus > 0 ? [{ etiket: 'süresi dolmuş', deger: dolmus, vurgu: 'tehlike' as const }] : []),
          ...(yakin > 0 ? [{ etiket: '30 günden az', deger: yakin, vurgu: 'uyari' as const }] : []),
          ...(yok > 0 ? [{ etiket: 'SSL yok', deger: yok, vurgu: 'uyari' as const }] : []),
        ]
      }}
    />
  )
}
