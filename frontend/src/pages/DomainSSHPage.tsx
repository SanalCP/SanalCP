import KopyalaButton from '@/components/KopyalaButton'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, Link } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Durum = {
  alan_adi: string
  kullanici: string
  aktif: boolean
  shell: string
  ssh_host: string
  ssh_port: number
  anahtar_var: boolean
  is_demo: boolean
}

export default function DomainSSHPage() {
  const { t } = useTranslation(['DomainSSHPage', 'common'])
  const { id } = useParams()
  const [d, setD] = useState<Durum | null>(null)
  const [yuk, setYuk] = useState(true)
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [anahtar, setAnahtar] = useState('')
  const [uretiliyor, setUretiliyor] = useState(false)
  const [uretilenAnahtar, setUretilenAnahtar] = useState<{ genel: string; ozel: string; dosyaAdi: string } | null>(null)

  function yukle() {
    if (!id) return
    setYuk(true); setHata(null)
    api.get<Durum>(`/domains/${id}/ssh`)
      .then(r => setD(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [id])

  async function toggle(aktif: boolean) {
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      await api.put(`/domains/${id}/ssh`, { aktif })
      setBasari(aktif ? t('DomainSSHPage:enabled_msg') : t('DomainSSHPage:disabled_msg'))
      setTimeout(() => setBasari(null), 4000)
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainSSHPage:action_failed')))
    } finally { setIsleniyor(false) }
  }

  async function anahtarKaydet() {
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      const { data } = await api.put(`/domains/${id}/ssh/anahtar`, { anahtar })
      setBasari(data.anahtar_var ? t('DomainSSHPage:key_saved') : t('DomainSSHPage:keys_cleared'))
      setTimeout(() => setBasari(null), 4000)
      setAnahtar('')
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainSSHPage:key_save_failed')))
    } finally { setIsleniyor(false) }
  }

  async function anahtarUret() {
    setUretiliyor(true); setHata(null); setBasari(null)
    try {
      const { data } = await api.post(`/domains/${id}/ssh/anahtar-uret`)
      setUretilenAnahtar({ genel: data.genel_anahtar, ozel: data.ozel_anahtar, dosyaAdi: data.dosya_adi })
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainSSHPage:key_generate_failed')))
    } finally { setUretiliyor(false) }
  }

  function ozelAnahtarIndir() {
    if (!uretilenAnahtar) return
    const blob = new Blob([uretilenAnahtar.ozel], { type: 'application/x-pem-file' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = uretilenAnahtar.dosyaAdi
    document.body.appendChild(a); a.click(); a.remove()
    URL.revokeObjectURL(url)
  }

  if (yuk) return <div className="px-6 py-5 text-slate-400">{t('common:loading')}</div>
  if (!d) return <div className="px-6 py-5"><div className="text-sm text-red-600">{hata || t('DomainSSHPage:not_found')}</div></div>

  const sshKomut = `ssh ${d.kullanici}@${d.ssh_host} -p ${d.ssh_port}`

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { etiket: t('DomainSSHPage:breadcrumb_home'), href: '/' },
          { etiket: t('DomainSSHPage:breadcrumb_domains'), href: '/domainler' },
          { etiket: d.alan_adi, href: `/abonelikler/${id}` },
          { etiket: t('DomainSSHPage:breadcrumb_ssh') },
        ]} />

        <div className="flex items-start justify-between gap-4 mb-1">
          <div>
            <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('DomainSSHPage:title')}</h1>
            <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
              <span className="font-mono">{d.alan_adi}</span> {t('DomainSSHPage:subtitle')}
            </p>
          </div>
          <span className={`shrink-0 inline-flex items-center gap-1.5 text-xs font-semibold px-2.5 py-1 rounded-full ${
            d.aktif ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-300'
          }`}>
            <span className={`w-2 h-2 rounded-full ${d.aktif ? 'bg-emerald-500' : 'bg-slate-400'}`} />
            {d.aktif ? t('DomainSSHPage:ssh_on') : t('DomainSSHPage:ssh_off')}
          </span>
        </div>

        {hata && <div className="my-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{hata}</div>}
        {basari && <div className="my-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

        {/* Durum + toggle */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainSSHPage:shell_access')}</h3>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                {t('DomainSSHPage:shell_open_label')} <code className="font-mono">/bin/bash</code> {t('DomainSSHPage:shell_closed_label')} <code className="font-mono">/usr/sbin/nologin</code>{t('DomainSSHPage:shell_current_label')} <code className="font-mono">{d.shell || '—'}</code>
              </p>
            </div>
            {d.aktif ? (
              <button onClick={() => toggle(false)} disabled={isleniyor || d.is_demo}
                className="shrink-0 px-4 py-2 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 text-sm font-medium rounded-lg">
                {t('DomainSSHPage:close_ssh')}
              </button>
            ) : (
              <button onClick={() => toggle(true)} disabled={isleniyor || d.is_demo}
                className="shrink-0 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-lg">
                {t('DomainSSHPage:open_ssh')}
              </button>
            )}
          </div>
          {d.is_demo && <p className="mt-3 text-xs text-amber-600 dark:text-amber-400">{t('DomainSSHPage:demo_notice')}</p>}
        </div>

        {/* Bağlantı bilgisi */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainSSHPage:connection_info')}</h3>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-sm">
            <Bilgi etiket={t('DomainSSHPage:user')} deger={d.kullanici} />
            <Bilgi etiket={t('DomainSSHPage:server')} deger={d.ssh_host} />
            <Bilgi etiket={t('DomainSSHPage:port')} deger={String(d.ssh_port)} />
          </div>
          <div className="mt-3">
            <label className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainSSHPage:connection_command')}</label>
            <div className="mt-1 flex items-center gap-2">
              <code className="flex-1 px-3 py-2 bg-slate-900 text-slate-100 rounded-lg text-xs font-mono overflow-x-auto">{sshKomut}</code>
              <KopyalaButton metin={sshKomut} className="shrink-0 text-xs px-2.5 py-2 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700" />
            </div>
          </div>
          <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">{t('DomainSSHPage:password_hint')} <strong>{t('DomainSSHPage:password_hint_bold')}</strong> {t('DomainSSHPage:password_hint_rest')}</p>
        </div>

        {/* SSH Public Key */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainSSHPage:public_key_title')}</h3>
          <p className="text-xs text-slate-500 dark:text-slate-400 mb-3">
            {t('DomainSSHPage:public_key_desc')} {d.anahtar_var
              ? <span className="text-emerald-600 dark:text-emerald-400">{t('DomainSSHPage:key_defined')}</span>
              : <span className="text-slate-500">{t('DomainSSHPage:key_not_defined')}</span>}
          </p>
          <textarea
            value={anahtar}
            onChange={e => setAnahtar(e.target.value)}
            rows={4}
            spellCheck={false}
            placeholder={t('DomainSSHPage:key_placeholder')}
            className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-xs font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none"
          />
          <div className="mt-3 flex items-center justify-between">
            <p className="text-xs text-slate-400">{t('DomainSSHPage:key_clear_hint')}</p>
            <button onClick={anahtarKaydet} disabled={isleniyor || d.is_demo}
              className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-lg">
              {t('DomainSSHPage:save_key')}
            </button>
          </div>

          <div className="mt-4 pt-4 border-t border-slate-100 dark:border-slate-800 flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-slate-700 dark:text-slate-300">{t('DomainSSHPage:generate_key_title')}</p>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{t('DomainSSHPage:generate_key_desc')}</p>
            </div>
            <button onClick={anahtarUret} disabled={uretiliyor || d.is_demo}
              className="shrink-0 px-4 py-2 border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-60 text-sm font-medium rounded-lg text-slate-700 dark:text-slate-200">
              {uretiliyor ? t('DomainSSHPage:generating') : t('DomainSSHPage:generate_key')}
            </button>
          </div>

          {uretilenAnahtar && (
            <div className="mt-4 px-4 py-3 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
              <p className="text-xs font-semibold text-amber-800 dark:text-amber-300">{t('DomainSSHPage:generated_warning')}</p>
              <div className="mt-3">
                <label className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainSSHPage:generated_private_label')}</label>
                <textarea readOnly value={uretilenAnahtar.ozel} rows={6} spellCheck={false}
                  className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-[11px] font-mono outline-none" />
                <div className="mt-2 flex flex-wrap gap-2">
                  <button onClick={ozelAnahtarIndir}
                    className="text-xs px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg font-medium">
                    {t('DomainSSHPage:download_private')}
                  </button>
                  <KopyalaButton metin={uretilenAnahtar.ozel} className="text-xs px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700" />
                </div>
              </div>
              <div className="mt-3">
                <label className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainSSHPage:generated_public_label')}</label>
                <code className="mt-1 block px-3 py-2 bg-slate-900 text-slate-100 rounded-lg text-[11px] font-mono overflow-x-auto">{uretilenAnahtar.genel}</code>
              </div>
              <button onClick={() => setUretilenAnahtar(null)}
                className="mt-3 text-xs px-3 py-1.5 border border-amber-300 dark:border-amber-700 text-amber-800 dark:text-amber-300 rounded-lg hover:bg-amber-100 dark:hover:bg-amber-900/30 font-medium">
                {t('DomainSSHPage:generated_dismiss')}
              </button>
            </div>
          )}
        </div>

        <div className="mt-4">
          <Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">{t('DomainSSHPage:back_to_subscription')}</Link>
        </div>
      </div>
    </div>
  )
}

function Bilgi({ etiket, deger }: { etiket: string; deger: string }) {
  return (
    <div className="px-3 py-2 bg-slate-50 dark:bg-slate-900/40 rounded-lg border border-slate-200 dark:border-slate-700">
      <div className="text-[10px] uppercase tracking-wider text-slate-400">{etiket}</div>
      <div className="font-mono text-slate-800 dark:text-slate-200 truncate">{deger}</div>
    </div>
  )
}
