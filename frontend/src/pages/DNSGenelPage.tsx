// Sunucu geneli DNS bakışı — hangi zone'da kaç kayıt var, MX/TXT eksiği olan
// domainler hangileri. Düzenleme hâlâ domain sayfasında (satırdan gidilir).
import { Link } from 'react-router-dom'
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

function Sayi({ n, uyariSifir }: { n: number; uyariSifir?: boolean }) {
  if (n === 0 && uyariSifir) {
    return <span className="text-amber-600 dark:text-amber-400" title="Bu tipte kayıt yok">0</span>
  }
  return <span className={n === 0 ? 'text-slate-400' : ''}>{n}</span>
}

const kolonlar: Kolon<Satir>[] = [
  {
    baslik: 'Alan Adı',
    dar: true,
    hucre: (s) => (
      <Link to={`/abonelikler/${s.domain_id}/dns`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
        {s.alan_adi}
      </Link>
    ),
  },
  { baslik: 'Kayıt', dar: true, hucre: (s) => <Sayi n={s.kayit_sayisi} uyariSifir /> },
  { baslik: 'A', dar: true, hucre: (s) => <Sayi n={s.a_sayisi} uyariSifir /> },
  { baslik: 'MX', dar: true, hucre: (s) => <Sayi n={s.mx_sayisi} /> },
  { baslik: 'TXT', dar: true, hucre: (s) => <Sayi n={s.txt_sayisi} /> },
  {
    baslik: 'Pasif',
    dar: true,
    hucre: (s) => (s.pasif_sayisi > 0
      ? <span className="text-amber-600 dark:text-amber-400">{s.pasif_sayisi}</span>
      : <span className="text-slate-400">—</span>),
  },
  {
    baslik: 'DNSSEC',
    dar: true,
    hucre: (s) => (s.dnssec_aktif
      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">Açık</span>
      : <span className="text-slate-400 text-xs">Kapalı</span>),
  },
  {
    baslik: 'Domain',
    dar: true,
    hucre: (s) => (s.durum === 'aktif'
      ? <span className="text-xs text-slate-500">Aktif</span>
      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">Pasif</span>),
  },
]

export default function DNSGenelPage() {
  return (
    <GenelListe<Satir>
      baslik="DNS Yönetimi"
      aciklama="Sunucudaki tüm zone'ların özeti. Kayıtları düzenlemek için alan adına tıklayın."
      uc="/genel/dns"
      kolonlar={kolonlar}
      araAlan={(s) => s.alan_adi}
      satirAnahtar={(s) => s.domain_id}
      bosMesaj="Henüz DNS zone'u olan alan adı yok"
      ozet={(l) => [
        { etiket: 'zone', deger: l.length },
        { etiket: 'toplam kayıt', deger: l.reduce((t, s) => t + s.kayit_sayisi, 0) },
        ...(l.filter((s) => s.mx_sayisi === 0).length > 0
          ? [{ etiket: 'MX kaydı yok', deger: l.filter((s) => s.mx_sayisi === 0).length, vurgu: 'uyari' as const }]
          : []),
        ...(l.filter((s) => s.dnssec_aktif).length > 0
          ? [{ etiket: 'DNSSEC açık', deger: l.filter((s) => s.dnssec_aktif).length }]
          : []),
      ]}
    />
  )
}
