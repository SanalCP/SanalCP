// Sunucu geneli veritabanı bakışı. Boyutlar information_schema'dan gelir —
// hangi veritabanının diski yediğini görmek için buradaki tek yer. Satır
// aksiyonları (phpMyAdmin/parola/sil) domain-özel veritabanı sayfasıyla aynı
// /databases/:id uçlarını kullanır.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import GenelListe, { type Kolon } from '@/components/GenelListe'
import ConfirmDialog from '@/components/ConfirmDialog'
import DBParolaSifirlaModal from '@/components/DBParolaSifirlaModal'

type Satir = {
  id: number
  domain_id: number
  alan_adi: string
  db_adi: string
  db_user: string
  db_host: string
  db_parola: string
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
  const [paroliGoster, setParolaGoster] = useState<Record<number, boolean>>({})
  const [kopya, setKopya] = useState<number | null>(null)
  const [silinecek, setSilinecek] = useState<Satir | null>(null)
  const [pwResetFor, setPwResetFor] = useState<Satir | null>(null)
  const [yenile, setYenile] = useState(0)

  async function pmaAc(s: Satir) {
    try {
      const { data } = await api.post<{ signon_url: string }>(`/databases/${s.id}/pma-token`)
      window.open(data.signon_url, '_blank', 'noopener')
    } catch (e) {
      alert(apiHata(e, t('VeritabanlariGenelPage:pma_token_failed')))
    }
  }

  async function sil() {
    if (!silinecek) return
    try { await api.delete(`/databases/${silinecek.id}`); setSilinecek(null); setYenile(v => v + 1) }
    catch (e) { alert(apiHata(e, t('VeritabanlariGenelPage:delete_failed'))) }
  }

  function kopyala(s: Satir) {
    navigator.clipboard.writeText(s.db_parola)
    setKopya(s.id)
    setTimeout(() => setKopya(null), 1500)
  }

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
      baslik: t('VeritabanlariGenelPage:col_password'),
      dar: true,
      hucre: (s) => (
        <div className="flex items-center gap-1">
          <button
            onClick={() => setParolaGoster({ ...paroliGoster, [s.id]: !paroliGoster[s.id] })}
            className="font-mono text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 rounded"
            title={paroliGoster[s.id] ? t('VeritabanlariGenelPage:password_hide') : t('VeritabanlariGenelPage:password_show')}
          >
            {paroliGoster[s.id] ? s.db_parola : '••••••••'}
          </button>
          {paroliGoster[s.id] && (
            <button onClick={() => kopyala(s)} className="text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-brand-100 dark:hover:bg-brand-900/30 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 rounded" title={t('VeritabanlariGenelPage:copy')}>
              {kopya === s.id ? '✓' : '⧉'}
            </button>
          )}
        </div>
      ),
    },
    {
      baslik: t('VeritabanlariGenelPage:col_size'),
      dar: true,
      sinif: 'text-right',
      hucre: (s) => <span className={s.boyut_kb > 1024 * 1024 ? 'text-amber-600 dark:text-amber-400' : ''}>{boyutYaz(s.boyut_kb, t)}</span>,
    },
    { baslik: t('VeritabanlariGenelPage:col_created'), dar: true, hucre: (s) => <span className="text-xs text-slate-500">{s.olusturma || t('VeritabanlariGenelPage:size_empty')}</span> },
    {
      baslik: t('VeritabanlariGenelPage:col_actions'),
      sinif: 'text-right',
      hucre: (s) => (
        <div className="flex flex-wrap items-center gap-1 lg:justify-end">
          <button onClick={() => pmaAc(s)} className="text-sm text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-900/20 px-2 py-1 rounded" title={t('VeritabanlariGenelPage:pma_open_title')}>{t('VeritabanlariGenelPage:pma_button')}</button>
          <button onClick={() => setPwResetFor(s)} className="text-sm text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 px-2 py-1 rounded">{t('VeritabanlariGenelPage:reset_password')}</button>
          <button onClick={() => setSilinecek(s)} className="text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 px-2 py-1 rounded">{t('common:delete')}</button>
        </div>
      ),
    },
  ]

  return (
    <>
      <GenelListe<Satir>
        baslik={t('VeritabanlariGenelPage:title')}
        aciklama={t('VeritabanlariGenelPage:subtitle')}
        uc="/genel/veritabanlari"
        kolonlar={kolonlar}
        araAlan={(s) => `${s.db_adi} ${s.db_user} ${s.alan_adi}`}
        siralaAlan={(s) => s.db_adi}
        satirAnahtar={(s) => s.id}
        bosMesaj={t('VeritabanlariGenelPage:empty')}
        yenilemeTetik={yenile}
        ozet={(l) => {
          const toplam = l.reduce((t, s) => t + s.boyut_kb, 0)
          return [
            { etiket: t('VeritabanlariGenelPage:summary_count'), deger: l.length },
            { etiket: t('VeritabanlariGenelPage:summary_total_size'), deger: boyutYaz(toplam, t) },
            { etiket: t('VeritabanlariGenelPage:summary_unique_users'), deger: new Set(l.map((s) => s.db_user)).size },
          ]
        }}
      />

      {pwResetFor && (
        <DBParolaSifirlaModal
          db={{ id: pwResetFor.id, db_adi: pwResetFor.db_adi, db_kullanici: pwResetFor.db_user }}
          onKapat={() => setPwResetFor(null)}
          onTamam={() => { setPwResetFor(null); setYenile(v => v + 1) }}
        />
      )}

      <ConfirmDialog
        acik={!!silinecek}
        baslik={t('VeritabanlariGenelPage:delete_dialog_title')}
        mesaj={t('VeritabanlariGenelPage:delete_dialog_msg', { ad: silinecek?.db_adi })}
        tehlikeli
        onayMetni={t('VeritabanlariGenelPage:delete_confirm')}
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </>
  )
}
