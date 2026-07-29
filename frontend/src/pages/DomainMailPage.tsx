import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Domain = { id: number; alan_adi: string }
type Mailbox = { id: number; local_part: string; email: string; status: string; created_at: string }
type Durum = { etkin: boolean; dkim_selector?: string }
type Alias = { id: number; source: string; destination: string; catch_all: boolean; status: string; created_at: string }
type SpamSettings = { enabled: boolean; greylist_score: number; add_header_score: number; reject_score: number }
type SpamResponse = { settings: SpamSettings; rspamd: boolean }
type Autoresponder = { mailbox_id: number; email: string; enabled: boolean; subject: string; body: string; interval_days: number }
type MailFilter = {
  id: number; mailbox_id: number; email: string; name: string; match_field: 'from'|'to'|'subject'
  match_value: string; action_type: 'move'|'redirect'|'discard'; action_value: string; priority: number; enabled: boolean
}
type SendLimits = { mailbox_id: number; email: string; hour_limit: number; day_limit: number; sent_hour: number; sent_day: number; spam_suspended_at?: string }

export default function DomainMailPage() {
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [durum, setDurum] = useState<Durum | null>(null)
  const [liste, setListe] = useState<Mailbox[]>([])
  const [aliasListe, setAliasListe] = useState<Alias[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [localPart, setLocalPart] = useState('')
  const [parola, setParola] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [yeniPw, setYeniPw] = useState<{ email: string; parola: string } | null>(null)
  const [aliasKaynak, setAliasKaynak] = useState('')
  const [aliasCatchAll, setAliasCatchAll] = useState(false)
  const [aliasHedef, setAliasHedef] = useState('')
  const [aliasIsleniyor, setAliasIsleniyor] = useState(false)
  const [spam, setSpam] = useState<SpamSettings>({ enabled: true, greylist_score: 4, add_header_score: 6, reject_score: 15 })
  const [rspamd, setRspamd] = useState(false)
  const [spamIsleniyor, setSpamIsleniyor] = useState(false)
  const [auto, setAuto] = useState<Autoresponder>({ mailbox_id: 0, email: '', enabled: true, subject: 'Otomatik yanıt', body: '', interval_days: 7 })
  const [autoIsleniyor, setAutoIsleniyor] = useState(false)
  const [filtreler, setFiltreler] = useState<MailFilter[]>([])
  const [filtre, setFiltre] = useState<Omit<MailFilter,'id'|'email'>>({
    mailbox_id: 0, name: '', match_field: 'subject', match_value: '', action_type: 'move', action_value: 'Junk', priority: 100, enabled: true,
  })
  const [filtreIsleniyor, setFiltreIsleniyor] = useState(false)
  const [limit, setLimit] = useState<SendLimits>({ mailbox_id: 0, email: '', hour_limit: 100, day_limit: 500, sent_hour: 0, sent_day: 0 })
  const [limitIsleniyor, setLimitIsleniyor] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true)
    Promise.all([
      api.get<Durum>(`/domains/${id}/mail/durum`),
      api.get<Mailbox[]>(`/domains/${id}/mail`),
      api.get<Alias[]>(`/domains/${id}/mail/aliases`),
      api.get<SpamResponse>(`/domains/${id}/mail/spam`).catch(() => ({ data: { settings: spam, rspamd: false } as SpamResponse })),
      api.get<MailFilter[]>(`/domains/${id}/mail/filters`).catch(() => ({ data: [] as MailFilter[] })),
    ])
      .then(([d, m, a, s, f]) => {
        setDurum(d.data); setListe(m.data || []); setAliasListe(a.data || [])
        setSpam(s.data.settings); setRspamd(s.data.rspamd)
        setFiltreler(f.data || [])
        if (!filtre.mailbox_id && m.data?.length) setFiltre(x => ({ ...x, mailbox_id: m.data[0].id }))
        if (!auto.mailbox_id && m.data?.length) autoYukle(m.data[0].id)
        if (!limit.mailbox_id && m.data?.length) limitYukle(m.data[0].id)
      })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(() => {
    if (!id) return
    api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(e => setHata(apiHata(e)))
    yukle()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  async function etkinlestir() {
    setIsleniyor(true); setHata(null)
    try {
      await api.post(`/domains/${id}/mail/etkinlestir`)
      setOk('E-posta bu domain için etkinleştirildi. MX/SPF/DKIM/DMARC kayıtları DNS bölümüne eklendi.')
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Etkinleştirilemedi'))
    } finally {
      setIsleniyor(false)
    }
  }

  async function ekle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setYeniPw(null); setIsleniyor(true)
    try {
      const { data } = await api.post(`/domains/${id}/mail`, { local_part: localPart, parola })
      setYeniPw({ email: data.email, parola: data.parola })
      setLocalPart(''); setParola('')
      yukle()
    } catch (e2) {
      setHata(apiHata(e2, 'Kutu oluşturulamadı'))
    } finally {
      setIsleniyor(false)
    }
  }

  async function sil(k: Mailbox) {
    if (!confirm(`"${k.email}" kutusunu silmek istediğinize emin misiniz? (Maildir diskte kalır, yalnızca hesap kaldırılır.)`)) return
    setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/mail/${k.id}`)
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Silinemedi'))
    }
  }

  async function parolaSifirla(k: Mailbox) {
    setHata(null); setOk(null); setYeniPw(null)
    try {
      const { data } = await api.put(`/domains/${id}/mail/${k.id}/parola`, {})
      setYeniPw({ email: k.email, parola: data.parola })
    } catch (e) {
      setHata(apiHata(e, 'Parola sıfırlanamadı'))
    }
  }

  async function kutuDurumDegistir(k: Mailbox) {
    setHata(null); setOk(null)
    try {
      await api.post(`/domains/${id}/mail/${k.id}/durum`, { status: k.status === 'active' ? 'suspended' : 'active' })
      yukle()
    } catch (e) { setHata(apiHata(e, 'Kutu durumu değiştirilemedi')) }
  }

  async function aliasEkle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setAliasIsleniyor(true)
    try {
      await api.post(`/domains/${id}/mail/aliases`, {
        local_part: aliasCatchAll ? '' : aliasKaynak,
        destination: aliasHedef,
      })
      setAliasKaynak(''); setAliasHedef(''); setAliasCatchAll(false)
      setOk('Yönlendirme eklendi.')
      yukle()
    } catch (e2) {
      setHata(apiHata(e2, 'Yönlendirme eklenemedi'))
    } finally {
      setAliasIsleniyor(false)
    }
  }

  async function aliasSil(a: Alias) {
    if (!confirm(`"${a.source}" yönlendirmesini silmek istediğinize emin misiniz?`)) return
    setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/mail/aliases/${a.id}`)
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Silinemedi'))
    }
  }

  async function aliasDurumDegistir(a: Alias) {
    setHata(null); setOk(null)
    try {
      await api.post(`/domains/${id}/mail/aliases/${a.id}/durum`, { status: a.status === 'active' ? 'suspended' : 'active' })
      yukle()
    } catch (e) {
      setHata(apiHata(e, 'Durum değiştirilemedi'))
    }
  }

  async function spamKaydet(e: React.FormEvent) {
    e.preventDefault()
    setSpamIsleniyor(true); setHata(null); setOk(null)
    try {
      const { data } = await api.put<{ settings: SpamSettings }>(`/domains/${id}/mail/spam`, spam)
      setSpam(data.settings)
      setRspamd(true)
      setOk('Rspamd spam politikası doğrulandı ve etkinleştirildi.')
    } catch (e2) {
      setHata(apiHata(e2, 'Spam ayarları uygulanamadı'))
    } finally {
      setSpamIsleniyor(false)
    }
  }

  async function autoYukle(mid: number) {
    if (!mid) return
    try {
      const { data } = await api.get<Autoresponder>(`/domains/${id}/mail/${mid}/autoresponder`)
      setAuto(data)
    } catch (e) {
      setHata(apiHata(e, 'Otomatik yanıtlayıcı okunamadı'))
    }
  }

  async function autoKaydet(e: React.FormEvent) {
    e.preventDefault(); setAutoIsleniyor(true); setHata(null); setOk(null)
    try {
      await api.put(`/domains/${id}/mail/${auto.mailbox_id}/autoresponder`, auto)
      setOk('Otomatik yanıtlayıcı ve Sieve betiği etkinleştirildi.')
    } catch (e2) {
      setHata(apiHata(e2, 'Otomatik yanıtlayıcı kaydedilemedi'))
    } finally {
      setAutoIsleniyor(false)
    }
  }

  async function autoSil() {
    setAutoIsleniyor(true); setHata(null)
    try {
      await api.delete(`/domains/${id}/mail/${auto.mailbox_id}/autoresponder`)
      setAuto(a => ({ ...a, enabled: false, body: '' }))
      setOk('Otomatik yanıtlayıcı kaldırıldı.')
    } catch (e) { setHata(apiHata(e)) } finally { setAutoIsleniyor(false) }
  }

  async function filtreEkle(e: React.FormEvent) {
    e.preventDefault(); setFiltreIsleniyor(true); setHata(null); setOk(null)
    try {
      await api.post(`/domains/${id}/mail/filters`, filtre)
      setFiltre(f => ({ ...f, name: '', match_value: '' }))
      setOk('Posta filtresi derlendi ve etkinleştirildi.')
      yukle()
    } catch (e2) { setHata(apiHata(e2, 'Filtre eklenemedi')) } finally { setFiltreIsleniyor(false) }
  }

  async function filtreSil(f: MailFilter) {
    if (!confirm(`"${f.name}" filtresi silinsin mi?`)) return
    try {
      await api.delete(`/domains/${id}/mail/filters/${f.id}`)
      yukle()
    } catch (e) { setHata(apiHata(e)) }
  }

  async function limitYukle(mid: number) {
    if (!mid) return
    try {
      const { data } = await api.get<SendLimits>(`/domains/${id}/mail/${mid}/send-limits`)
      setLimit(data)
    } catch (e) { setHata(apiHata(e, 'Gönderim limitleri okunamadı')) }
  }

  async function limitKaydet(e: React.FormEvent) {
    e.preventDefault(); setLimitIsleniyor(true); setHata(null); setOk(null)
    try {
      await api.put(`/domains/${id}/mail/${limit.mailbox_id}/send-limits`, limit)
      setOk('Gönderim limitleri kaydedildi.')
      limitYukle(limit.mailbox_id)
    } catch (e2) { setHata(apiHata(e2, 'Limitler kaydedilemedi')) } finally { setLimitIsleniyor(false) }
  }

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { etiket: 'Anasayfa', href: '/' },
          { etiket: 'Domainler', href: '/domainler' },
          { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
          { etiket: 'E-posta' },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">E-posta Hesapları</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          Postfix/Dovecot tabanlı posta kutuları. SMTP (587, STARTTLS) uygulamalarınızda (PHPMailer vb.) kimlik doğrulamalı gönderim için kullanılabilir.
        </p>

        {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
        {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

        {yeniPw && (
          <div className="mb-3 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg p-4">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium mb-1">✓ {yeniPw.email} parolası</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300 mb-2">Bunu güvenli bir yere kaydedin, sonra tekrar gösterilmez:</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{yeniPw.parola}</code>
              <button onClick={() => navigator.clipboard.writeText(yeniPw.parola)} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">Kopyala</button>
            </div>
          </div>
        )}

        {yuk ? (
          <div className="text-sm text-slate-400">Yükleniyor…</div>
        ) : !durum?.etkin ? (
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6 text-center">
            <div className="text-3xl mb-2">📧</div>
            <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">Bu domain için e-posta henüz etkin değil.</p>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">Etkinleştirince MX/SPF/DKIM/DMARC kayıtları otomatik olarak DNS'e eklenir.</p>
            <button onClick={etkinlestir} disabled={isleniyor}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
              {isleniyor ? 'Etkinleştiriliyor…' : 'E-postayı Etkinleştir'}
            </button>
          </div>
        ) : (
          <>
            <form onSubmit={ekle} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Yeni kutu ekle</h3>
              <div className="flex items-center gap-2">
                <input value={localPart} onChange={e => setLocalPart(e.target.value)} required placeholder="bilgi"
                  className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.alan_adi}</span>
                <input value={parola} onChange={e => setParola(e.target.value)} type="password" placeholder="parola (boşsa üretilir)"
                  className="w-56 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <button disabled={isleniyor || !localPart} className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {isleniyor ? 'Ekleniyor…' : 'Ekle'}
                </button>
              </div>
            </form>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Kutular</h3>
              {liste.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-sm text-slate-500 dark:text-slate-400">Henüz kutu yok.</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {liste.map(k => (
                    <li key={k.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">{k.email}</span>
                        {k.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">askıda</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button onClick={() => parolaSifirla(k)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">Parola sıfırla</button>
                        <button onClick={() => kutuDurumDegistir(k)} className="text-xs text-amber-600 dark:text-amber-400 hover:underline">{k.status === 'active' ? 'Askıya al' : 'Etkinleştir'}</button>
                        <button onClick={() => sil(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Sil</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">Yönlendirmeler (Forwarder) &amp; Catch-All</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
                Gelen postayı bir kutu oluşturmadan başka adres(ler)e yönlendirir. "Bu domaine gelen tüm postayı yönlendir" seçilirse, tanımlı kutusu olmayan her adrese gelen mail bu hedefe gider (catch-all).
              </p>
              <form onSubmit={aliasEkle} className="mb-4 space-y-2">
                <div className="flex items-center gap-2">
                  {aliasCatchAll ? (
                    <span className="flex-1 px-3 py-2 border border-dashed border-slate-300 dark:border-slate-600 rounded-lg text-sm text-slate-500 dark:text-slate-400 font-mono">*@{domain?.alan_adi}</span>
                  ) : (
                    <>
                      <input value={aliasKaynak} onChange={e => setAliasKaynak(e.target.value)} required={!aliasCatchAll} placeholder="destek"
                        className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                      <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.alan_adi}</span>
                    </>
                  )}
                </div>
                <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
                  <input type="checkbox" checked={aliasCatchAll} onChange={e => setAliasCatchAll(e.target.checked)} />
                  Bu domaine gelen tüm postayı yönlendir (catch-all)
                </label>
                <div className="flex items-center gap-2">
                  <input value={aliasHedef} onChange={e => setAliasHedef(e.target.value)} required placeholder="hedef1@ornek.com, hedef2@ornek.com"
                    className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                  <button disabled={aliasIsleniyor || !aliasHedef || (!aliasCatchAll && !aliasKaynak)}
                    className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                    {aliasIsleniyor ? 'Ekleniyor…' : 'Ekle'}
                  </button>
                </div>
              </form>

              {aliasListe.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-sm text-slate-500 dark:text-slate-400">Henüz yönlendirme yok.</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {aliasListe.map(a => (
                    <li key={a.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">
                          {a.catch_all ? `*@${domain?.alan_adi}` : a.source}
                        </span>
                        <span className="mx-1.5 text-slate-400">→</span>
                        <span className="text-sm font-mono text-slate-600 dark:text-slate-400">{a.destination}</span>
                        {a.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">askıda</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button onClick={() => aliasDurumDegistir(a)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">
                          {a.status === 'active' ? 'Askıya al' : 'Etkinleştir'}
                        </button>
                        <button onClick={() => aliasSil(a)} className="text-xs text-red-600 dark:text-red-400 hover:underline">Sil</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <form onSubmit={spamKaydet} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Rspamd Spam Koruması</h3>
                  <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                    SPF, DKIM, içerik, itibar ve öğrenilmiş Bayes puanına göre gelen postaya uygulanacak domain politikası.
                  </p>
                </div>
                <span className={`shrink-0 text-[10px] uppercase tracking-wider font-semibold px-2 py-1 rounded ${
                  rspamd ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' :
                    'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
                }`}>{rspamd ? '● Rspamd aktif' : 'Rspamd kurulu değil'}</span>
              </div>
              <label className="mt-4 flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={spam.enabled}
                  onChange={e => setSpam(s => ({ ...s, enabled: e.target.checked }))}/>
                Bu domain için spam filtresini etkinleştir
              </label>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
                {([
                  ['greylist_score', 'Greylist', 'Şüpheli göndericiyi geçici bekletir'],
                  ['add_header_score', 'Spam başlığı', 'İletiyi teslim eder, spam olarak işaretler'],
                  ['reject_score', 'Kesin reddet', 'SMTP aşamasında iletiyi kabul etmez'],
                ] as const).map(([key, label, help]) => (
                  <label key={key} className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{label} puanı</span>
                    <input type="number" min={0} max={50} step={0.5} value={spam[key]}
                      disabled={!spam.enabled}
                      onChange={e => setSpam(s => ({ ...s, [key]: Number(e.target.value) }))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono disabled:opacity-50"/>
                    <span className="block mt-1 text-[10px] leading-snug text-slate-500">{help}</span>
                  </label>
                ))}
              </div>
              <div className="mt-4 flex justify-end">
                <button disabled={spamIsleniyor || !rspamd}
                  className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {spamIsleniyor ? 'Doğrulanıyor…' : 'Spam Politikasını Kaydet'}
                </button>
              </div>
            </form>

            {liste.length > 0 && (
              <form onSubmit={autoKaydet} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Otomatik Yanıtlayıcı</h3>
                <p className="text-xs text-slate-500 mt-1">Dovecot Sieve vacation ile aynı gönderene belirlenen aralıkta yalnız bir kez yanıt verir; posta döngülerini engeller.</p>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
                  <label className="block sm:col-span-2">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">Posta kutusu</span>
                    <select value={auto.mailbox_id} onChange={e => autoYukle(Number(e.target.value))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono">
                      {liste.map(m => <option key={m.id} value={m.id}>{m.email}</option>)}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">Aynı gönderene yanıt aralığı</span>
                    <select value={auto.interval_days} onChange={e => setAuto(a => ({ ...a, interval_days: Number(e.target.value) }))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm">
                      {[1,2,3,7,14,30].map(n => <option key={n} value={n}>{n} gün</option>)}
                    </select>
                  </label>
                </div>
                <input value={auto.subject} onChange={e => setAuto(a => ({ ...a, subject: e.target.value }))} required maxLength={255}
                  placeholder="Konu" className="mt-3 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm"/>
                <textarea value={auto.body} onChange={e => setAuto(a => ({ ...a, body: e.target.value }))} required maxLength={10000} rows={4}
                  placeholder="Ofis dışında olduğunuzu bildiren mesaj…" className="mt-3 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm"/>
                <div className="mt-3 flex items-center justify-between gap-2">
                  <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={auto.enabled} onChange={e => setAuto(a => ({ ...a, enabled: e.target.checked }))}/> Etkin</label>
                  <div className="flex gap-2">
                    <button type="button" onClick={autoSil} disabled={autoIsleniyor} className="px-3 py-1.5 text-xs text-red-600 border border-red-300 rounded">Kaldır</button>
                    <button disabled={autoIsleniyor || !auto.body || !auto.mailbox_id} className="px-3 py-1.5 text-xs bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded disabled:opacity-50">Kaydet</button>
                  </div>
                </div>
              </form>
            )}

            {liste.length > 0 && (
              <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Posta Filtreleri</h3>
                <p className="text-xs text-slate-500 mt-1">Gönderen, alıcı veya konu eşleşince klasöre taşı, başka adrese yönlendir ya da sil.</p>
                <form onSubmit={filtreEkle} className="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-4">
                  <select value={filtre.mailbox_id} onChange={e => setFiltre(f => ({ ...f, mailbox_id: Number(e.target.value) }))}
                    className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs font-mono">
                    {liste.map(m => <option key={m.id} value={m.id}>{m.email}</option>)}
                  </select>
                  <input value={filtre.name} onChange={e => setFiltre(f => ({ ...f, name: e.target.value }))} required placeholder="Filtre adı"
                    className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs"/>
                  <div className="flex gap-2">
                    <select value={filtre.match_field} onChange={e => setFiltre(f => ({ ...f, match_field: e.target.value as MailFilter['match_field'] }))}
                      className="w-28 px-2 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs">
                      <option value="from">Gönderen</option><option value="to">Alıcı</option><option value="subject">Konu</option>
                    </select>
                    <input value={filtre.match_value} onChange={e => setFiltre(f => ({ ...f, match_value: e.target.value }))} required placeholder="şunu içerirse…"
                      className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs"/>
                  </div>
                  <div className="flex gap-2">
                    <select value={filtre.action_type} onChange={e => setFiltre(f => ({ ...f, action_type: e.target.value as MailFilter['action_type'], action_value: e.target.value === 'move' ? 'Junk' : '' }))}
                      className="w-28 px-2 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs">
                      <option value="move">Klasöre taşı</option><option value="redirect">Yönlendir</option><option value="discard">Sil</option>
                    </select>
                    {filtre.action_type !== 'discard' && <input value={filtre.action_value} onChange={e => setFiltre(f => ({ ...f, action_value: e.target.value }))} required
                      placeholder={filtre.action_type === 'move' ? 'Junk' : 'hedef@ornek.com'}
                      className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs"/>}
                  </div>
                  <button disabled={filtreIsleniyor || !filtre.mailbox_id} className="sm:col-span-2 px-3 py-2 bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded text-xs disabled:opacity-50">Filtre ekle ve derle</button>
                </form>
                {filtreler.length > 0 && <ul className="mt-4 divide-y divide-slate-100 dark:divide-slate-700">
                  {filtreler.map(f => <li key={f.id} className="py-2 flex items-center justify-between gap-3 text-xs">
                    <div><span className="font-semibold">{f.name}</span> · <span className="font-mono">{f.email}</span><div className="text-slate-500">{f.match_field}: “{f.match_value}” → {f.action_type}{f.action_value ? `: ${f.action_value}` : ''}</div></div>
                    <button onClick={() => filtreSil(f)} className="text-red-600">Sil</button>
                  </li>)}
                </ul>}
              </div>
            )}

            {liste.length > 0 && (
              <form onSubmit={limitKaydet} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm mt-5">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Gönderim Limiti ve Spam Askıya Alma</h3>
                <p className="text-xs text-slate-500 mt-1">SMTP AUTH ile gönderilen alıcı sayısı izlenir. Eşik aşılırsa hesap otomatik askıya alınır; 0 sınırsızdır.</p>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">Posta kutusu</span>
                    <select value={limit.mailbox_id} onChange={e => limitYukle(Number(e.target.value))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs font-mono">
                      {liste.map(m => <option key={m.id} value={m.id}>{m.email}</option>)}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">Saatlik alıcı limiti</span>
                    <input type="number" min={0} max={100000} value={limit.hour_limit}
                      onChange={e => setLimit(x => ({ ...x, hour_limit: Number(e.target.value) }))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono"/>
                    <span className="text-[10px] text-slate-500">Kullanım: {limit.sent_hour}</span>
                  </label>
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">Günlük alıcı limiti</span>
                    <input type="number" min={0} max={100000} value={limit.day_limit}
                      onChange={e => setLimit(x => ({ ...x, day_limit: Number(e.target.value) }))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono"/>
                    <span className="text-[10px] text-slate-500">Kullanım: {limit.sent_day}</span>
                  </label>
                </div>
                {limit.spam_suspended_at && <div className="mt-3 text-xs text-red-700 bg-red-50 dark:bg-red-900/20 dark:text-red-300 p-2 rounded">
                  Bu hesap {limit.spam_suspended_at} tarihinde limit aşımı nedeniyle otomatik askıya alındı. Kutular listesinden yeniden etkinleştirebilirsiniz.
                </div>}
                <div className="mt-3 flex justify-end">
                  <button disabled={limitIsleniyor || !limit.mailbox_id}
                    className="px-3 py-1.5 text-xs bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded disabled:opacity-50">Limitleri kaydet</button>
                </div>
              </form>
            )}
          </>
        )}

        <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">← Aboneliğe dön</Link></div>
      </div>
    </div>
  )
}
