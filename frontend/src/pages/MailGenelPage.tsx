// Sunucu geneli e-posta bakışı: hangi domainde posta barındırma açık, kaç
// kutu ve yönlendirme var. Kutu eklemek/silmek domain sayfasında.
import { Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import GenelListe, { type Kolon } from '@/components/GenelListe'
import { api, apiHata } from '@/lib/api'

type Satir = {
  domain_id: number
  alan_adi: string
  mail_aktif: boolean
  mail_durum: string // active | suspended | ''
  kutu_sayisi: number
  alias_sayisi: number
  pasif_kutu: number
}
type QueueRecipient = { address: string; delay_reason?: string }
type QueueMessage = {
  queue_id: string; queue_name: string; arrival_time: number; message_size: number
  sender: string; recipients: QueueRecipient[]
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
  const [queue, setQueue] = useState<QueueMessage[]>([])
  const [queueHata, setQueueHata] = useState<string | null>(null)
  const [queueYuk, setQueueYuk] = useState(true)
  const [queueIslem, setQueueIslem] = useState('')

  function queueYukle() {
    setQueueYuk(true); setQueueHata(null)
    api.get<{ messages: QueueMessage[] }>('/admin/mail/queue')
      .then(r => setQueue(r.data.messages || []))
      .catch(e => setQueueHata(apiHata(e, 'Posta kuyruğu okunamadı')))
      .finally(() => setQueueYuk(false))
  }
  useEffect(queueYukle, [])

  async function queueAction(action: 'flush'|'delete'|'hold'|'release'|'requeue', queue_id = '') {
    if (action === 'delete' && !confirm(`${queue_id} kuyruk iletisi kalıcı olarak silinsin mi?`)) return
    setQueueIslem(action + queue_id); setQueueHata(null)
    try {
      await api.post('/admin/mail/queue', { action, queue_id })
      queueYukle()
    } catch (e) {
      setQueueHata(apiHata(e, 'Kuyruk işlemi başarısız'))
    } finally {
      setQueueIslem('')
    }
  }

  return (
    <>
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
      <div className="w-full max-w-[1600px] px-6 pb-8">
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
          <div className="p-5 flex items-center justify-between gap-3 border-b border-slate-200 dark:border-slate-700">
            <div>
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Postfix Mail Kuyruğu</h2>
              <p className="text-xs text-slate-500 mt-1">Teslim bekleyen, ertelenen veya yönetici tarafından bekletilen iletiler.</p>
            </div>
            <div className="flex gap-2">
              <button onClick={queueYukle} disabled={queueYuk}
                className="px-3 py-1.5 text-xs border border-slate-300 dark:border-slate-600 rounded">↻ Yenile</button>
              <button onClick={() => queueAction('flush')} disabled={!!queueIslem}
                className="px-3 py-1.5 text-xs bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded disabled:opacity-50">
                Kuyruğu yeniden dene
              </button>
            </div>
          </div>
          {queueHata && <div className="m-4 p-3 text-sm text-red-700 bg-red-50 dark:bg-red-900/20 dark:text-red-300 rounded">{queueHata}</div>}
          {queueYuk ? <div className="p-8 text-center text-sm text-slate-400">Kuyruk okunuyor…</div> :
           queue.length === 0 ? <div className="p-8 text-center text-sm text-emerald-600 dark:text-emerald-400">✓ Mail kuyruğu boş</div> :
           <div className="overflow-x-auto">
             <table className="w-full text-sm">
               <thead className="bg-slate-50 dark:bg-slate-900 text-xs text-slate-500">
                 <tr><th className="text-left p-3">Kuyruk ID</th><th className="text-left p-3">Gönderen → Alıcı</th><th className="text-left p-3">Boyut / Zaman</th><th className="text-right p-3">İşlemler</th></tr>
               </thead>
               <tbody className="divide-y divide-slate-100 dark:divide-slate-700">
                 {queue.map(m => <tr key={m.queue_id}>
                   <td className="p-3 font-mono">{m.queue_id}<div className="text-[10px] text-slate-400">{m.queue_name}</div></td>
                   <td className="p-3">
                     <div className="font-mono text-xs">{m.sender || '<>'}</div>
                     <div className="font-mono text-xs text-slate-500">→ {m.recipients.map(r => r.address).join(', ')}</div>
                     {m.recipients.find(r => r.delay_reason)?.delay_reason &&
                       <div className="mt-1 text-[10px] text-amber-600 max-w-xl">{m.recipients.find(r => r.delay_reason)?.delay_reason}</div>}
                   </td>
                   <td className="p-3 text-xs text-slate-500">{formatBoyut(m.message_size)}<div>{new Date(m.arrival_time * 1000).toLocaleString('tr-TR')}</div></td>
                   <td className="p-3 text-right whitespace-nowrap">
                     {m.queue_name === 'hold' ?
                       <button onClick={() => queueAction('release', m.queue_id)} className="text-xs text-emerald-600 px-2">Serbest bırak</button> :
                       <button onClick={() => queueAction('hold', m.queue_id)} className="text-xs text-amber-600 px-2">Beklet</button>}
                     <button onClick={() => queueAction('requeue', m.queue_id)} className="text-xs text-brand-600 px-2">Yeniden sırala</button>
                     <button onClick={() => queueAction('delete', m.queue_id)} className="text-xs text-red-600 px-2">Sil</button>
                   </td>
                 </tr>)}
               </tbody>
             </table>
           </div>}
        </div>
      </div>
    </>
  )
}

function formatBoyut(b: number) {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  return `${(b / 1024 / 1024).toFixed(1)} MB`
}
