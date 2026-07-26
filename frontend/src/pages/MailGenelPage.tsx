// Sunucu geneli e-posta bakışı: hangi domainde posta barındırma açık, kaç
// kutu ve yönlendirme var. Kutu eklemek/silmek domain sayfasında.
import { Link } from 'react-router-dom'
import GenelListe, { type Kolon } from '@/components/GenelListe'

type Satir = {
  domain_id: number
  alan_adi: string
  mail_aktif: boolean
  mail_durum: string // active | suspended | ''
  kutu_sayisi: number
  alias_sayisi: number
  pasif_kutu: number
}

const kolonlar: Kolon<Satir>[] = [
  {
    baslik: 'Alan Adı',
    dar: true,
    hucre: (s) => (
      <Link to={`/abonelikler/${s.domain_id}/mail`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
        {s.alan_adi}
      </Link>
    ),
  },
  {
    baslik: 'Posta',
    dar: true,
    hucre: (s) => {
      if (!s.mail_aktif) return <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">Kapalı</span>
      if (s.mail_durum === 'suspended') return <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">Askıda</span>
      return <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">Açık</span>
    },
  },
  {
    baslik: 'Kutu',
    dar: true,
    hucre: (s) => (s.kutu_sayisi === 0
      ? <span className="text-slate-400">—</span>
      : <span>{s.kutu_sayisi}</span>),
  },
  {
    baslik: 'Yönlendirme',
    dar: true,
    hucre: (s) => (s.alias_sayisi === 0
      ? <span className="text-slate-400">—</span>
      : <span>{s.alias_sayisi}</span>),
  },
  {
    baslik: 'Askıda Kutu',
    dar: true,
    hucre: (s) => (s.pasif_kutu > 0
      ? <span className="text-amber-600 dark:text-amber-400">{s.pasif_kutu}</span>
      : <span className="text-slate-400">—</span>),
  },
  {
    baslik: '',
    dar: true,
    sinif: 'text-right',
    hucre: (s) => (
      <Link to={`/abonelikler/${s.domain_id}/mail`} className="text-xs text-brand-600 dark:text-brand-400 hover:underline">
        {s.mail_aktif ? 'Yönet' : 'Etkinleştir'}
      </Link>
    ),
  },
]

export default function MailGenelPage() {
  return (
    <GenelListe<Satir>
      baslik="E-posta Hesapları"
      aciklama="Sunucudaki tüm posta kutuları ve yönlendirmeler. Kutu eklemek için alan adına tıklayın."
      uc="/genel/mail"
      kolonlar={kolonlar}
      araAlan={(s) => s.alan_adi}
      satirAnahtar={(s) => s.domain_id}
      bosMesaj="Henüz alan adı yok"
      ozet={(l) => {
        const askida = l.reduce((t, s) => t + s.pasif_kutu, 0)
        return [
          { etiket: 'posta açık domain', deger: l.filter((s) => s.mail_aktif).length },
          { etiket: 'toplam kutu', deger: l.reduce((t, s) => t + s.kutu_sayisi, 0) },
          { etiket: 'yönlendirme', deger: l.reduce((t, s) => t + s.alias_sayisi, 0) },
          ...(askida > 0 ? [{ etiket: 'askıda kutu', deger: askida, vurgu: 'uyari' as const }] : []),
        ]
      }}
    />
  )
}
