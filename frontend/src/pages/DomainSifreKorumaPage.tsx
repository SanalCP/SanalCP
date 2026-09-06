import { modalOnay } from '@/lib/dialog'
import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ParolaGirdisi from '@/components/ParolaGirdisi'

type Domain = { id: number; alan_adi: string }
type Kayit = { id: number; yol: string; kullanici: string; created_at: string }

export default function DomainSifreKorumaPage() {
  const { t } = useTranslation(['DomainSifreKorumaPage', 'common'])
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [liste, setListe] = useState<Kayit[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [yol, setYol] = useState('/gizli')
  const [kullanici, setKullanici] = useState('')
  const [parola, setParola] = useState('')
  const [kaydediyor, setKaydediyor] = useState(false)

  function yukle() {
    if (!id) return
    setYuk(true)
    api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    api.get<Kayit[]>(`/domains/${id}/koruma`)
      .then(r => setListe(r.data || [])).catch(e => setHata(apiHata(e))).finally(() => setYuk(false))
  }
  useEffect(yukle, [id])

  async function ekle(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setOk(null); setKaydediyor(true)
    try {
      await api.post(`/domains/${id}/koruma`, { yol, kullanici, parola })
      setOk(t('DomainSifreKorumaPage:protected', { yol, kullanici }))
      setParola('')
      yukle()
    } catch (err) {
      setHata(apiHata(err, t('DomainSifreKorumaPage:add_failed')))
    } finally { setKaydediyor(false) }
  }

  async function sil(k: Kayit) {
    if (!(await modalOnay(t('DomainSifreKorumaPage:confirm_remove', { kullanici: k.kullanici, yol: k.yol })))) return
    setHata(null); setOk(null)
    try {
      await api.delete(`/domains/${id}/koruma/${k.id}`)
      yukle()
    } catch (err) { setHata(apiHata(err, t('DomainSifreKorumaPage:delete_failed'))) }
  }

  // yol -> o yola ait kullanıcılar
  const grup = liste.reduce<Record<string, Kayit[]>>((a, k) => { (a[k.yol] ||= []).push(k); return a }, {})

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { etiket: t('common:home'), href: '/' },
          { etiket: t('common:domain'), href: '/domainler' },
          { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
          { etiket: t('DomainSifreKorumaPage:breadcrumb_title') },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainSifreKorumaPage:title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          {t('DomainSifreKorumaPage:subtitle')}
        </p>

        {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
        {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

        {/* Ekleme formu */}
        <form onSubmit={ekle} className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainSifreKorumaPage:new_protection_title')}</h3>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <label className="block">
              <span className="text-xs text-slate-500 dark:text-slate-400">{t('DomainSifreKorumaPage:dir_path_label')}</span>
              <input value={yol} onChange={e => setYol(e.target.value)} required placeholder={t('DomainSifreKorumaPage:dir_path_placeholder')}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
            <label className="block">
              <span className="text-xs text-slate-500 dark:text-slate-400">{t('DomainSifreKorumaPage:username_label')}</span>
              <input value={kullanici} onChange={e => setKullanici(e.target.value)} required placeholder={t('DomainSifreKorumaPage:username_placeholder')}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
            </label>
            <label className="block">
              <span className="text-xs text-slate-500 dark:text-slate-400">{t('DomainSifreKorumaPage:password_label')}</span>
              <div className="mt-1">
                <ParolaGirdisi value={parola} onChange={setParola} placeholder="••••••••" required
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
              </div>
            </label>
          </div>
          <p className="text-[11px] text-slate-400 mt-2">{t('DomainSifreKorumaPage:dir_help')}</p>
          <button disabled={kaydediyor} className="mt-3 px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {kaydediyor ? t('DomainSifreKorumaPage:adding') : t('DomainSifreKorumaPage:add_button')}
          </button>
        </form>

        {/* Mevcut korumalar */}
        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainSifreKorumaPage:existing_protected_title')}</h3>
          {yuk ? (
            <div className="text-sm text-slate-400">{t('common:loading')}</div>
          ) : liste.length === 0 ? (
            <div className="text-center py-8">
              <div className="text-3xl mb-2">🔒</div>
              <p className="text-sm text-slate-500 dark:text-slate-400">{t('DomainSifreKorumaPage:no_protected')}</p>
            </div>
          ) : (
            <div className="space-y-4">
              {Object.entries(grup).map(([g, ks]) => (
                <div key={g} className="border border-slate-100 dark:border-slate-700 rounded-lg overflow-hidden">
                  <div className="flex items-center gap-2 px-3 py-2 bg-slate-50 dark:bg-slate-900/40">
                    <span className="text-sm">🔒</span>
                    <span className="font-mono text-sm text-slate-700 dark:text-slate-200">{g}</span>
                    <span className="text-xs text-slate-400">{t('DomainSifreKorumaPage:users_count', { count: ks.length })}</span>
                  </div>
                  <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
                    {ks.map(k => (
                      <li key={k.id} className="flex items-center justify-between px-3 py-2">
                        <span className="text-sm text-slate-600 dark:text-slate-300">{k.kullanici}</span>
                        <button onClick={() => sil(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('DomainSifreKorumaPage:remove')}</button>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">{t('DomainSifreKorumaPage:back_to_subscription')}</Link></div>
      </div>
    </div>
  )
}