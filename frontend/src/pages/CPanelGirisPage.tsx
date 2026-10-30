// sanal-dark-swept
// sanal-dark-swept-v2
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiHata } from '@/lib/api'
import { useAuth } from '@/store/auth'
import axios from 'axios'

export default function CPanelGirisPage() {
  const { t } = useTranslation(['CPanelGirisPage'])
  const [kullanici, setKullanici] = useState('')
  const [parola, setParola] = useState('')
  const [kod, setKod] = useState('')
  const [ikiFa, setIkiFa] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [yuk, setYuk] = useState(false)
  const nav = useNavigate()

  async function gir(e: React.FormEvent) {
    e.preventDefault()
    setYuk(true); setHata(null)
    try {
      const { data } = await axios.post('/api/v1/musteri/login', { kullanici, parola, kod })
      if (data.iki_fa_gerekli) {
        setIkiFa(true)
        return
      }
      const { bitis, domain_id, alan_adi } = data
      // Oturum HttpOnly çerezde geldi; store yalnız arayüz durumunu yazıyor.
      useAuth.getState().girisMusteri(bitis, domain_id, alan_adi, kullanici)
      nav('/abonelikler/' + domain_id, { replace: true })
      setTimeout(() => window.location.reload(), 100)
    } catch (e) {
      setHata(apiHata(e, t('CPanelGirisPage:login_failed')))
    } finally {
      setYuk(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-100 to-brand-50 px-4">
      <div className="w-full max-w-md bg-white dark:bg-slate-800 rounded-2xl shadow-xl p-7">
        <div className="text-center mb-6">
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 text-2xl mb-3">🌐</div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">{t('CPanelGirisPage:title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-500 mt-1">{t('CPanelGirisPage:subtitle')}</p>
        </div>

        {hata && (
          <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>
        )}

        <form onSubmit={gir} className="space-y-3">
          <div>
            <label htmlFor="musteri-kullanici" className="ta-label-sm">{t('CPanelGirisPage:username')}</label>
            <input id="musteri-kullanici" type="text" value={kullanici} onChange={e => setKullanici(e.target.value)}
              autoComplete="username" required autoFocus readOnly={ikiFa}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-sm focus:border-brand-500 outline-none" />
          </div>
          <div>
            <label htmlFor="musteri-parola" className="ta-label-sm">{t('CPanelGirisPage:password')}</label>
            <input id="musteri-parola" type="password" value={parola} onChange={e => setParola(e.target.value)}
              autoComplete="current-password" required readOnly={ikiFa}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-sm focus:border-brand-500 outline-none" />
          </div>
          {ikiFa && (
            <div>
              <label htmlFor="musteri-kod" className="ta-label-sm">{t('CPanelGirisPage:twofa_code')}</label>
              <input id="musteri-kod" type="text" value={kod}
                onChange={e => setKod(e.target.value.replace(/\D/g, '').slice(0, 6))}
                inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}"
                required autoFocus
                className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-center text-lg tracking-[0.3em] focus:border-brand-500 outline-none" />
              <p className="mt-2 text-xs text-slate-500">{t('CPanelGirisPage:twofa_hint')}</p>
            </div>
          )}
          <button type="submit" disabled={yuk || !kullanici || !parola || (ikiFa && kod.length !== 6)}
            className="w-full px-4 py-2.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 font-medium rounded-md">
            {yuk ? t('CPanelGirisPage:submitting') : ikiFa ? t('CPanelGirisPage:verify_and_login') : t('CPanelGirisPage:submit')}
          </button>
          {ikiFa && (
            <button type="button" disabled={yuk}
              onClick={() => { setIkiFa(false); setKod(''); setParola(''); setHata(null) }}
              className="w-full py-2 text-sm text-slate-500 hover:text-slate-700">
              {t('CPanelGirisPage:change_account')}
            </button>
          )}
        </form>
      </div>
    </div>
  )
}
