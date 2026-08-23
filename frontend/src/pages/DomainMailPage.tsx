import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ParolaGirdisi from '@/components/ParolaGirdisi'

type Domain = { id: number; alan_adi: string; ssl?: boolean }
type Mailbox = { id: number; local_part: string; email: string; status: string; created_at: string }
type Durum = { etkin: boolean; dkim_selector?: string; altyapi_eksik?: string[] }
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
  const { t } = useTranslation(['DomainMailPage', 'common'])
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
  const [siliniyor, setSiliniyor] = useState(false)
  const [yeniPw, setYeniPw] = useState<{ email: string; parola: string } | null>(null)
  const [aliasKaynak, setAliasKaynak] = useState('')
  const [aliasCatchAll, setAliasCatchAll] = useState(false)
  const [aliasHedef, setAliasHedef] = useState('')
  const [aliasIsleniyor, setAliasIsleniyor] = useState(false)
  const [spam, setSpam] = useState<SpamSettings>({ enabled: true, greylist_score: 4, add_header_score: 6, reject_score: 15 })
  const [rspamd, setRspamd] = useState(false)
  const [spamIsleniyor, setSpamIsleniyor] = useState(false)
  const [auto, setAuto] = useState<Autoresponder>({ mailbox_id: 0, email: '', enabled: true, subject: t('DomainMailPage:autoresponder.default_subject'), body: '', interval_days: 7 })
  const [autoIsleniyor, setAutoIsleniyor] = useState(false)
  const [filtreler, setFiltreler] = useState<MailFilter[]>([])
  const [filtre, setFiltre] = useState<Omit<MailFilter,'id'|'email'>>({
    mailbox_id: 0, name: '', match_field: 'subject', match_value: '', action_type: 'move', action_value: 'Junk', priority: 100, enabled: true,
  })
  const [filtreIsleniyor, setFiltreIsleniyor] = useState(false)
  const [limit, setLimit] = useState<SendLimits>({ mailbox_id: 0, email: '', hour_limit: 100, day_limit: 500, sent_hour: 0, sent_day: 0 })
  const [limitIsleniyor, setLimitIsleniyor] = useState(false)

  const altyapiEksik = durum?.altyapi_eksik || []

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
      setOk(t('DomainMailPage:enable.success'))
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainMailPage:enable.failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  // Hizmeti kapat. Backend (mailH.Devredisi -> MailKaldir) posta kutularını
  // SİLMEZ, yalnızca mail_domains.durum='suspended' yapar — yeniden
  // etkinleştirmek kutuları ve yönlendiricileri olduğu gibi geri getirir.
  // Onay metni bunu açıkça söylemeli: "sil" demek kullanıcıyı yanıltırdı.
  async function devredisiBirak() {
    if (!confirm(t('DomainMailPage:disable.confirm', { domain: domain?.alan_adi }))) return
    setIsleniyor(true); setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/mail/etkinlestir`)
      setOk(t('DomainMailPage:disable.success'))
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainMailPage:disable.failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  // Hizmeti tamamen kaldır — GERİ DÖNÜŞÜ YOK (kutular, alias'lar, filtreler ve
  // DİSKTEKİ posta dosyaları silinir). Bu yüzden tek tıklık confirm yetmez:
  // kullanıcıya alan adını YAZDIRIYORUZ. Yanlışlıkla "Tamam"a basmak, yıllarca
  // birikmiş postayı silmeye yetmemeli.
  async function hizmetiSil() {
    const yazilan = window.prompt(t('DomainMailPage:purge.confirm_prompt', { domain: domain?.alan_adi }))
    if (yazilan === null) return // vazgeçildi
    if (yazilan.trim().toLowerCase() !== (domain?.alan_adi || '').toLowerCase()) {
      setHata(t('DomainMailPage:purge.confirm_mismatch'))
      return
    }
    setSiliniyor(true); setHata(null); setOk(null)
    try {
      const { data } = await api.delete(`/domains/${id}/mail/hizmet`)
      // Sunucu DB'yi temizleyip diskte takılırsa 200 + uyari döner: hizmet
      // gerçekten kaldırıldı ama dosyalar kalmış olabilir — bunu gizleme.
      if (data?.uyari) setHata(t('DomainMailPage:purge.partial', { detay: data.uyari }))
      else setOk(t('DomainMailPage:purge.success'))
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainMailPage:purge.failed')))
    } finally {
      setSiliniyor(false)
    }
  }

  async function ekle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setYeniPw(null); setIsleniyor(true)
    try {
      const { data } = await api.post(`/domains/${id}/mail`, { local_part: localPart, parola })
      await parolaGoster(data.id, data.email, data.parola_reveal_token)
      setLocalPart(''); setParola('')
      yukle()
    } catch (e2) {
      setHata(apiHata(e2, t('DomainMailPage:mailbox.add_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  // Sunucu artık parolayı create/reset yanıtında düz metin döndürmüyor — tek
  // kullanımlık gösterim token'ı ile ayrı bir istekte bir kez gösterilir
  // (bkz. internal/mail/reveal.go).
  async function parolaGoster(mailboxId: number, email: string, token: string) {
    const { data } = await api.get(`/domains/${id}/mail/${mailboxId}/parola-reveal/${token}`)
    setYeniPw({ email, parola: data.parola })
  }

  async function sil(k: Mailbox) {
    if (!confirm(t('DomainMailPage:mailbox.delete_confirm', { email: k.email }))) return
    setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/mail/${k.id}`)
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainMailPage:mailbox.delete_failed')))
    }
  }

  async function parolaSifirla(k: Mailbox) {
    setHata(null); setOk(null); setYeniPw(null)
    try {
      const { data } = await api.put(`/domains/${id}/mail/${k.id}/parola`, {})
      await parolaGoster(k.id, k.email, data.parola_reveal_token)
    } catch (e) {
      setHata(apiHata(e, t('DomainMailPage:mailbox.reset_failed')))
    }
  }

  async function kutuDurumDegistir(k: Mailbox) {
    setHata(null); setOk(null)
    try {
      await api.post(`/domains/${id}/mail/${k.id}/durum`, { status: k.status === 'active' ? 'suspended' : 'active' })
      yukle()
    } catch (e) { setHata(apiHata(e, t('DomainMailPage:mailbox.status_failed'))) }
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
      setOk(t('DomainMailPage:forwarders.add_success'))
      yukle()
    } catch (e2) {
      setHata(apiHata(e2, t('DomainMailPage:forwarders.add_failed')))
    } finally {
      setAliasIsleniyor(false)
    }
  }

  async function aliasSil(a: Alias) {
    if (!confirm(t('DomainMailPage:forwarders.delete_confirm', { source: a.source }))) return
    setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/mail/aliases/${a.id}`)
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainMailPage:forwarders.delete_failed')))
    }
  }

  async function aliasDurumDegistir(a: Alias) {
    setHata(null); setOk(null)
    try {
      await api.post(`/domains/${id}/mail/aliases/${a.id}/durum`, { status: a.status === 'active' ? 'suspended' : 'active' })
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainMailPage:forwarders.status_failed')))
    }
  }

  async function spamKaydet(e: React.FormEvent) {
    e.preventDefault()
    setSpamIsleniyor(true); setHata(null); setOk(null)
    try {
      const { data } = await api.put<{ settings: SpamSettings }>(`/domains/${id}/mail/spam`, spam)
      setSpam(data.settings)
      setRspamd(true)
      setOk(t('DomainMailPage:spam.save_success'))
    } catch (e2) {
      setHata(apiHata(e2, t('DomainMailPage:spam.save_failed')))
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
      setHata(apiHata(e, t('DomainMailPage:autoresponder.load_failed')))
    }
  }

  async function autoKaydet(e: React.FormEvent) {
    e.preventDefault(); setAutoIsleniyor(true); setHata(null); setOk(null)
    try {
      await api.put(`/domains/${id}/mail/${auto.mailbox_id}/autoresponder`, auto)
      setOk(t('DomainMailPage:autoresponder.save_success'))
    } catch (e2) {
      setHata(apiHata(e2, t('DomainMailPage:autoresponder.save_failed')))
    } finally {
      setAutoIsleniyor(false)
    }
  }

  async function autoSil() {
    setAutoIsleniyor(true); setHata(null)
    try {
      await api.delete(`/domains/${id}/mail/${auto.mailbox_id}/autoresponder`)
      setAuto(a => ({ ...a, enabled: false, body: '' }))
      setOk(t('DomainMailPage:autoresponder.remove_success'))
    } catch (e) { setHata(apiHata(e)) } finally { setAutoIsleniyor(false) }
  }

  async function filtreEkle(e: React.FormEvent) {
    e.preventDefault(); setFiltreIsleniyor(true); setHata(null); setOk(null)
    try {
      await api.post(`/domains/${id}/mail/filters`, filtre)
      setFiltre(f => ({ ...f, name: '', match_value: '' }))
      setOk(t('DomainMailPage:filters.add_success'))
      yukle()
    } catch (e2) { setHata(apiHata(e2, t('DomainMailPage:filters.add_failed'))) } finally { setFiltreIsleniyor(false) }
  }

  async function filtreSil(f: MailFilter) {
    if (!confirm(t('DomainMailPage:filters.delete_confirm', { name: f.name }))) return
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
    } catch (e) { setHata(apiHata(e, t('DomainMailPage:limits.load_failed'))) }
  }

  async function limitKaydet(e: React.FormEvent) {
    e.preventDefault(); setLimitIsleniyor(true); setHata(null); setOk(null)
    try {
      await api.put(`/domains/${id}/mail/${limit.mailbox_id}/send-limits`, limit)
      setOk(t('DomainMailPage:limits.save_success'))
      limitYukle(limit.mailbox_id)
    } catch (e2) { setHata(apiHata(e2, t('DomainMailPage:limits.save_failed'))) } finally { setLimitIsleniyor(false) }
  }

  // Webmail müşterinin KENDİ alan adından servis edilir (vhost'a eklenen
  // /webmail/ bloğu). Panelin origin'ini kullanmak, panele IP ile girilmişse
  // müşteriye çıplak sunucu IP'si göstermek demekti.
  // Domain henüz yüklenmediyse panel origin'i geçici olarak kullanılır.
  const webmailURL = domain
    ? `${domain.ssl === false ? 'http' : 'https'}://${domain.alan_adi}/webmail/`
    : `${window.location.origin}/webmail/`

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { etiket: t('common:home'), href: '/' },
          { etiket: t('DomainMailPage:breadcrumb.domains'), href: '/domainler' },
          { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
          { etiket: t('DomainMailPage:breadcrumb.mail') },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainMailPage:title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          {t('DomainMailPage:subtitle')}
        </p>

        {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
        {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

        {yeniPw && (
          <div className="mb-3 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg p-4">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium mb-1">{t('DomainMailPage:newPassword.title', { email: yeniPw.email })}</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300 mb-2">{t('DomainMailPage:newPassword.hint')}</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{yeniPw.parola}</code>
              <button onClick={() => navigator.clipboard.writeText(yeniPw.parola)} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">{t('common:copy')}</button>
            </div>
          </div>
        )}

        {yuk ? (
          <div className="text-sm text-slate-400">{t('common:loading')}</div>
        ) : !durum?.etkin ? (
          <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-6 text-center">
            <div className="text-3xl mb-2">📧</div>
            <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">{t('DomainMailPage:enable.not_enabled')}</p>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">{t('DomainMailPage:enable.hint')}</p>
            <div className="flex justify-center mb-4">
              <div className="inline-flex items-start gap-2 text-left px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
                <span className="text-amber-500 dark:text-amber-400 text-sm leading-none mt-0.5">⚠</span>
                <span className="text-xs text-amber-800 dark:text-amber-300">{t('DomainMailPage:enable.resource_warning')}</span>
              </div>
            </div>
            {/* Sunucu yöneticisi mail yığınını kapatmış olabilir. Butonu baştan
                kapatıp nedenini söylüyoruz — tıklayıp 503 almaktan iyi. */}
            {altyapiEksik.length > 0 && (
              <div className="mx-auto mb-4 max-w-xl px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-xs text-red-700 dark:text-red-300">
                {t('DomainMailPage:enable.stack_down', { servisler: altyapiEksik.join(', ') })}
              </div>
            )}
            <button onClick={etkinlestir} disabled={isleniyor || altyapiEksik.length > 0}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50 disabled:cursor-not-allowed">
              {isleniyor ? t('DomainMailPage:enable.enabling') : t('DomainMailPage:enable.button')}
            </button>
          </div>
        ) : (
          <>
            {/* Webmail — Roundcube panel vhost'unda /webmail/ altında servis edilir
                (bkz. assets/nginx/_panel.conf). mail.<domain> ADRESİ DEĞİLDİR:
                o kayıt MX hedefidir, web arayüzü sunmaz. */}
            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm flex items-center gap-4 flex-wrap">
              <div className="w-11 h-11 rounded-xl bg-sky-100 dark:bg-sky-900/30 text-sky-700 dark:text-sky-300 flex items-center justify-center text-xl shrink-0">✉️</div>
              <div className="flex-1 min-w-[200px]">
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainMailPage:webmail.title')}</div>
                <div className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{t('DomainMailPage:webmail.desc')}</div>
                <code className="text-[11px] text-slate-500 dark:text-slate-500 font-mono break-all">{webmailURL}</code>
              </div>
              <div className="flex items-center gap-2">
                <a href={webmailURL} target="_blank" rel="noopener noreferrer"
                  className="px-4 py-2 bg-sky-600 hover:bg-sky-700 text-white text-sm font-medium rounded-lg">
                  {t('DomainMailPage:webmail.open')}
                </a>
                <button onClick={() => { navigator.clipboard?.writeText(webmailURL); setOk(t('DomainMailPage:webmail.copied')) }}
                  className="px-3 py-2 border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 text-sm rounded-lg">
                  {t('common:copy')}
                </button>
              </div>
            </div>

            {/* Ayar kartları 2 sütunlu grid'te (dashboard deseni). items-start:
                kartlar komşusunun boyuna uzamasın, kendi içerikleri kadar dursun. */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 items-start">
            <form onSubmit={ekle} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainMailPage:mailbox.add_title')}</h3>
              <div className="flex items-center gap-2 flex-wrap">
                <input value={localPart} onChange={e => setLocalPart(e.target.value)} required placeholder={t('DomainMailPage:mailbox.local_part_placeholder')}
                  className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.alan_adi}</span>
                <ParolaGirdisi value={parola} onChange={setParola} placeholder={t('DomainMailPage:mailbox.password_placeholder')}
                  className="w-40 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                <button disabled={isleniyor || !localPart} className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                  {isleniyor ? t('DomainMailPage:mailbox.adding') : t('common:add')}
                </button>
              </div>
            </form>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainMailPage:mailbox.list_title')}</h3>
              {liste.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('DomainMailPage:mailbox.empty')}</p>
                </div>
              ) : (
                <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                  {liste.map(k => (
                    <li key={k.id} className="flex items-center justify-between py-2.5">
                      <div>
                        <span className="text-sm font-mono text-slate-800 dark:text-slate-200">{k.email}</span>
                        {k.status !== 'active' && (
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">{t('DomainMailPage:mailbox.suspended')}</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button onClick={() => parolaSifirla(k)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">{t('DomainMailPage:mailbox.reset_password')}</button>
                        <button onClick={() => kutuDurumDegistir(k)} className="text-xs text-amber-600 dark:text-amber-400 hover:underline">{k.status === 'active' ? t('DomainMailPage:mailbox.suspend') : t('DomainMailPage:mailbox.activate')}</button>
                        <button onClick={() => sil(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('common:delete')}</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainMailPage:forwarders.title')}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
                {t('DomainMailPage:forwarders.desc')}
              </p>
              <form onSubmit={aliasEkle} className="mb-4 space-y-2">
                <div className="flex items-center gap-2">
                  {aliasCatchAll ? (
                    <span className="flex-1 px-3 py-2 border border-dashed border-slate-300 dark:border-slate-600 rounded-lg text-sm text-slate-500 dark:text-slate-400 font-mono">*@{domain?.alan_adi}</span>
                  ) : (
                    <>
                      <input value={aliasKaynak} onChange={e => setAliasKaynak(e.target.value)} required={!aliasCatchAll} placeholder={t('DomainMailPage:forwarders.source_placeholder')}
                        className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                      <span className="text-slate-500 dark:text-slate-400 text-sm">@{domain?.alan_adi}</span>
                    </>
                  )}
                </div>
                <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
                  <input type="checkbox" checked={aliasCatchAll} onChange={e => setAliasCatchAll(e.target.checked)} />
                  {t('DomainMailPage:forwarders.catch_all_label')}
                </label>
                <div className="flex items-center gap-2">
                  <input value={aliasHedef} onChange={e => setAliasHedef(e.target.value)} required placeholder={t('DomainMailPage:forwarders.destination_placeholder')}
                    className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
                  <button disabled={aliasIsleniyor || !aliasHedef || (!aliasCatchAll && !aliasKaynak)}
                    className="px-3 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
                    {aliasIsleniyor ? t('DomainMailPage:forwarders.adding') : t('common:add')}
                  </button>
                </div>
              </form>

              {aliasListe.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('DomainMailPage:forwarders.empty')}</p>
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
                          <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider text-amber-700 dark:text-amber-300 bg-amber-100 dark:bg-amber-900/30 px-1.5 py-0.5 rounded">{t('DomainMailPage:mailbox.suspended')}</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <button onClick={() => aliasDurumDegistir(a)} className="text-xs text-slate-600 dark:text-slate-300 hover:underline">
                          {a.status === 'active' ? t('DomainMailPage:mailbox.suspend') : t('DomainMailPage:mailbox.activate')}
                        </button>
                        <button onClick={() => aliasSil(a)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('common:delete')}</button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <form onSubmit={spamKaydet} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainMailPage:spam.title')}</h3>
                  <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                    {t('DomainMailPage:spam.desc')}
                  </p>
                </div>
                <span className={`shrink-0 text-[10px] uppercase tracking-wider font-semibold px-2 py-1 rounded ${
                  rspamd ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' :
                    'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
                }`}>{rspamd ? t('DomainMailPage:spam.active_badge') : t('DomainMailPage:spam.not_installed_badge')}</span>
              </div>
              <label className="mt-4 flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={spam.enabled}
                  onChange={e => setSpam(s => ({ ...s, enabled: e.target.checked }))}/>
                {t('DomainMailPage:spam.enable_label')}
              </label>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
                {([
                  ['greylist_score', t('DomainMailPage:spam.greylist_label'), t('DomainMailPage:spam.greylist_help')],
                  ['add_header_score', t('DomainMailPage:spam.add_header_label'), t('DomainMailPage:spam.add_header_help')],
                  ['reject_score', t('DomainMailPage:spam.reject_label'), t('DomainMailPage:spam.reject_help')],
                ] as const).map(([key, label, help]) => (
                  <label key={key} className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainMailPage:spam.score_suffix', { label })}</span>
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
                  {spamIsleniyor ? t('DomainMailPage:spam.verifying') : t('DomainMailPage:spam.save')}
                </button>
              </div>
            </form>

            {liste.length > 0 && (
              <form onSubmit={autoKaydet} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainMailPage:autoresponder.title')}</h3>
                <p className="text-xs text-slate-500 mt-1">{t('DomainMailPage:autoresponder.desc')}</p>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
                  <label className="block sm:col-span-2">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainMailPage:autoresponder.mailbox_label')}</span>
                    <select value={auto.mailbox_id} onChange={e => autoYukle(Number(e.target.value))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono">
                      {liste.map(m => <option key={m.id} value={m.id}>{m.email}</option>)}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainMailPage:autoresponder.interval_label')}</span>
                    <select value={auto.interval_days} onChange={e => setAuto(a => ({ ...a, interval_days: Number(e.target.value) }))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm">
                      {[1,2,3,7,14,30].map(n => <option key={n} value={n}>{t('DomainMailPage:autoresponder.interval_days', { n })}</option>)}
                    </select>
                  </label>
                </div>
                <input value={auto.subject} onChange={e => setAuto(a => ({ ...a, subject: e.target.value }))} required maxLength={255}
                  placeholder={t('DomainMailPage:autoresponder.subject_placeholder')} className="mt-3 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm"/>
                <textarea value={auto.body} onChange={e => setAuto(a => ({ ...a, body: e.target.value }))} required maxLength={10000} rows={4}
                  placeholder={t('DomainMailPage:autoresponder.body_placeholder')} className="mt-3 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm"/>
                <div className="mt-3 flex items-center justify-between gap-2">
                  <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={auto.enabled} onChange={e => setAuto(a => ({ ...a, enabled: e.target.checked }))}/> {t('DomainMailPage:autoresponder.enabled_label')}</label>
                  <div className="flex gap-2">
                    <button type="button" onClick={autoSil} disabled={autoIsleniyor} className="px-3 py-1.5 text-xs text-red-600 border border-red-300 rounded">{t('DomainMailPage:autoresponder.remove')}</button>
                    <button disabled={autoIsleniyor || !auto.body || !auto.mailbox_id} className="px-3 py-1.5 text-xs bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded disabled:opacity-50">{t('common:save')}</button>
                  </div>
                </div>
              </form>
            )}

            {liste.length > 0 && (
              <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainMailPage:filters.title')}</h3>
                <p className="text-xs text-slate-500 mt-1">{t('DomainMailPage:filters.desc')}</p>
                <form onSubmit={filtreEkle} className="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-4">
                  <select value={filtre.mailbox_id} onChange={e => setFiltre(f => ({ ...f, mailbox_id: Number(e.target.value) }))}
                    className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs font-mono">
                    {liste.map(m => <option key={m.id} value={m.id}>{m.email}</option>)}
                  </select>
                  <input value={filtre.name} onChange={e => setFiltre(f => ({ ...f, name: e.target.value }))} required placeholder={t('DomainMailPage:filters.name_placeholder')}
                    className="px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs"/>
                  <div className="flex gap-2">
                    <select value={filtre.match_field} onChange={e => setFiltre(f => ({ ...f, match_field: e.target.value as MailFilter['match_field'] }))}
                      className="w-28 px-2 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs">
                      <option value="from">{t('DomainMailPage:filters.field_from')}</option><option value="to">{t('DomainMailPage:filters.field_to')}</option><option value="subject">{t('DomainMailPage:filters.field_subject')}</option>
                    </select>
                    <input value={filtre.match_value} onChange={e => setFiltre(f => ({ ...f, match_value: e.target.value }))} required placeholder={t('DomainMailPage:filters.value_placeholder')}
                      className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs"/>
                  </div>
                  <div className="flex gap-2">
                    <select value={filtre.action_type} onChange={e => setFiltre(f => ({ ...f, action_type: e.target.value as MailFilter['action_type'], action_value: e.target.value === 'move' ? 'Junk' : '' }))}
                      className="w-28 px-2 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs">
                      <option value="move">{t('DomainMailPage:filters.action_move')}</option><option value="redirect">{t('DomainMailPage:filters.action_redirect')}</option><option value="discard">{t('DomainMailPage:filters.action_discard')}</option>
                    </select>
                    {filtre.action_type !== 'discard' && <input value={filtre.action_value} onChange={e => setFiltre(f => ({ ...f, action_value: e.target.value }))} required
                      placeholder={filtre.action_type === 'move' ? 'Junk' : t('DomainMailPage:filters.action_value_placeholder_redirect')}
                      className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs"/>}
                  </div>
                  <button disabled={filtreIsleniyor || !filtre.mailbox_id} className="sm:col-span-2 px-3 py-2 bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded text-xs disabled:opacity-50">{t('DomainMailPage:filters.submit')}</button>
                </form>
                {filtreler.length > 0 && <ul className="mt-4 divide-y divide-slate-100 dark:divide-slate-700">
                  {filtreler.map(f => <li key={f.id} className="py-2 flex items-center justify-between gap-3 text-xs">
                    <div><span className="font-semibold">{f.name}</span> · <span className="font-mono">{f.email}</span><div className="text-slate-500">{f.match_field}: “{f.match_value}” → {f.action_type}{f.action_value ? `: ${f.action_value}` : ''}</div></div>
                    <button onClick={() => filtreSil(f)} className="text-red-600">{t('common:delete')}</button>
                  </li>)}
                </ul>}
              </div>
            )}

            {liste.length > 0 && (
              <form onSubmit={limitKaydet} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainMailPage:limits.title')}</h3>
                <p className="text-xs text-slate-500 mt-1">{t('DomainMailPage:limits.desc')}</p>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainMailPage:limits.mailbox_label')}</span>
                    <select value={limit.mailbox_id} onChange={e => limitYukle(Number(e.target.value))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-xs font-mono">
                      {liste.map(m => <option key={m.id} value={m.id}>{m.email}</option>)}
                    </select>
                  </label>
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainMailPage:limits.hour_label')}</span>
                    <input type="number" min={0} max={100000} value={limit.hour_limit}
                      onChange={e => setLimit(x => ({ ...x, hour_limit: Number(e.target.value) }))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono"/>
                    <span className="text-[10px] text-slate-500">{t('DomainMailPage:limits.usage', { n: limit.sent_hour })}</span>
                  </label>
                  <label className="block">
                    <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainMailPage:limits.day_label')}</span>
                    <input type="number" min={0} max={100000} value={limit.day_limit}
                      onChange={e => setLimit(x => ({ ...x, day_limit: Number(e.target.value) }))}
                      className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded text-sm font-mono"/>
                    <span className="text-[10px] text-slate-500">{t('DomainMailPage:limits.usage', { n: limit.sent_day })}</span>
                  </label>
                </div>
                {limit.spam_suspended_at && <div className="mt-3 text-xs text-red-700 bg-red-50 dark:bg-red-900/20 dark:text-red-300 p-2 rounded">
                  {t('DomainMailPage:limits.suspended_notice', { date: limit.spam_suspended_at })}
                </div>}
                <div className="mt-3 flex justify-end">
                  <button disabled={limitIsleniyor || !limit.mailbox_id}
                    className="px-3 py-1.5 text-xs bg-slate-900 text-white dark:bg-white dark:text-slate-900 rounded disabled:opacity-50">{t('DomainMailPage:limits.save')}</button>
                </div>
              </form>
            )}
            </div>

            {/* İki kapatma yolu yan yana. Ayrımın görsel olarak da okunması
                gerekiyor: soldaki GERİ ALINABİLİR (amber), sağdaki GERİ
                DÖNÜŞSÜZ (kırmızı + dolu buton). Aynı renkte olsalardı kullanıcı
                ikisini eşdeğer sanardı. */}
            <div className="mt-5 grid grid-cols-1 lg:grid-cols-2 gap-5 items-start">
              <div className="bg-white dark:bg-slate-800 border border-amber-200 dark:border-amber-900/50 rounded-2xl p-5 shadow-sm">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainMailPage:disable.title')}</h3>
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">{t('DomainMailPage:disable.desc')}</p>
                <button onClick={devredisiBirak} disabled={isleniyor || siliniyor}
                  className="mt-3 px-4 py-2 border border-amber-300 dark:border-amber-700 text-amber-800 dark:text-amber-300 hover:bg-amber-50 dark:hover:bg-amber-900/30 disabled:opacity-50 text-sm font-medium rounded-lg transition">
                  {isleniyor ? t('DomainMailPage:disable.working') : t('DomainMailPage:disable.button')}
                </button>
              </div>

              <div className="bg-white dark:bg-slate-800 border border-red-300 dark:border-red-800 rounded-2xl p-5 shadow-sm">
                <h3 className="text-sm font-semibold text-red-800 dark:text-red-300">{t('DomainMailPage:purge.title')}</h3>
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">{t('DomainMailPage:purge.desc')}</p>
                <ul className="mt-2 space-y-0.5 text-xs text-slate-500 dark:text-slate-400 list-disc list-inside">
                  <li>{t('DomainMailPage:purge.item_mailboxes')}</li>
                  <li>{t('DomainMailPage:purge.item_files')}</li>
                  <li>{t('DomainMailPage:purge.item_dns')}</li>
                </ul>
                <button onClick={hizmetiSil} disabled={isleniyor || siliniyor}
                  className="mt-3 px-4 py-2 bg-red-600 hover:bg-red-700 text-white disabled:opacity-50 text-sm font-medium rounded-lg transition">
                  {siliniyor ? t('DomainMailPage:purge.working') : t('DomainMailPage:purge.button')}
                </button>
              </div>
            </div>
          </>
        )}

        <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">{t('DomainMailPage:back_to_subscription')}</Link></div>
      </div>
    </div>
  )
}
