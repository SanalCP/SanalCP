// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import { useAuth } from '@/store/auth'
import LanguageSwitcher from '@/components/LanguageSwitcher'

type LoginResp = {
  token?: string
  bitis?: number
  kullanici?: { id: number; adi: string; rol: 'admin' | 'reseller' | 'user'; ad_soyad?: string }
  iki_fa_gerekli?: boolean
}

export default function LoginPage() {
  const { t } = useTranslation(['LoginPage'])
  const [kullanici, setKullanici] = useState('')
  const [parola, setParola] = useState('')
  const [kod, setKod] = useState('')
  const [ikiFa, setIkiFa] = useState(false)
  const [yukleniyor, setYukleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const navigate = useNavigate()
  const giris = useAuth((s) => s.giris)
  const [surum, setSurum] = useState('')

  // /healthz auth gerektirmez (login ekranından ÖNCE erişilebilir olmalı) ve
  // panelin TEK sürüm kaynağından (internal/system.SurumNo) geliyor — burada
  // sabit bir string YAZILMAMALI, aksi hâlde her release'de unutulup
  // (nitekim öyle oldu: "0.2.0-f1" aylardır güncellenmeyen bir kalıntıydı).
  useEffect(() => {
    fetch('/healthz').then(r => r.json()).then(d => { if (d?.surum) setSurum(d.surum) }).catch(() => {})
  }, [])

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setYukleniyor(true)
    try {
      const { data } = await api.post<LoginResp>('/auth/login', { kullanici, parola, kod })
      if (data.iki_fa_gerekli) {
        setIkiFa(true); setYukleniyor(false)
        return
      }
      giris(data.token!, data.kullanici!, data.bitis!)
      navigate('/', { replace: true })
    } catch (err) {
      setHata(apiHata(err, t('LoginPage:login_failed')))
    } finally {
      setYukleniyor(false)
    }
  }

  return (
    <div className="relative min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-50 to-orange-50 dark:from-slate-950 dark:to-slate-900 px-4">
      <LanguageSwitcher className="absolute top-4 right-4 px-2.5 py-1.5 text-xs font-semibold text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-white/60 dark:hover:bg-slate-800/60 rounded-md transition" />
      <div className="w-full max-w-md">
        <div className="flex items-center justify-center mb-8">
          <div className="w-12 h-12 rounded-2xl bg-brand-600 flex items-center justify-center shadow-lg shadow-brand-600/30">
            <svg viewBox="0 0 32 32" className="w-7 h-7 text-white" fill="currentColor">
              <path d="M9 10h14v3H9zM9 15h14v3H9zM9 20h9v3H9z" />
            </svg>
          </div>
          <div className="ml-3">
            <div className="text-xl font-semibold text-slate-900 dark:text-slate-100">SanalCP</div>
            <div className="text-xs text-slate-500 dark:text-slate-500">{t('LoginPage:subtitle')}</div>
          </div>
        </div>

        <div className="bg-white dark:bg-slate-800 rounded-2xl shadow-xl border border-slate-200 dark:border-slate-700/60 p-8">
          <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('LoginPage:welcome')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">{t('LoginPage:continue_prompt')}</p>

          <form onSubmit={onSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('LoginPage:username')}</label>
              <input
                type="text"
                value={kullanici}
                onChange={(e) => setKullanici(e.target.value)}
                autoComplete="username"
                autoFocus
                required
                className="w-full px-3.5 py-2.5 border border-slate-300 dark:border-slate-600 rounded-lg focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('LoginPage:password')}</label>
              <input
                type="password"
                value={parola}
                onChange={(e) => setParola(e.target.value)}
                autoComplete="current-password"
                required
                readOnly={ikiFa}
                className="w-full px-3.5 py-2.5 border border-slate-300 dark:border-slate-600 rounded-lg focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition disabled:opacity-60"
              />
            </div>

            {ikiFa && (
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('LoginPage:twofa_code')}</label>
                <input
                  type="text"
                  inputMode="numeric"
                  value={kod}
                  onChange={(e) => setKod(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  autoFocus
                  placeholder="000000"
                  className="w-full px-3.5 py-2.5 text-center text-lg font-mono tracking-[0.4em] border border-slate-300 dark:border-slate-600 rounded-lg focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none transition"
                />
                <p className="text-xs text-slate-400 dark:text-slate-500 mt-1.5">{t('LoginPage:twofa_hint')}</p>
              </div>
            )}

            {hata && (
              <div className="px-3.5 py-2.5 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">
                {hata}
              </div>
            )}

            <button
              type="submit"
              disabled={yukleniyor}
              className="w-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 font-medium py-2.5 rounded-lg transition shadow-lg shadow-brand-600/20 disabled:shadow-none"
            >
              {yukleniyor ? t('LoginPage:signing_in') : ikiFa ? t('LoginPage:verify_and_login') : t('LoginPage:login')}
            </button>
          </form>
        </div>

        {surum && (
          <p className="text-center text-xs text-slate-400 dark:text-slate-500 mt-6">
            {t('LoginPage:footer', { version: surum })}
          </p>
        )}
      </div>
    </div>
  )
}