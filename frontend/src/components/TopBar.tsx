// sanal-dark-swept
// sanal-dark-swept-v2
// sp-mobil-v1
import { useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/store/auth'
import { getTheme, setTheme, type Theme } from '@/lib/theme'
import { api } from '@/lib/api'
import LanguageSwitcher from '@/components/LanguageSwitcher'

type AramaKaydi = {
  tur: 'sayfa' | 'domain' | 'musteri' | 'kullanici'
  baslik: string
  aciklama: string
  yol: string
  anahtarlar?: string
}

type Domain = { id: number; alan_adi: string; sistem_kullanici?: string; durum?: string }
type Musteri = { id: number; ad: string; eposta?: string; durum?: string }
type Kullanici = { id: number; kullanici_adi: string; ad_soyad?: string; eposta?: string; rol?: string }

type TFunc = (key: string) => string

function sayfalar(t: TFunc): Array<AramaKaydi & { roller?: string[] }> {
  return [
    { tur: 'sayfa', baslik: t('TopBar:pages.home.baslik'), aciklama: t('TopBar:pages.home.aciklama'), yol: '/', anahtarlar: 'dashboard genel bakış' },
    { tur: 'sayfa', baslik: t('TopBar:pages.domains.baslik'), aciklama: t('TopBar:pages.domains.aciklama'), yol: '/domainler', anahtarlar: 'site hosting abonelik', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.dns.baslik'), aciklama: t('TopBar:pages.dns.aciklama'), yol: '/dns', anahtarlar: 'zone nameserver ns', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.mail.baslik'), aciklama: t('TopBar:pages.mail.aciklama'), yol: '/mail', anahtarlar: 'email mailbox', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.databases.baslik'), aciklama: t('TopBar:pages.databases.aciklama'), yol: '/veritabanlari', anahtarlar: 'database mysql db', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.ssl.baslik'), aciklama: t('TopBar:pages.ssl.aciklama'), yol: '/ssl', anahtarlar: 'https lets encrypt', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.wordpress.baslik'), aciklama: t('TopBar:pages.wordpress.aciklama'), yol: '/wordpress', anahtarlar: 'wp uygulama', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.plans.baslik'), aciklama: t('TopBar:pages.plans.aciklama'), yol: '/hizmet-planlari', anahtarlar: 'paket kota', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.users.baslik'), aciklama: t('TopBar:pages.users.aciklama'), yol: '/kullanicilar', anahtarlar: 'hesap bayi admin müşteri', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.customers.baslik'), aciklama: t('TopBar:pages.customers.aciklama'), yol: '/musteriler', anahtarlar: 'customer fatura', roller: ['admin', 'reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.server_status.baslik'), aciklama: t('TopBar:pages.server_status.aciklama'), yol: '/sunucu-durumu', anahtarlar: 'cpu ram disk', roller: ['reseller'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.monitor.baslik'), aciklama: t('TopBar:pages.monitor.aciklama'), yol: '/izleme', anahtarlar: 'monitor log cpu ram', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.stats.baslik'), aciklama: t('TopBar:pages.stats.aciklama'), yol: '/istatistikler', anahtarlar: 'grafik trafik', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.services.baslik'), aciklama: t('TopBar:pages.services.aciklama'), yol: '/araclar/servisler', anahtarlar: 'systemd nginx mysql php', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.php_versions.baslik'), aciklama: t('TopBar:pages.php_versions.aciklama'), yol: '/araclar/php-surumler', anahtarlar: 'fpm', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.php_modules.baslik'), aciklama: t('TopBar:pages.php_modules.aciklama'), yol: '/sistem/php-modulleri', anahtarlar: 'extension pecl', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.packages.baslik'), aciklama: t('TopBar:pages.packages.aciklama'), yol: '/araclar/paketler', anahtarlar: 'dnf rpm', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.dns_template.baslik'), aciklama: t('TopBar:pages.dns_template.aciklama'), yol: '/araclar/dns-sablonu', anahtarlar: 'template zone', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.firewall.baslik'), aciklama: t('TopBar:pages.firewall.aciklama'), yol: '/firewall', anahtarlar: 'port ip engelle', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.security_log.baslik'), aciklama: t('TopBar:pages.security_log.aciklama'), yol: '/guvenlik-gunlugu', anahtarlar: 'audit log olay', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.security_notifications.baslik'), aciklama: t('TopBar:pages.security_notifications.aciklama'), yol: '/guvenlik-bildirimleri', anahtarlar: 'security correlation alerts', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.backup.baslik'), aciklama: t('TopBar:pages.backup.aciklama'), yol: '/backup-yonetimi', anahtarlar: 'backup geri yükle', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.transfer.baslik'), aciklama: t('TopBar:pages.transfer.aciklama'), yol: '/hesap-aktarimi', anahtarlar: 'migration transfer cpanel', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.tools_settings.baslik'), aciklama: t('TopBar:pages.tools_settings.aciklama'), yol: '/araclar-ayarlar', anahtarlar: 'settings tools', roller: ['admin'] },
    { tur: 'sayfa', baslik: t('TopBar:pages.profile.baslik'), aciklama: t('TopBar:pages.profile.aciklama'), yol: '/profil', anahtarlar: 'parola şifre 2fa tema' },
  ]
}

function domainSayfalari(t: TFunc): readonly [string, string, string][] {
  return [
    ['', t('TopBar:domain_pages.overview.baslik'), t('TopBar:domain_pages.overview.aciklama')],
    ['/dosyalar', t('TopBar:domain_pages.files.baslik'), t('TopBar:domain_pages.files.aciklama')],
    ['/web-sunucu', t('TopBar:domain_pages.webserver.baslik'), t('TopBar:domain_pages.webserver.aciklama')],
    ['/php', t('TopBar:domain_pages.php.baslik'), t('TopBar:domain_pages.php.aciklama')],
    ['/composer', t('TopBar:domain_pages.composer.baslik'), t('TopBar:domain_pages.composer.aciklama')],
    ['/performans', t('TopBar:domain_pages.performance.baslik'), t('TopBar:domain_pages.performance.aciklama')],
    ['/redis', t('TopBar:domain_pages.redis.baslik'), t('TopBar:domain_pages.redis.aciklama')],
    ['/wordpress', t('TopBar:domain_pages.wordpress.baslik'), t('TopBar:domain_pages.wordpress.aciklama')],
    ['/dns', t('TopBar:domain_pages.dns.baslik'), t('TopBar:domain_pages.dns.aciklama')],
    ['/subdomainler', t('TopBar:domain_pages.subdomains.baslik'), t('TopBar:domain_pages.subdomains.aciklama')],
    ['/ek-alanlar', t('TopBar:domain_pages.addon_domains.baslik'), t('TopBar:domain_pages.addon_domains.aciklama')],
    ['/ssl', t('TopBar:domain_pages.ssl.baslik'), t('TopBar:domain_pages.ssl.aciklama')],
    ['/veritabanlari', t('TopBar:domain_pages.databases.baslik'), t('TopBar:domain_pages.databases.aciklama')],
    ['/ftp', t('TopBar:domain_pages.ftp.baslik'), t('TopBar:domain_pages.ftp.aciklama')],
    ['/mail', t('TopBar:domain_pages.mail.baslik'), t('TopBar:domain_pages.mail.aciklama')],
    ['/yedekler', t('TopBar:domain_pages.backups.baslik'), t('TopBar:domain_pages.backups.aciklama')],
    ['/kopyala', t('TopBar:domain_pages.clone.baslik'), t('TopBar:domain_pages.clone.aciklama')],
    ['/git', t('TopBar:domain_pages.git.baslik'), t('TopBar:domain_pages.git.aciklama')],
    ['/cron', t('TopBar:domain_pages.cron.baslik'), t('TopBar:domain_pages.cron.aciklama')],
    ['/ssh-erisim', t('TopBar:domain_pages.ssh.baslik'), t('TopBar:domain_pages.ssh.aciklama')],
    ['/gunlukler', t('TopBar:domain_pages.logs.baslik'), t('TopBar:domain_pages.logs.aciklama')],
    ['/waf', t('TopBar:domain_pages.waf.baslik'), t('TopBar:domain_pages.waf.aciklama')],
    ['/rate-limit', t('TopBar:domain_pages.rate_limit.baslik'), t('TopBar:domain_pages.rate_limit.aciklama')],
    ['/erisim-kontrol', t('TopBar:domain_pages.access_control.baslik'), t('TopBar:domain_pages.access_control.aciklama')],
    ['/sifre-koruma', t('TopBar:domain_pages.password_protect.baslik'), t('TopBar:domain_pages.password_protect.aciklama')],
    ['/imunify', t('TopBar:domain_pages.imunify.baslik'), t('TopBar:domain_pages.imunify.aciklama')],
    ['/istatistik', t('TopBar:domain_pages.stats.baslik'), t('TopBar:domain_pages.stats.aciklama')],
    ['/baglanti', t('TopBar:domain_pages.connection_info.baslik'), t('TopBar:domain_pages.connection_info.aciklama')],
  ]
}

function normalize(s: string) {
  return s.toLocaleLowerCase('tr-TR').normalize('NFD').replace(/[\u0300-\u036f]/g, '')
}

function panoYaz(text: string): boolean {
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).catch(() => {})
    return true
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.top = '0'
    ta.style.left = '0'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    return true
  } catch { return false }
}

export default function TopBar({ onMenuAc, menuAcik }: { onMenuAc?: () => void; menuAcik?: boolean }) {
  const { t } = useTranslation(['TopBar', 'common'])
  const kullanici = useAuth((s) => s.kullanici)
  const cikis = useAuth((s) => s.cikis)
  const navigate = useNavigate()
  const location = useLocation()
  const aramaRef = useRef<HTMLInputElement>(null)
  const aramaKutusuRef = useRef<HTMLDivElement>(null)
  const [menuAcikProfil, setMenuAcik] = useState(false)
  const [tema, setTema] = useState<Theme>(getTheme())
  const [sunucuIp, setSunucuIp] = useState<string | null>(null)
  const [ipKopyalandi, setIpKopyalandi] = useState(false)
  const [arama, setArama] = useState('')
  const [aramaAcik, setAramaAcik] = useState(false)
  const [aramaYukleniyor, setAramaYukleniyor] = useState(false)
  const [seciliSonuc, setSeciliSonuc] = useState(0)
  const [kayitlar, setKayitlar] = useState<AramaKaydi[]>([])
  const [kayitlarYuklendi, setKayitlarYuklendi] = useState(false)
  const [bildirim, setBildirim] = useState({ acik: 0, kritik: 0 })

  const rol = kullanici?.rol || 'user'
  const domainID = location.pathname.match(/^\/abonelikler\/(\d+)/)?.[1]

  const SAYFALAR = useMemo(() => sayfalar(t), [t])
  const DOMAIN_SAYFALARI = useMemo(() => domainSayfalari(t), [t])

  useEffect(() => {
    if (rol !== 'admin') return
    const yukle = () => api.get<{ acik: number; kritik: number }>('/guvenlik-bildirimleri/ozet').then(r => setBildirim(r.data)).catch(() => {})
    void yukle(); const timer = window.setInterval(yukle, 60000); return () => window.clearInterval(timer)
  }, [rol])

  const sonuclar = useMemo(() => {
    const q = normalize(arama.trim())
    if (!q) return []
    const sayfalarFiltrelenmis = SAYFALAR.filter(s => !s.roller || s.roller.includes(rol))
    const domainSayfalariEslesen: AramaKaydi[] = domainID ? DOMAIN_SAYFALARI.map(([ek, baslik, aciklama]) => ({
      tur: 'sayfa', baslik, aciklama: `${aciklama}${t('TopBar:domain_pages.open_domain_suffix')}`, yol: `/abonelikler/${domainID}${ek}`,
      anahtarlar: 'domain site abonelik',
    })) : []
    return [...sayfalarFiltrelenmis, ...domainSayfalariEslesen, ...kayitlar]
      .map((s) => {
        const metin = normalize(`${s.baslik} ${s.aciklama} ${s.anahtarlar || ''}`)
        const puan = normalize(s.baslik).startsWith(q) ? 0 : metin.includes(q) ? 1 : 2
        return { s, puan }
      })
      .filter(x => x.puan < 2)
      .sort((a, b) => a.puan - b.puan || a.s.baslik.localeCompare(b.s.baslik, 'tr'))
      .slice(0, 12)
      .map(x => x.s)
  }, [arama, domainID, kayitlar, rol, SAYFALAR, DOMAIN_SAYFALARI, t])

  async function aramaVerileriniYukle() {
    if (kayitlarYuklendi || aramaYukleniyor) return
    setAramaYukleniyor(true)
    const istekler: Promise<AramaKaydi[]>[] = [
      api.get<Domain[]>('/domains').then(r => (Array.isArray(r.data) ? r.data : []).map(d => ({
        tur: 'domain' as const, baslik: d.alan_adi,
        aciklama: `${t('TopBar:types.domain')}${d.sistem_kullanici ? ` · ${d.sistem_kullanici}` : ''}${d.durum ? ` · ${d.durum}` : ''}`,
        yol: `/abonelikler/${d.id}`, anahtarlar: `site hosting ${d.sistem_kullanici || ''}`,
      }))).catch(() => []),
    ]
    if (rol === 'admin' || rol === 'reseller') {
      istekler.push(
        api.get<Musteri[]>('/customers').then(r => (Array.isArray(r.data) ? r.data : []).map(m => ({
          tur: 'musteri' as const, baslik: m.ad, aciklama: `${t('TopBar:types.musteri')}${m.eposta ? ` · ${m.eposta}` : ''}`,
          yol: `/musteriler?arama=${encodeURIComponent(m.eposta || m.ad)}`, anahtarlar: m.eposta,
        }))).catch(() => []),
        api.get<Kullanici[]>('/users').then(r => (Array.isArray(r.data) ? r.data : []).map(u => ({
          tur: 'kullanici' as const, baslik: u.ad_soyad || u.kullanici_adi,
          aciklama: `${t('TopBar:types.kullanici')} · ${u.kullanici_adi}${u.eposta ? ` · ${u.eposta}` : ''}`,
          yol: `/kullanicilar?arama=${encodeURIComponent(u.kullanici_adi)}`,
          anahtarlar: `${u.eposta || ''} ${u.rol || ''}`,
        }))).catch(() => []),
      )
    }
    const gelen = await Promise.all(istekler)
    setKayitlar(gelen.flat())
    setKayitlarYuklendi(true)
    setAramaYukleniyor(false)
  }

  function sonucaGit(s: AramaKaydi) {
    setArama('')
    setAramaAcik(false)
    navigate(s.yol)
  }

  useEffect(() => {
    const h = (e: Event) => setTema((e as CustomEvent<Theme>).detail)
    window.addEventListener('sanal:theme-change', h)
    return () => window.removeEventListener('sanal:theme-change', h)
  }, [])

  useEffect(() => {
    function tus(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        aramaRef.current?.focus()
        setAramaAcik(true)
        void aramaVerileriniYukle()
      }
      if (e.key === 'Escape') {
        setAramaAcik(false)
        aramaRef.current?.blur()
      }
    }
    function disari(e: MouseEvent) {
      if (!aramaKutusuRef.current?.contains(e.target as Node)) setAramaAcik(false)
    }
    window.addEventListener('keydown', tus)
    document.addEventListener('mousedown', disari)
    return () => {
      window.removeEventListener('keydown', tus)
      document.removeEventListener('mousedown', disari)
    }
  })

  useEffect(() => { setSeciliSonuc(0) }, [arama])

  useEffect(() => {
    api.get<{ sunucu_ip: string }>('/system/panel-domain')
      .then(r => setSunucuIp(r.data.sunucu_ip || null))
      .catch(() => {})
  }, [])

  function ipKopyala() {
    if (!sunucuIp) return
    panoYaz(sunucuIp)
    setIpKopyalandi(true)
    setTimeout(() => setIpKopyalandi(false), 1800)
  }

  function temaDegistir() {
    const siradaki: Theme = tema === 'light' ? 'dark' : tema === 'dark' ? 'system' : 'light'
    setTheme(siradaki)
    setTema(siradaki)
  }

  function onCikis() {
    cikis()
    navigate('/giris', { replace: true })
  }

  return (
    <header className="h-20 bg-white/95 dark:bg-[#101828]/95 backdrop-blur border-b border-slate-200 dark:border-slate-800 flex items-center px-4 sm:px-6 sticky top-0 z-30 gap-3 sm:gap-5">
      {/* Hamburger — yalnız < lg; kenar çubuğu orada çekmeceye dönüşüyor */}
      <button
        onClick={onMenuAc}
        className="lg:hidden -ml-1 p-2 text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md transition flex-shrink-0"
        aria-label={t('TopBar:menu_open')}
        aria-expanded={!!menuAcik}
        aria-controls="sp-kenar-cubugu"
      >
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
      </button>

      {/* Dar ekranda ortalama boşluğu yok; lg'de eski ortalanmış arama korunur */}
      <div className="hidden lg:block flex-1" />

      <div className="flex-1 lg:flex-none w-full lg:max-w-xl min-w-0">
        <div className="relative" ref={aramaKutusuRef}>
          <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 dark:text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            ref={aramaRef}
            type="search"
            value={arama}
            onChange={e => { setArama(e.target.value); setAramaAcik(true) }}
            onFocus={() => { setAramaAcik(true); void aramaVerileriniYukle() }}
            onKeyDown={e => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setSeciliSonuc(i => Math.max(0, Math.min(i + 1, sonuclar.length - 1))) }
              if (e.key === 'ArrowUp') { e.preventDefault(); setSeciliSonuc(i => Math.max(i - 1, 0)) }
              if (e.key === 'Enter' && sonuclar[seciliSonuc]) { e.preventDefault(); sonucaGit(sonuclar[seciliSonuc]) }
            }}
            placeholder={t('TopBar:search_placeholder')}
            aria-label={t('TopBar:search_aria')}
            role="combobox"
            aria-autocomplete="list"
            aria-expanded={aramaAcik && !!arama.trim()}
            aria-controls="global-arama-sonuclari"
            className="w-full pl-10 pr-16 py-2.5 text-sm bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl focus:bg-white dark:focus:bg-slate-900 focus:border-brand-400 focus:ring-4 focus:ring-brand-500/10 outline-none transition"
          />
          <span className="hidden sm:block absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] text-slate-400 dark:text-slate-500 border border-slate-200 dark:border-slate-700 rounded px-1.5 py-0.5 pointer-events-none">{t('TopBar:search_shortcut')}</span>
          {aramaAcik && arama.trim() && (
            <div id="global-arama-sonuclari" role="listbox" className="absolute top-full left-0 right-0 mt-2 max-h-[min(70vh,32rem)] overflow-y-auto bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-xl shadow-2xl z-50 p-1.5">
              {sonuclar.map((s, i) => (
                <button
                  key={`${s.tur}-${s.yol}-${s.baslik}`}
                  type="button"
                  role="option"
                  aria-selected={i === seciliSonuc}
                  onMouseEnter={() => setSeciliSonuc(i)}
                  onClick={() => sonucaGit(s)}
                  className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-left transition ${i === seciliSonuc ? 'bg-brand-50 dark:bg-brand-900/25' : 'hover:bg-slate-50 dark:hover:bg-slate-800'}`}
                >
                  <span className={`w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 text-sm font-semibold ${
                    s.tur === 'domain' ? 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
                    : s.tur === 'musteri' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
                    : s.tur === 'kullanici' ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300'
                    : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'
                  }`}>{s.tur === 'domain' ? 'D' : s.tur === 'musteri' ? 'M' : s.tur === 'kullanici' ? 'K' : '↗'}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium text-slate-900 dark:text-slate-100 truncate">{s.baslik}</span>
                    <span className="block text-xs text-slate-500 dark:text-slate-400 truncate">{s.aciklama}</span>
                  </span>
                  <span className="text-[10px] uppercase tracking-wide text-slate-400 dark:text-slate-500">{t(`TopBar:types.${s.tur}`)}</span>
                </button>
              ))}
              {aramaYukleniyor && <div className="px-3 py-3 text-sm text-slate-500">{t('TopBar:loading_records')}</div>}
              {!aramaYukleniyor && sonuclar.length === 0 && (
                <div className="px-3 py-6 text-center text-sm text-slate-500">{t('TopBar:no_results', { query: arama })}</div>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="flex-none lg:flex-1 flex items-center justify-end gap-0.5 sm:gap-1">
        {sunucuIp && (
          <button
            onClick={ipKopyala}
            title={t('TopBar:ip_copy_title')}
            className="hidden sm:inline-flex items-center gap-1.5 px-2 py-1.5 text-xs font-mono text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md transition"
          >
            <svg className="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
            </svg>
            {ipKopyalandi ? (
              <span className="text-emerald-600 dark:text-emerald-400 font-sans font-medium">{t('TopBar:ip_copied')}</span>
            ) : (
              <span>{sunucuIp}</span>
            )}
          </button>
        )}
        <LanguageSwitcher />
        <button onClick={temaDegistir}
          type="button"
          aria-label={t('TopBar:theme_toggle_title', { theme: tema })}
          className="p-2 text-slate-500 hover:bg-slate-100 hover:text-slate-700 dark:bg-slate-800 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-200 rounded-md transition"
          title={t('TopBar:theme_toggle_title', { theme: tema })}>
          {tema === 'dark' ? (
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
            </svg>
          ) : tema === 'light' ? (
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
            </svg>
          ) : (
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
          )}
        </button>
        <button onClick={() => navigate('/guvenlik-bildirimleri')} type="button" aria-label={t('TopBar:notifications')} className="relative hidden sm:inline-flex p-2 text-slate-500 hover:bg-slate-100 hover:text-slate-700 dark:bg-slate-800 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-200 rounded-md transition" title={t('TopBar:notifications')}>
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
          </svg>
          {bildirim.acik > 0 && <span className={`absolute -right-1 -top-1 min-w-4 rounded-full px-1 text-center text-[10px] font-bold text-white ${bildirim.kritik ? 'bg-red-600' : 'bg-amber-500'}`}>{bildirim.acik > 99 ? '99+' : bildirim.acik}</span>}
        </button>

        <div className="relative">
          <button
            type="button"
            onClick={() => setMenuAcik((v) => !v)}
            className="flex items-center gap-2 px-1.5 sm:px-2 py-1.5 hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-800 rounded-md transition"
            aria-label={t('TopBar:account_menu')}
            aria-expanded={menuAcikProfil}
          >
            <div className="w-7 h-7 rounded-full bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 font-semibold text-xs flex items-center justify-center flex-shrink-0">
              {(kullanici?.ad_soyad || kullanici?.adi || '?').slice(0, 1).toUpperCase()}
            </div>
            {/* İsim dar ekranda gizli — avatar zaten kimliği taşıyor */}
            <span className="hidden md:inline text-sm font-medium text-slate-700 dark:text-slate-300 max-w-[12rem] truncate">{kullanici?.ad_soyad || kullanici?.adi}</span>
            <svg className="hidden sm:block w-4 h-4 text-slate-400 dark:text-slate-500 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          {menuAcikProfil && (
            <>
              {/* Backdrop: dropdown'ı dışarı tıklayınca kapatır, dekoratif — gerçek menü
                  öğeleri aşağıda native <button>, zaten klavye erişilebilir. */}
              {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
              <div className="fixed inset-0 z-40" onClick={() => setMenuAcik(false)} />
              <div className="absolute right-0 mt-1 w-56 max-w-[calc(100vw-1.5rem)] bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg shadow-lg z-50 py-1">
                <div className="px-3 py-2 border-b border-slate-100 dark:border-slate-800">
                  <div className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate">{kullanici?.ad_soyad || kullanici?.adi}</div>
                  <div className="text-xs text-slate-500 dark:text-slate-500 capitalize">{kullanici?.rol}</div>
                </div>
                <button
                  onClick={() => { setMenuAcik(false); navigate('/profil') }}
                  className="w-full text-left px-3 py-2 text-sm text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800"
                >
                  {t('TopBar:profile_prefs')}
                </button>
                <div className="border-t border-slate-100 dark:border-slate-800 my-1"></div>
                <button
                  onClick={onCikis}
                  className="w-full text-left px-3 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20"
                >
                  {t('TopBar:logout')}
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  )
}
