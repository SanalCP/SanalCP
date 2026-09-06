// sanal-dark-swept
// sanal-dark-swept-v2
import { modalOnay, modalUyari } from '@/lib/dialog'
import { useCallback, useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import Modal from '@/components/Modal'
import { T } from '@/lib/tablo'

type Gorev = {
  idx: number
  dakika: string
  saat: string
  gun: string
  ay: string
  hafta: string
  komut: string
  yorum?: string
}

type Domain = { id: number; alan_adi: string; sistem_kullanici: string }

type ListResp = { sistem_kullanici: string; toplam: number; gorevler: Gorev[] }

function onAyarlar(t: (k: string) => string): Array<{ etiket: string; secim: { dakika: string; saat: string; gun: string; ay: string; hafta: string } }> {
  return [
    { etiket: t('DomainCronPage:presets.everyMinute'), secim: { dakika: '*',  saat: '*', gun: '*', ay: '*', hafta: '*' } },
    { etiket: t('DomainCronPage:presets.everyHour'),   secim: { dakika: '0',  saat: '*', gun: '*', ay: '*', hafta: '*' } },
    { etiket: t('DomainCronPage:presets.dailyAt3'),    secim: { dakika: '0',  saat: '3', gun: '*', ay: '*', hafta: '*' } },
    { etiket: t('DomainCronPage:presets.mondayAt9'),   secim: { dakika: '0',  saat: '9', gun: '*', ay: '*', hafta: '1' } },
    { etiket: t('DomainCronPage:presets.every5Min'),   secim: { dakika: '*/5', saat: '*', gun: '*', ay: '*', hafta: '*' } },
    { etiket: t('DomainCronPage:presets.every15Min'),  secim: { dakika: '*/15', saat: '*', gun: '*', ay: '*', hafta: '*' } },
    { etiket: t('DomainCronPage:presets.firstOfMonth'), secim: { dakika: '0',  saat: '0', gun: '1', ay: '*', hafta: '*' } },
  ]
}

export default function DomainCronPage() {
  const { t } = useTranslation(['DomainCronPage', 'common'])
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [gorevler, setGorevler] = useState<Gorev[]>([])
  const [yukleniyor, setYukleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [modal, setModal] = useState(false)

  const yukle = useCallback(() => {
    if (!id) return
    setYukleniyor(true); setHata(null)
    api.get<ListResp>(`/domains/${id}/cron`)
      .then(r => setGorevler(r.data.gorevler))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYukleniyor(false))
  }, [id])

  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    yukle()
  }, [id, yukle])

  async function sil(g: Gorev) {
    if (!(await modalOnay(t('DomainCronPage:confirmDelete', { command: g.komut.slice(0, 60) })))) return
    try {
      await api.delete(`/domains/${id}/cron/${g.idx}`)
      yukle()
    } catch (e) {
      await modalUyari(apiHata(e, t('DomainCronPage:errors.deleteFailed')))
    }
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('DomainCronPage:breadcrumb.domains'), href: '/domainler' },
        { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainCronPage:breadcrumb.title') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainCronPage:title')}</h1>
      {domain && (
        <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
          <Link to={`/abonelikler/${id}`} className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-400 dark:hover:text-brand-300">{domain.alan_adi}</Link>
          {' · '}
          <span className="font-mono text-slate-600 dark:text-slate-400">/var/spool/cron/{domain.sistem_kullanici}</span>
        </p>
      )}

      <div className="flex items-center gap-2 mb-4">
        <button
          onClick={() => setModal(true)}
          className="inline-flex items-center gap-1.5 px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md shadow-sm transition"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          {t('DomainCronPage:addTask')}
        </button>
        <button onClick={yukle} className="ta-secondary-button">{t('DomainCronPage:refresh')}</button>
        <span className="ml-auto text-sm text-slate-500 dark:text-slate-500">{t('DomainCronPage:taskCount', { count: gorevler.length })}</span>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
        {yukleniyor ? (
          <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div>
        ) : gorevler.length === 0 ? (
          <div className="py-16 text-center">
            <div className="w-14 h-14 mx-auto rounded-full bg-slate-100 dark:bg-slate-800 flex items-center justify-center mb-3">
              <svg className="w-7 h-7 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <p className="text-sm text-slate-500 dark:text-slate-500">{t('DomainCronPage:empty')}</p>
          </div>
        ) : (
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700`}>
              <tr>
                <th className={T.baslik}>{t('DomainCronPage:table.command')}</th>
                <th className={T.baslik}>{t('DomainCronPage:table.minute')}</th>
                <th className={T.baslik}>{t('DomainCronPage:table.hour')}</th>
                <th className={T.baslik}>{t('DomainCronPage:table.day')}</th>
                <th className={T.baslik}>{t('DomainCronPage:table.month')}</th>
                <th className={T.baslik}>{t('DomainCronPage:table.week')}</th>
                <th className={`${T.baslik} text-right`}>{t('DomainCronPage:table.action')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
              {gorevler.map((g) => (
                <tr key={g.idx} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800`}>
                  <td className={T.hucreBaslik}>
                    <div className="font-mono text-slate-800 dark:text-slate-200 truncate max-w-md lg:text-sm text-base" title={g.komut}>{g.komut}</div>
                    {g.yorum && <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5 font-normal">{g.yorum}</div>}
                  </td>
                  <td className={T.hucre} data-etiket={t('DomainCronPage:table.minute')}><span className="font-mono">{g.dakika}</span></td>
                  <td className={T.hucre} data-etiket={t('DomainCronPage:table.hour')}><span className="font-mono">{g.saat}</span></td>
                  <td className={T.hucre} data-etiket={t('DomainCronPage:table.day')}><span className="font-mono">{g.gun}</span></td>
                  <td className={T.hucre} data-etiket={t('DomainCronPage:table.month')}><span className="font-mono">{g.ay}</span></td>
                  <td className={T.hucre} data-etiket={t('DomainCronPage:table.week')}><span className="font-mono">{g.hafta}</span></td>
                  <td className={T.hucreAksiyon}>
                    <button onClick={() => sil(g)} className="text-sm text-red-600 dark:text-red-400 hover:text-red-700 dark:text-red-300 px-2 py-1 -mx-2 lg:mx-0 rounded hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 transition">{t('DomainCronPage:table.delete')}</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <CronEkleModal acik={modal} onKapat={() => setModal(false)} onEklendi={yukle} domainId={Number(id)} />
    </div>
  )
}

function CronEkleModal({ acik, onKapat, onEklendi, domainId }: {
  acik: boolean; onKapat: () => void; onEklendi: () => void; domainId: number
}) {
  const { t } = useTranslation(['DomainCronPage', 'common'])
  const ON_AYARLAR = onAyarlar(t)
  const [dakika, setDakika] = useState('0')
  const [saat, setSaat] = useState('3')
  const [gun, setGun] = useState('*')
  const [ay, setAy] = useState('*')
  const [hafta, setHafta] = useState('*')
  const [komut, setKomut] = useState('')
  const [yorum, setYorum] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  function uygula(p: typeof ON_AYARLAR[number]['secim']) {
    setDakika(p.dakika); setSaat(p.saat); setGun(p.gun); setAy(p.ay); setHafta(p.hafta)
  }

  async function gonder(e: React.FormEvent) {
    e.preventDefault()
    setIsleniyor(true); setHata(null)
    try {
      await api.post(`/domains/${domainId}/cron`, { dakika, saat, gun, ay, hafta, komut: komut.trim(), yorum: yorum.trim() })
      onEklendi()
      setKomut(''); setYorum('')
      onKapat()
    } catch (e) {
      setHata(apiHata(e, t('DomainCronPage:errors.addFailed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={acik} baslik={t('DomainCronPage:modal.title')} onKapat={onKapat} genislik="lg">
      <form onSubmit={gonder} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('DomainCronPage:modal.presetsLabel')}</label>
          <div className="flex flex-wrap gap-1.5">
            {ON_AYARLAR.map(p => (
              <button
                key={p.etiket}
                type="button"
                onClick={() => uygula(p.secim)}
                className="px-2 py-1 text-xs bg-slate-100 dark:bg-slate-800 hover:bg-brand-100 dark:bg-brand-900/30 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 rounded transition"
              >
                {p.etiket}
              </button>
            ))}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
          <Alan etiket={t('DomainCronPage:modal.minute')} value={dakika} onChange={setDakika} />
          <Alan etiket={t('DomainCronPage:modal.hour')}   value={saat}   onChange={setSaat} />
          <Alan etiket={t('DomainCronPage:modal.day')}    value={gun}    onChange={setGun} />
          <Alan etiket={t('DomainCronPage:modal.month')}  value={ay}     onChange={setAy} />
          <Alan etiket={t('DomainCronPage:modal.week')}   value={hafta}  onChange={setHafta} />
        </div>
        <p className="ta-hint">{t('DomainCronPage:modal.cronHint')}</p>

        <div>
          <label className="ta-label">{t('DomainCronPage:modal.commandLabel')}</label>
          <input
            type="text"
            value={komut}
            onChange={e => setKomut(e.target.value)}
            placeholder={t('DomainCronPage:modal.commandPlaceholder')}
            required
            className="ta-input w-full font-mono"
          />
        </div>

        <div>
          <label className="ta-label">{t('DomainCronPage:modal.descLabel')}</label>
          <input
            type="text"
            value={yorum}
            onChange={e => setYorum(e.target.value)}
            placeholder={t('DomainCronPage:modal.descPlaceholder')}
            className="ta-input w-full"
          />
        </div>

        {hata && <div className="ta-form-error" role="alert">{hata}</div>}

        <div className="ta-form-actions">
          <button type="button" onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('common:cancel')}</button>
          <button type="submit" disabled={isleniyor || !komut.trim()} className="ta-primary-button">
            {isleniyor ? t('DomainCronPage:modal.adding') : t('DomainCronPage:modal.add')}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function Alan({ etiket, value, onChange }: { etiket: string; value: string; onChange: (v: string) => void }) {
  return (
    <div>
      <label className="ta-label-sm">{etiket}</label>
      <input
        type="text"
        value={value}
        onChange={e => onChange(e.target.value)}
        className="ta-input ta-input-sm w-full font-mono"
      />
    </div>
  )
}
