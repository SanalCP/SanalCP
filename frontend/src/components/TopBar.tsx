// sanal-dark-swept
// sanal-dark-swept-v2
// sp-mobil-v1
import { useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '@/store/auth'
import { getTheme, setTheme, type Theme } from '@/lib/theme'
import { api } from '@/lib/api'

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

const SAYFALAR: Array<AramaKaydi & { roller?: string[] }> = [
  { tur: 'sayfa', baslik: 'Anasayfa', aciklama: 'Panel özeti', yol: '/', anahtarlar: 'dashboard genel bakış' },
  { tur: 'sayfa', baslik: 'Domainler', aciklama: 'Alan adları ve abonelikler', yol: '/domainler', anahtarlar: 'site hosting abonelik', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'DNS Yönetimi', aciklama: 'Genel DNS kayıtları', yol: '/dns', anahtarlar: 'zone nameserver ns', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'E-posta Hesapları', aciklama: 'Posta kutuları ve mail', yol: '/mail', anahtarlar: 'email mailbox', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'Veritabanları', aciklama: 'MySQL veritabanları', yol: '/veritabanlari', anahtarlar: 'database mysql db', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'SSL Sertifikaları', aciklama: 'TLS ve sertifika yönetimi', yol: '/ssl', anahtarlar: 'https lets encrypt', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'WordPress', aciklama: 'WordPress siteleri', yol: '/wordpress', anahtarlar: 'wp uygulama', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'Hizmet Planları', aciklama: 'Paket ve kota planları', yol: '/hizmet-planlari', anahtarlar: 'paket kota', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'Kullanıcılar', aciklama: 'Panel hesapları', yol: '/kullanicilar', anahtarlar: 'hesap bayi admin müşteri', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'Müşteriler', aciklama: 'Müşteri ve iletişim kayıtları', yol: '/musteriler', anahtarlar: 'customer fatura', roller: ['admin', 'reseller'] },
  { tur: 'sayfa', baslik: 'Sunucu Durumu', aciklama: 'Kaynak ve servis özeti', yol: '/sunucu-durumu', anahtarlar: 'cpu ram disk', roller: ['reseller'] },
  { tur: 'sayfa', baslik: 'Sunucu İzleme', aciklama: 'Canlı sunucu metrikleri ve günlükler', yol: '/izleme', anahtarlar: 'monitor log cpu ram', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'İstatistikler', aciklama: 'Kullanım istatistikleri', yol: '/istatistikler', anahtarlar: 'grafik trafik', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Servisler', aciklama: 'Sistem servisleri', yol: '/araclar/servisler', anahtarlar: 'systemd nginx mysql php', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'PHP Sürümleri', aciklama: 'PHP sürüm yönetimi', yol: '/araclar/php-surumler', anahtarlar: 'fpm', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'PHP Modülleri', aciklama: 'PHP eklenti yönetimi', yol: '/sistem/php-modulleri', anahtarlar: 'extension pecl', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Paket Yöneticisi', aciklama: 'Sistem paketleri', yol: '/araclar/paketler', anahtarlar: 'dnf rpm', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'DNS Şablonu', aciklama: 'Varsayılan DNS kayıtları', yol: '/araclar/dns-sablonu', anahtarlar: 'template zone', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Güvenlik Duvarı', aciklama: 'Firewall kuralları', yol: '/firewall', anahtarlar: 'port ip engelle', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Güvenlik Günlüğü', aciklama: 'Güvenlik olayları', yol: '/guvenlik-gunlugu', anahtarlar: 'audit log olay', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Yedekleme', aciklama: 'Sunucu yedek yönetimi', yol: '/backup-yonetimi', anahtarlar: 'backup geri yükle', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Hesap Aktarımı', aciklama: 'Hesap ve site taşıma', yol: '/hesap-aktarimi', anahtarlar: 'migration transfer cpanel', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Araçlar ve Ayarlar', aciklama: 'Panel araçları', yol: '/araclar-ayarlar', anahtarlar: 'settings tools', roller: ['admin'] },
  { tur: 'sayfa', baslik: 'Profil ve Tercihler', aciklama: 'Hesap ve güvenlik ayarları', yol: '/profil', anahtarlar: 'parola şifre 2fa tema' },
]

const DOMAIN_SAYFALARI = [
  ['', 'Genel Bakış', 'Domain özeti'],
  ['/dosyalar', 'Dosya Yöneticisi', 'Dosyalar ve dizinler'],
  ['/web-sunucu', 'Apache & nginx', 'Web sunucusu ayarları'],
  ['/php', 'PHP Ayarları', 'PHP ve FPM yapılandırması'],
  ['/composer', 'Composer', 'PHP paket yönetimi'],
  ['/performans', 'Performans', 'Site performans ayarları'],
  ['/redis', 'Redis Cache', 'Önbellek yönetimi'],
  ['/wordpress', 'WordPress', 'WordPress araçları'],
  ['/dns', 'DNS Yönetimi', 'Domain DNS kayıtları'],
  ['/subdomainler', 'Subdomainler', 'Alt alan adları'],
  ['/ek-alanlar', 'Ek Alan Adları', 'Alias domainler'],
  ['/ssl', 'SSL/TLS', 'HTTPS sertifikası'],
  ['/veritabanlari', 'Veritabanları', 'MySQL veritabanları'],
  ['/ftp', 'FTP Hesapları', 'FTP erişimi'],
  ['/mail', 'E-posta', 'Posta kutuları'],
  ['/yedekler', 'Yedekler', 'Domain yedekleri'],
  ['/kopyala', 'Siteyi Kopyala', 'Site klonlama'],
  ['/git', 'Git', 'Git deploy'],
  ['/cron', 'Zamanlanmış Görevler', 'Cron işleri'],
  ['/ssh-erisim', 'SSH Erişimi', 'Terminal erişimi'],
  ['/gunlukler', 'Günlükler', 'Erişim ve hata logları'],
  ['/waf', 'WAF', 'Web uygulama güvenlik duvarı'],
  ['/erisim-kontrol', 'Erişim Kontrolü', 'IP erişim kuralları'],
  ['/sifre-koruma', 'Şifre Korumalı Dizinler', 'Dizin parolası'],
  ['/imunify', 'Imunify', 'Zararlı yazılım taraması'],
  ['/istatistik', 'İstatistikler', 'Domain kullanım verileri'],
  ['/baglanti', 'Bağlantı Bilgisi', 'FTP ve veritabanı bilgileri'],
] as const

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

  const rol = kullanici?.rol || 'user'
  const domainID = location.pathname.match(/^\/abonelikler\/(\d+)/)?.[1]

  const sonuclar = useMemo(() => {
    const q = normalize(arama.trim())
    if (!q) return []
    const sayfalar = SAYFALAR.filter(s => !s.roller || s.roller.includes(rol))
    const domainSayfalari: AramaKaydi[] = domainID ? DOMAIN_SAYFALARI.map(([ek, baslik, aciklama]) => ({
      tur: 'sayfa', baslik, aciklama: `${aciklama} · açık domain`, yol: `/abonelikler/${domainID}${ek}`,
      anahtarlar: 'domain site abonelik',
    })) : []
    return [...sayfalar, ...domainSayfalari, ...kayitlar]
      .map((s) => {
        const metin = normalize(`${s.baslik} ${s.aciklama} ${s.anahtarlar || ''}`)
        const puan = normalize(s.baslik).startsWith(q) ? 0 : metin.includes(q) ? 1 : 2
        return { s, puan }
      })
      .filter(x => x.puan < 2)
      .sort((a, b) => a.puan - b.puan || a.s.baslik.localeCompare(b.s.baslik, 'tr'))
      .slice(0, 12)
      .map(x => x.s)
  }, [arama, domainID, kayitlar, rol])

  async function aramaVerileriniYukle() {
    if (kayitlarYuklendi || aramaYukleniyor) return
    setAramaYukleniyor(true)
    const istekler: Promise<AramaKaydi[]>[] = [
      api.get<Domain[]>('/domains').then(r => (Array.isArray(r.data) ? r.data : []).map(d => ({
        tur: 'domain' as const, baslik: d.alan_adi,
        aciklama: `Domain${d.sistem_kullanici ? ` · ${d.sistem_kullanici}` : ''}${d.durum ? ` · ${d.durum}` : ''}`,
        yol: `/abonelikler/${d.id}`, anahtarlar: `site hosting ${d.sistem_kullanici || ''}`,
      }))).catch(() => []),
    ]
    if (rol === 'admin' || rol === 'reseller') {
      istekler.push(
        api.get<Musteri[]>('/customers').then(r => (Array.isArray(r.data) ? r.data : []).map(m => ({
          tur: 'musteri' as const, baslik: m.ad, aciklama: `Müşteri${m.eposta ? ` · ${m.eposta}` : ''}`,
          yol: `/musteriler?arama=${encodeURIComponent(m.eposta || m.ad)}`, anahtarlar: m.eposta,
        }))).catch(() => []),
        api.get<Kullanici[]>('/users').then(r => (Array.isArray(r.data) ? r.data : []).map(u => ({
          tur: 'kullanici' as const, baslik: u.ad_soyad || u.kullanici_adi,
          aciklama: `Kullanıcı · ${u.kullanici_adi}${u.eposta ? ` · ${u.eposta}` : ''}`,
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
    <header className="h-14 bg-white dark:bg-slate-800 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700 dark:border-slate-800 flex items-center px-3 sm:px-4 sticky top-0 z-30 gap-2 sm:gap-4">
      {/* Hamburger — yalnız < lg; kenar çubuğu orada çekmeceye dönüşüyor */}
      <button
        onClick={onMenuAc}
        className="lg:hidden -ml-1 p-2 text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md transition flex-shrink-0"
        aria-label="Menüyü aç"
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
            placeholder="Her şeyi ara..."
            aria-label="Panelde ara"
            aria-expanded={aramaAcik && !!arama.trim()}
            aria-controls="global-arama-sonuclari"
            className="w-full pl-9 pr-16 py-1.5 text-sm bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg focus:bg-white dark:focus:bg-slate-800 focus:border-brand-400 focus:ring-2 focus:ring-brand-500/15 outline-none transition"
          />
          <span className="hidden sm:block absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] text-slate-400 dark:text-slate-500 border border-slate-200 dark:border-slate-700 rounded px-1.5 py-0.5 pointer-events-none">Ctrl K</span>
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
                  <span className="text-[10px] uppercase tracking-wide text-slate-400 dark:text-slate-500">{s.tur}</span>
                </button>
              ))}
              {aramaYukleniyor && <div className="px-3 py-3 text-sm text-slate-500">Kayıtlar yükleniyor…</div>}
              {!aramaYukleniyor && sonuclar.length === 0 && (
                <div className="px-3 py-6 text-center text-sm text-slate-500">“{arama}” için sonuç bulunamadı.</div>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="flex-none lg:flex-1 flex items-center justify-end gap-0.5 sm:gap-1">
        {sunucuIp && (
          <button
            onClick={ipKopyala}
            title="Tıkla → kopyala"
            className="hidden sm:inline-flex items-center gap-1.5 px-2 py-1.5 text-xs font-mono text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md transition"
          >
            <svg className="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
            </svg>
            {ipKopyalandi ? (
              <span className="text-emerald-600 dark:text-emerald-400 font-sans font-medium">✓ Kopyalandı</span>
            ) : (
              <span>{sunucuIp}</span>
            )}
          </button>
        )}
        <button onClick={temaDegistir}
          className="p-2 text-slate-500 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300 hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-800 dark:text-slate-400 dark:text-slate-500 dark:hover:text-slate-200 dark:hover:bg-slate-800 rounded-md transition"
          title={`Tema: ${tema} — tıkla değiştir`}>
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
        <button className="hidden sm:inline-flex p-2 text-slate-500 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300 hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-800 dark:text-slate-400 dark:text-slate-500 dark:hover:text-slate-200 dark:hover:bg-slate-800 rounded-md transition" title="Bildirimler">
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
          </svg>
        </button>

        <div className="relative">
          <button
            onClick={() => setMenuAcik((v) => !v)}
            className="flex items-center gap-2 px-1.5 sm:px-2 py-1.5 hover:bg-slate-100 dark:bg-slate-800 dark:hover:bg-slate-800 rounded-md transition"
            aria-label="Hesap menüsü"
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
                  Profil ve Tercihler
                </button>
                <div className="border-t border-slate-100 dark:border-slate-800 my-1"></div>
                <button
                  onClick={onCikis}
                  className="w-full text-left px-3 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20"
                >
                  Çıkış Yap
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  )
}
