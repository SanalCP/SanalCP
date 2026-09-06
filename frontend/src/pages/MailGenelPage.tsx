// Sunucu geneli e-posta bakışı: hangi domainde posta barındırma açık, kaç
// kutu ve yönlendirme var. Kutu eklemek/silmek domain sayfasında.
import { modalOnay } from '@/lib/dialog'
import { Link } from 'react-router-dom'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import GenelListe, { type Kolon } from '@/components/GenelListe'
import { api, apiHata } from '@/lib/api'
import { T } from '@/lib/tablo'

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

function formatBoyut(b: number, t: (k: string, opts?: Record<string, unknown>) => string): string {
  if (b < 1024) return t('MailGenelPage:size_b', { n: b })
  if (b < 1024 * 1024) return t('MailGenelPage:size_kb', { n: (b / 1024).toFixed(1) })
  return t('MailGenelPage:size_mb', { n: (b / 1024 / 1024).toFixed(1) })
}

export default function MailGenelPage() {
  const { t } = useTranslation(['MailGenelPage', 'common'])
  const kolonlar: Kolon<Satir>[] = [
    {
      baslik: t('MailGenelPage:col_domain'),
      dar: true,
      hucre: (s) => (
        <Link to={`/abonelikler/${s.domain_id}/mail`} className="font-medium text-slate-900 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400 transition">
          {s.alan_adi}
        </Link>
      ),
    },
    {
      baslik: t('MailGenelPage:col_mail'),
      dar: true,
      hucre: (s) => {
        if (!s.mail_aktif) return <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{t('MailGenelPage:mail_off')}</span>
        if (s.mail_durum === 'suspended') return <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{t('MailGenelPage:mail_suspended')}</span>
        return <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('MailGenelPage:mail_active')}</span>
      },
    },
    {
      baslik: t('MailGenelPage:col_mailbox'),
      dar: true,
      hucre: (s) => (s.kutu_sayisi === 0
        ? <span className="text-slate-400">—</span>
        : <span>{s.kutu_sayisi}</span>),
    },
    {
      baslik: t('MailGenelPage:col_alias'),
      dar: true,
      hucre: (s) => (s.alias_sayisi === 0
        ? <span className="text-slate-400">—</span>
        : <span>{s.alias_sayisi}</span>),
    },
    {
      baslik: t('MailGenelPage:col_suspended'),
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
          {s.mail_aktif ? t('MailGenelPage:manage_link') : t('MailGenelPage:enable_link')}
        </Link>
      ),
    },
  ]

  const [queue, setQueue] = useState<QueueMessage[]>([])
  const [queueHata, setQueueHata] = useState<string | null>(null)
  const [queueYuk, setQueueYuk] = useState(true)
  const [queueIslem, setQueueIslem] = useState('')

  const queueYukle = useCallback(() => {
    setQueueYuk(true); setQueueHata(null)
    api.get<{ messages: QueueMessage[] }>('/admin/mail/queue')
      .then(r => setQueue(r.data.messages || []))
      .catch(e => setQueueHata(apiHata(e, t('MailGenelPage:queue_failed'))))
      .finally(() => setQueueYuk(false))
  }, [t])
  useEffect(queueYukle, [queueYukle])

  async function queueAction(action: 'flush'|'delete'|'hold'|'release'|'requeue', queue_id = '') {
    if (action === 'delete' && !(await modalOnay(t('MailGenelPage:confirm_delete_queue', { id: queue_id })))) return
    setQueueIslem(action + queue_id); setQueueHata(null)
    try {
      await api.post('/admin/mail/queue', { action, queue_id })
      queueYukle()
    } catch (e) {
      setQueueHata(apiHata(e, t('MailGenelPage:queue_action_failed')))
    } finally {
      setQueueIslem('')
    }
  }

  const locale = t('MailGenelPage:datetime_locale')

  return (
    <>
      <GenelListe<Satir>
        baslik={t('MailGenelPage:title')}
        aciklama={t('MailGenelPage:subtitle')}
        uc="/genel/mail"
        kolonlar={kolonlar}
        araAlan={(s) => s.alan_adi}
        satirAnahtar={(s) => s.domain_id}
        bosMesaj={t('MailGenelPage:empty')}
        ozet={(l) => {
          const askida = l.reduce((tt, s) => tt + s.pasif_kutu, 0)
          return [
            { etiket: t('MailGenelPage:summary_active_domains'), deger: l.filter((s) => s.mail_aktif).length },
            { etiket: t('MailGenelPage:summary_total_mailboxes'), deger: l.reduce((tt, s) => tt + s.kutu_sayisi, 0) },
            { etiket: t('MailGenelPage:summary_aliases'), deger: l.reduce((tt, s) => tt + s.alias_sayisi, 0) },
            ...(askida > 0 ? [{ etiket: t('MailGenelPage:summary_suspended_mailboxes'), deger: askida, vurgu: 'uyari' as const }] : []),
          ]
        }}
      />
      <div className="w-full px-6 pb-8">
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
          <div className="p-5 flex flex-col items-stretch justify-between gap-3 border-b border-slate-200 dark:border-slate-700 sm:flex-row sm:items-center">
            <div>
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('MailGenelPage:queue_title')}</h2>
              <p className="text-xs text-slate-500 mt-1">{t('MailGenelPage:queue_subtitle')}</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <button onClick={queueYukle} disabled={queueYuk}
                className="px-3 py-1.5 text-xs border border-slate-300 dark:border-slate-600 rounded">{t('MailGenelPage:refresh')}</button>
              <button onClick={() => queueAction('flush')} disabled={!!queueIslem}
                className="px-3 py-1.5 text-xs bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded disabled:opacity-50">
                {t('MailGenelPage:queue_retry')}
              </button>
            </div>
          </div>
          {queueHata && <div className="m-4 p-3 text-sm text-red-700 bg-red-50 dark:bg-red-900/20 dark:text-red-300 rounded">{queueHata}</div>}
          {queueYuk ? <div className="p-8 text-center text-sm text-slate-400">{t('MailGenelPage:queue_loading')}</div> :
           queue.length === 0 ? <div className="p-8 text-center text-sm text-emerald-600 dark:text-emerald-400">{t('MailGenelPage:queue_empty')}</div> :
           <div className="lg:overflow-x-auto">
             <table className={T.tablo}>
               <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900 text-xs text-slate-500`}>
                 <tr><th className={T.baslik}>{t('MailGenelPage:col_queue_id')}</th><th className={T.baslik}>{t('MailGenelPage:col_sender_recipients')}</th><th className={T.baslik}>{t('MailGenelPage:col_size_time')}</th><th className={`${T.baslik} text-right`}>{t('MailGenelPage:col_actions')}</th></tr>
               </thead>
               <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-700`}>
                 {queue.map(m => <tr key={m.queue_id} className={T.satir}>
                   <td className={`${T.hucreBaslik} font-mono`}>{m.queue_id}<div className="text-[10px] text-slate-400">{m.queue_name}</div></td>
                   <td className={T.hucre} data-etiket={t('MailGenelPage:col_sender_recipients')}>
                     <div className="min-w-0 text-right lg:text-left"><div className="break-all font-mono text-xs">{m.sender || t('MailGenelPage:no_recipient')}</div>
                     <div className="break-all font-mono text-xs text-slate-500">→ {m.recipients.map(r => r.address).join(', ')}</div>
                     {m.recipients.find(r => r.delay_reason)?.delay_reason &&
                       <div className="mt-1 max-w-xl break-words text-[10px] text-amber-600">{m.recipients.find(r => r.delay_reason)?.delay_reason}</div>}</div>
                   </td>
                   <td className={`${T.hucre} text-xs text-slate-500`} data-etiket={t('MailGenelPage:col_size_time')}><span className="text-right lg:text-left">{formatBoyut(m.message_size, t)}<span className="block">{new Date(m.arrival_time * 1000).toLocaleString(locale)}</span></span></td>
                   <td className={T.hucreAksiyon}>
                     {m.queue_name === 'hold' ?
                       <button onClick={() => queueAction('release', m.queue_id)} className="text-xs text-emerald-600 px-2">{t('MailGenelPage:release')}</button> :
                       <button onClick={() => queueAction('hold', m.queue_id)} className="text-xs text-amber-600 px-2">{t('MailGenelPage:hold')}</button>}
                     <button onClick={() => queueAction('requeue', m.queue_id)} className="text-xs text-brand-600 px-2">{t('MailGenelPage:requeue')}</button>
                     <button onClick={() => queueAction('delete', m.queue_id)} className="text-xs text-red-600 px-2">{t('MailGenelPage:delete')}</button>
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
