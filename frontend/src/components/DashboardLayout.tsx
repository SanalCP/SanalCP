// sanal-dark-swept
// sanal-dark-swept-v2
// sp-mobil-v1
import { Suspense, useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { api } from '@/lib/api'
import { useAuth } from '@/store/auth'
import TopBar from './TopBar'
import AltNavBar from './AltNavBar'
import DomainSecici from './DomainSecici'

const SURUM_UYARI_KAPALI_KEY = 'sp-surum-duyuru-kapatildi'
const MENU_KAPALI_GRUP_KEY = 'sp-menu-kapali-gruplar'
type SurumKontrol = { guncelleme_var: boolean; kritik: boolean; duyuru: string; son: string; mevcut?: string; build_tarihi?: string }
type SurumBilgi = { mevcut?: string; build_tarihi?: string }

type NavItem = { to: string; etiket: string; ikon: string; end?: boolean }
// baslikKey: dil değişse de kararlı kalan iç anahtar (kapalı-grup state'i ve
// localStorage bunu kullanır); baslik ise ekrana basılan çevrilmiş metin.
type NavGroup = { baslik?: string; baslikKey?: string; items: NavItem[] }

const ICONS = {
  home:        'M3 12l2-2 7-7 7 7 2 2v8a2 2 0 01-2 2h-3v-7H10v7H7a2 2 0 01-2-2v-8z',
  musteri:     'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z',
  bayi:        'M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z',
  domain:      'M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
  abonelik:    'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2',
  plan:        'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
  araclar:     'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.827 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.99.601 2.295.247 2.572-1.065zM15 12a3 3 0 11-6 0 3 3 0 016 0z',
  istatistik:  'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
  eklenti:     'M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4',
  wp:          'M12 2C6.477 2 2 6.477 2 12s4.477 10 10 10 10-4.477 10-10S17.523 2 12 2zm0 18a8 8 0 110-16 8 8 0 010 16z',
  izleme:      'M3 12l3-3 3 6 4-9 3 6h5',
  profil:      'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z',
  kilit:       'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z',
  firewall:    'M9 12l2 2 4-4m3 2c0 6-8 10-8 10S4 18 4 12V5l8-3 8 3v7z',
  // Sunucu grubu
  servis:      'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15',
  php:         'M12 14l9-5-9-5-9 5 9 5zm0 0l6.16-3.422a12.083 12.083 0 01.665 6.479A11.952 11.952 0 0012 20.055a11.952 11.952 0 00-6.824-2.998 12.078 12.078 0 01.665-6.479L12 14z',
  puzzle:      'M11 4a2 2 0 114 0v1a1 1 0 001 1h3a1 1 0 011 1v3a1 1 0 01-1 1h-1a2 2 0 100 4h1a1 1 0 011 1v3a1 1 0 01-1 1h-3a1 1 0 01-1-1v-1a2 2 0 10-4 0v1a1 1 0 01-1 1H7a1 1 0 01-1-1v-3a1 1 0 00-1-1H4a2 2 0 110-4h1a1 1 0 001-1V7a1 1 0 011-1h3a1 1 0 001-1V4z',
  paket:       'M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4',
  yedek:       'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1M16 12l-4 4-4-4M12 16V4',
  // Domain kipi
  dns:         'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01',
  dosyalar:    'M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V7z',
  db:          'M4 7c0-1.657 3.582-3 8-3s8 1.343 8 3-3.582 3-8 3-8-1.343-8-3zm0 0v10c0 1.657 3.582 3 8 3s8-1.343 8-3V7M4 12c0 1.657 3.582 3 8 3s8-1.343 8-3',
  ftp:         'M3 16V8a2 2 0 012-2h6l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2zM9 12l3-3 3 3M12 9v6',
  posta:       'M3 8l9 6 9-6m-9 6V4m0 0v16',
  apache:      'M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4',
  composer:    'M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3v6M9 12h6',
  redis:       'M13 10V3L4 14h7v7l9-11h-7z',
  subdomain:   'M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064',
  ekdomain:    'M9 12h6m-6 4h3m-3-8h6M5 6h14a1 1 0 011 1v10a1 1 0 01-1 1H5a1 1 0 01-1-1V7a1 1 0 011-1z',
  kopya:       'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z',
  iceaktarim:  'M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 4v12m0 0l-4-4m4 4l4-4',
  git:         'M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1',
  cron:        'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
  ssh:         'M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z',
  log:         'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
  waf:         'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
  erisim:      'M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z',
  imunify:     'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622',
}

// Sunucu kipi menüsü. Buradaki her giriş App.tsx'te tanımlı bir rotaya bakar —
// daha önce yalnız "Araçlar ve Ayarlar" hub sayfasından ulaşılabilen Servisler,
// PHP, Paket Yöneticisi, Yedekleme ve DNS Şablonu artık doğrudan menüde.
function nav(t: TFunction): NavGroup[] {
  return [
  { items: [{ to: '/', etiket: t('DashboardLayout:labels.home'), ikon: ICONS.home, end: true }] },
  { baslik: t('DashboardLayout:groups.hosting'), baslikKey: 'hosting', items: [
    { to: '/domainler',            etiket: t('DashboardLayout:items.domains'),       ikon: ICONS.domain },
    { to: '/dns',                  etiket: t('DashboardLayout:items.dns_management'), ikon: ICONS.dns },
    { to: '/mail',                 etiket: t('DashboardLayout:items.mail_accounts'), ikon: ICONS.posta },
    { to: '/veritabanlari',        etiket: t('DashboardLayout:items.databases'),     ikon: ICONS.db },
    { to: '/araclar/dns-sablonu',  etiket: t('DashboardLayout:items.dns_template'),  ikon: ICONS.ekdomain },
    { to: '/hizmet-planlari',      etiket: t('DashboardLayout:items.service_plans'), ikon: ICONS.plan },
  ]},
  { baslik: t('DashboardLayout:groups.apps'), baslikKey: 'apps', items: [
    { to: '/uygulamalar',          etiket: t('DashboardLayout:items.apps'),          ikon: ICONS.wp },
    { to: '/eklentiler',           etiket: t('DashboardLayout:items.plugins'),       ikon: ICONS.eklenti },
  ]},
  { baslik: t('DashboardLayout:groups.security'), baslikKey: 'security', items: [
    { to: '/ssl',                  etiket: t('DashboardLayout:items.ssl_certificates'), ikon: ICONS.kilit },
    { to: '/firewall',             etiket: t('DashboardLayout:items.firewall'),      ikon: ICONS.firewall },
  ]},
  { baslik: t('DashboardLayout:groups.server'), baslikKey: 'server', items: [
    { to: '/araclar/servisler',    etiket: t('DashboardLayout:items.services'),      ikon: ICONS.servis },
    { to: '/araclar/php-surumler', etiket: t('DashboardLayout:items.php_versions'),  ikon: ICONS.php },
    { to: '/sistem/php-modulleri', etiket: t('DashboardLayout:items.php_modules'),   ikon: ICONS.puzzle },
    { to: '/araclar/paketler',     etiket: t('DashboardLayout:items.package_manager'), ikon: ICONS.paket },
    { to: '/backup-yonetimi',      etiket: t('DashboardLayout:items.backup_management'), ikon: ICONS.yedek },
  ]},
  { baslik: t('DashboardLayout:groups.monitoring'), baslikKey: 'monitoring', items: [
    { to: '/izleme',               etiket: t('DashboardLayout:items.server_monitoring'), ikon: ICONS.izleme },
    { to: '/istatistikler',        etiket: t('DashboardLayout:items.statistics'),    ikon: ICONS.istatistik },
  ]},
  { baslik: t('DashboardLayout:groups.management'), baslikKey: 'management', items: [
    { to: '/kullanicilar',         etiket: t('DashboardLayout:items.users'),         ikon: ICONS.bayi },
    { to: '/bayi-paketleri',       etiket: t('DashboardLayout:items.reseller_packages'), ikon: ICONS.plan },
    { to: '/musteriler',           etiket: t('DashboardLayout:items.customers'),     ikon: ICONS.musteri },
    { to: '/hesap-aktarimi',       etiket: t('DashboardLayout:items.account_transfer'), ikon: ICONS.kopya },
    { to: '/guvenlik-gunlugu',     etiket: t('DashboardLayout:items.security_log'),  ikon: ICONS.log },
    { to: '/guvenlik-bildirimleri', etiket: t('DashboardLayout:items.security_notifications'), ikon: ICONS.kilit },
    { to: '/araclar-ayarlar',      etiket: t('DashboardLayout:items.tools_settings'), ikon: ICONS.araclar },
  ]},
  ]
}

// Bayi menüsü — YALNIZ bayinin gerçekten erişebildiği yerler.
//
// Faz 5D'de kapsam filtreleri (middleware.KapsamSQL) gelince domain listeleri
// bayiye açıldı: bayi artık kendi müşterilerinin domainlerini, DNS/SSL/e-posta
// ve veritabanı özetlerini görüyor. Sunucuyu DEĞİŞTİREN uçlar (servis işlem,
// paket kurulumu, firewall, panel ayarları) hâlâ yalnız admin'de.
function bayiNav(t: TFunction): NavGroup[] {
  return [
  { items: [{ to: '/', etiket: t('DashboardLayout:labels.home'), ikon: ICONS.home, end: true }] },
  { baslik: t('DashboardLayout:groups.hosting'), baslikKey: 'hosting', items: [
    { to: '/domainler',      etiket: t('DashboardLayout:items.domains'),   ikon: ICONS.domain },
    { to: '/dns',            etiket: t('DashboardLayout:items.dns_management'), ikon: ICONS.dns },
    { to: '/mail',           etiket: t('DashboardLayout:items.mail'),      ikon: ICONS.posta },
    { to: '/veritabanlari',  etiket: t('DashboardLayout:items.databases'), ikon: ICONS.db },
    { to: '/ssl',            etiket: t('DashboardLayout:items.ssl_certificates'), ikon: ICONS.kilit },
  ]},
  { baslik: t('DashboardLayout:groups.apps'), baslikKey: 'apps', items: [
    { to: '/uygulamalar',    etiket: t('DashboardLayout:items.apps'), ikon: ICONS.wp },
  ]},
  { baslik: t('DashboardLayout:groups.my_accounts'), baslikKey: 'my_accounts', items: [
    { to: '/bayi-ozet',      etiket: t('DashboardLayout:items.resource_summary'), ikon: ICONS.istatistik },
    { to: '/kullanicilar',   etiket: t('DashboardLayout:items.customer_accounts'), ikon: ICONS.bayi },
    { to: '/musteriler',     etiket: t('DashboardLayout:items.customer_records'), ikon: ICONS.musteri },
  ]},
  { baslik: t('DashboardLayout:groups.server'), baslikKey: 'server', items: [
    { to: '/sunucu-durumu',   etiket: t('DashboardLayout:items.server_status'),  ikon: ICONS.izleme },
    { to: '/hizmet-planlari', etiket: t('DashboardLayout:items.service_plans'), ikon: ICONS.plan },
  ]},
  ]
}

// Domain kipi menüsü — /abonelikler/:id/* altındayken kenar çubuğunun tamamı
// buna dönüşür. Karşılığı olan sayfaların hepsi zaten yazılmıştı; tek eksik
// menüye bağlanmalarıydı (DNS'e ulaşmak üç tık sürüyordu).
function domainNav(id: string, t: TFunction): NavGroup[] {
  const y = (s = '') => `/abonelikler/${id}${s}`
  return [
    { items: [{ to: y(), etiket: t('DashboardLayout:labels.overview'), ikon: ICONS.home, end: true }] },
    { baslik: t('DashboardLayout:groups.website'), baslikKey: 'website', items: [
      { to: y('/dosyalar'),      etiket: t('DashboardLayout:items.files'),        ikon: ICONS.dosyalar },
      { to: y('/web-sunucu'),    etiket: t('DashboardLayout:items.apache_nginx'), ikon: ICONS.apache },
      { to: y('/php'),           etiket: t('DashboardLayout:items.php_settings'), ikon: ICONS.php },
      { to: y('/composer'),      etiket: t('DashboardLayout:items.composer'),     ikon: ICONS.composer },
      { to: y('/performans'),    etiket: t('DashboardLayout:items.performance'),  ikon: ICONS.izleme },
      { to: y('/redis'),         etiket: t('DashboardLayout:items.redis_cache'),  ikon: ICONS.redis },
      { to: y('/uygulamalar'),   etiket: t('DashboardLayout:items.apps'),         ikon: ICONS.wp },
    ]},
    { baslik: t('DashboardLayout:groups.domain_name'), baslikKey: 'domain_name', items: [
      { to: y('/dns'),           etiket: t('DashboardLayout:items.dns_management'), ikon: ICONS.dns },
      { to: y('/subdomainler'),  etiket: t('DashboardLayout:items.subdomains'),   ikon: ICONS.subdomain },
      { to: y('/ek-alanlar'),    etiket: t('DashboardLayout:items.addon_domains'), ikon: ICONS.ekdomain },
      { to: y('/ssl'),           etiket: t('DashboardLayout:items.ssl_tls'),      ikon: ICONS.kilit },
    ]},
    { baslik: t('DashboardLayout:groups.data'), baslikKey: 'data', items: [
      { to: y('/veritabanlari'), etiket: t('DashboardLayout:items.databases'),    ikon: ICONS.db },
      { to: y('/ftp'),           etiket: t('DashboardLayout:items.ftp_accounts'), ikon: ICONS.ftp },
      { to: y('/mail'),          etiket: t('DashboardLayout:items.mail'),         ikon: ICONS.posta },
      { to: y('/yedekler'),      etiket: t('DashboardLayout:items.backups'),      ikon: ICONS.yedek },
      { to: y('/kopyala'),       etiket: t('DashboardLayout:items.clone_site'),   ikon: ICONS.kopya },
      { to: y('/ice-aktarim'),   etiket: t('DashboardLayout:items.import'),       ikon: ICONS.iceaktarim },
    ]},
    { baslik: t('DashboardLayout:groups.developer'), baslikKey: 'developer', items: [
      { to: y('/git'),           etiket: t('DashboardLayout:items.git'),          ikon: ICONS.git },
      { to: y('/cron'),          etiket: t('DashboardLayout:items.cron_jobs'),    ikon: ICONS.cron },
      { to: y('/ssh-erisim'),    etiket: t('DashboardLayout:items.ssh_access'),   ikon: ICONS.ssh },
      { to: y('/gunlukler'),     etiket: t('DashboardLayout:items.logs'),         ikon: ICONS.log },
    ]},
    { baslik: t('DashboardLayout:groups.security'), baslikKey: 'security', items: [
      { to: y('/waf'),           etiket: t('DashboardLayout:items.waf'),          ikon: ICONS.waf },
      { to: y('/rate-limit'),    etiket: t('DashboardLayout:items.rate_limit'),   ikon: ICONS.waf },
      { to: y('/erisim-kontrol'), etiket: t('DashboardLayout:items.access_control'), ikon: ICONS.erisim },
      { to: y('/sifre-koruma'),  etiket: t('DashboardLayout:items.password_protected_dirs'), ikon: ICONS.kilit },
      { to: y('/imunify'),       etiket: t('DashboardLayout:items.imunify'),      ikon: ICONS.imunify },
    ]},
    { baslik: t('DashboardLayout:groups.analytics'), baslikKey: 'analytics', items: [
      { to: y('/istatistik'),    etiket: t('DashboardLayout:items.statistics'),   ikon: ICONS.istatistik },
      { to: y('/baglanti'),      etiket: t('DashboardLayout:items.connection_info'), ikon: ICONS.plan },
    ]},
  ]
}

export default function DashboardLayout() {
  const { t } = useTranslation(['DashboardLayout'])
  const isMusteri = typeof window !== 'undefined' && localStorage.getItem('sanalcp.musteri') === '1'
  const musteriDomainID = typeof window !== 'undefined' ? localStorage.getItem('sanalcp.musteri.domain_id') || '' : ''
  const rol = useAuth((s) => s.kullanici?.rol)

  // Gruplar varsayılan olarak açıktır; yalnız kullanıcının kapattıkları saklanır.
  // Böylece menüye yeni grup eklendiğinde ayrıca kayıt gerekmez.
  const [kapaliGruplar, setKapaliGruplar] = useState<string[]>(() => {
    try {
      const ham = localStorage.getItem(MENU_KAPALI_GRUP_KEY)
      return ham ? (JSON.parse(ham) as string[]) : []
    } catch {
      return []
    }
  })

  // Mobil kenar çubuğu (off-canvas). lg ve üstünde sidebar zaten sabit görünür,
  // bu durum yalnızca < lg genişliklerde anlam taşır.
  const [mobilAcik, setMobilAcik] = useState(false)
  const konum = useLocation()

  // Sürüm güncelleme çubuğu — günde bir kez sunucu tarafında kontrol edilen
  // surum.json'dan gelir (bkz. internal/system/surumkontrol.go). Hem kritik
  // güvenlik duyuruları hem sıradan güncellemeler burada, tek satırlık
  // özetle gösterilir (detay sağdaki okla açılır); rutin sürüm bilgisi
  // Araçlar → Panel Güncelleme kartında ayrıca var. Kapatma, aynı duyuru
  // metni için kalıcıdır — yeni bir duyuru gelirse (anahtar değişir)
  // otomatik tekrar gösterilir.
  const [surum, setSurum] = useState<SurumKontrol | null>(null)
  const [duyuruKapali, setDuyuruKapali] = useState(false)
  const [duyuruGenis, setDuyuruGenis] = useState(false)
  useEffect(() => {
    api.get<SurumKontrol>('/system/surum-kontrol')
      .then((r) => {
        setSurum(r.data)
        const anahtar = `${r.data.son}:${r.data.duyuru}`
        setDuyuruKapali(localStorage.getItem(SURUM_UYARI_KAPALI_KEY) === anahtar)
      })
      .catch(() => {}) // yalnız bayi/admin — müşteride 403 beklenir, footer buna bağlı değil
  }, [])

  // Footer sürüm bilgisi — rol farkı gözetmeksizin (müşteri dahil) tüm oturum
  // açmış kullanıcılara açık uçtan (bkz. internal/system/surumkontrol.go
  // SurumBilgi). /system/surum-kontrol'ün aksine dış duyuru verisi taşımaz.
  const [surumBilgi, setSurumBilgi] = useState<SurumBilgi | null>(null)
  useEffect(() => {
    api.get<SurumBilgi>('/system/surum').then((r) => setSurumBilgi(r.data)).catch(() => {})
  }, [])

  // Rota değişince çekmeceyi kapat (link tıklamasında da onClick kapatıyor;
  // bu, geri/ileri gezinmesini de kapsayan güvenli ağ).
  useEffect(() => { setMobilAcik(false) }, [konum.pathname])

  // Çekmece açıkken Esc ile kapat + arka plan kaydırmasını kilitle
  useEffect(() => {
    if (!mobilAcik) return
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') setMobilAcik(false) }
    window.addEventListener('keydown', onKey)
    const eskiOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = eskiOverflow
    }
  }, [mobilAcik])

  // Müşteri navigasyonu — sadece kendi domain'i. Öğe kümesi öncekiyle aynı
  // (yetkisi olmayan araçlar burada yok), yalnız gruplandı.
  const my = (s = '') => `/abonelikler/${musteriDomainID}${s}`
  const MUSTERI_NAV: NavGroup[] = [
    { items: [{ to: my(), etiket: t('DashboardLayout:labels.overview'), ikon: ICONS.home, end: true }] },
    { baslik: t('DashboardLayout:groups.website'), baslikKey: 'website', items: [
      { to: my('/dosyalar'),      etiket: t('DashboardLayout:items.file_manager'), ikon: ICONS.dosyalar },
      { to: my('/web-sunucu'),    etiket: t('DashboardLayout:items.apache_nginx'), ikon: ICONS.apache },
      { to: my('/php'),           etiket: t('DashboardLayout:items.php_settings'), ikon: ICONS.php },
    ]},
    { baslik: t('DashboardLayout:groups.domain_name'), baslikKey: 'domain_name', items: [
      { to: my('/dns'),           etiket: t('DashboardLayout:items.dns_settings'), ikon: ICONS.dns },
      { to: my('/ssl'),           etiket: t('DashboardLayout:items.ssl_tls'),      ikon: ICONS.kilit },
    ]},
    { baslik: t('DashboardLayout:groups.data'), baslikKey: 'data', items: [
      { to: my('/veritabanlari'), etiket: t('DashboardLayout:items.databases'),    ikon: ICONS.db },
      { to: my('/ftp'),           etiket: t('DashboardLayout:items.ftp_accounts'), ikon: ICONS.ftp },
      { to: my('/yedekler'),      etiket: t('DashboardLayout:items.backups'),      ikon: ICONS.yedek },
    ]},
    { baslik: t('DashboardLayout:groups.developer'), baslikKey: 'developer', items: [
      { to: my('/cron'),          etiket: t('DashboardLayout:items.cron_jobs'),    ikon: ICONS.cron },
      { to: my('/git'),           etiket: t('DashboardLayout:items.git_deploy'),   ikon: ICONS.git },
      { to: my('/gunlukler'),     etiket: t('DashboardLayout:items.logs'),         ikon: ICONS.log },
    ]},
  ]

  // Domain kipi: /abonelikler/:id/* altındayken kenar çubuğu o alan adının
  // araçlarına dönüşür. Müşteri zaten tek domaine bağlı olduğu için kendi
  // menüsünü korur (ve alan adı seçici görmez).
  const domainEslesme = konum.pathname.match(/^\/abonelikler\/(\d+)/)
  const aktifDomainID = domainEslesme?.[1] ?? ''
  // Bayi de domain kipine girebilir (kendi müşterisinin domaini); yalnız
  // müşteri kendi sabit menüsünde kalır.
  const domainKipi = !isMusteri && aktifDomainID !== ''

  // Menü rolden türetilir. isMusteri, /cp'den giren oturumun localStorage
  // bayrağıdır ve rol='user' /me yanıtı gelmeden önce de bilinir; ikisi de
  // aynı müşteri menüsünü gösterir.
  const aktifNav = isMusteri || rol === 'user'
    ? MUSTERI_NAV
    : domainKipi
    ? domainNav(aktifDomainID, t)
    : rol === 'reseller'
    ? bayiNav(t)
    : nav(t)

  const grupAcik = (b: string) => !kapaliGruplar.includes(b)

  function toggle(b: string) {
    setKapaliGruplar((s) => {
      const yeni = s.includes(b) ? s.filter((x) => x !== b) : [...s, b]
      try { localStorage.setItem(MENU_KAPALI_GRUP_KEY, JSON.stringify(yeni)) } catch { /* yok say */ }
      return yeni
    })
  }

  return (
    <div className="min-h-screen flex items-start bg-[#f9fafb] dark:bg-[#101828]">
      <a href="#sp-main-content" className="skip-link">Ana içeriğe geç</a>
      {/* Mobil perde — yalnız çekmece açıkken ve < lg genişlikte */}
      {mobilAcik && (
        <div
          className="fixed inset-0 z-40 bg-slate-900/50 lg:hidden"
          onClick={() => setMobilAcik(false)}
          aria-hidden
        />
      )}

      {/*
        < lg : ekran dışına kaydırılmış sabit çekmece (hamburger ile açılır)
        >= lg: eski davranış — akışta duran yapışkan kenar çubuğu
      */}
      <aside
        id="sp-kenar-cubugu"
        className={`fixed inset-y-0 left-0 z-50 w-[290px] bg-white dark:bg-[#101828] border-r border-slate-200 dark:border-slate-800 flex flex-col flex-shrink-0 h-screen transform transition-transform duration-200 ease-out ${
          mobilAcik ? 'translate-x-0' : '-translate-x-full'
        } lg:sticky lg:top-0 lg:bottom-auto lg:left-auto lg:z-20 lg:w-[290px] lg:translate-x-0 lg:self-start`}
      >
        <div className="relative flex h-20 items-center justify-center border-b border-slate-100 px-4 dark:border-slate-800">
          <Link to="/" onClick={() => setMobilAcik(false)} className="flex items-center rounded-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500">
            <img
              src="/brand/sanalcp-logo-light.png"
              alt="SanalCP"
              className="h-16 w-auto max-w-[230px] object-contain dark:hidden"
            />
            <img
              src="/brand/sanalcp-logo-dark.png"
              alt="SanalCP"
              className="hidden h-16 w-auto max-w-[230px] object-contain dark:block"
            />
          </Link>
          <button
            onClick={() => setMobilAcik(false)}
            className="absolute right-3 p-2 text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 rounded-md transition lg:hidden"
            aria-label={t('DashboardLayout:close_menu_aria')}
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {domainKipi && (
          <div className="border-b border-slate-200 dark:border-slate-800 pb-2">
            <NavLink
              to="/domainler"
              onClick={() => setMobilAcik(false)}
              className="flex items-center gap-1.5 px-4 pt-2.5 text-xs text-slate-500 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200 transition"
            >
              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
              </svg>
              {t('DashboardLayout:server_management_back')}
            </NavLink>
            <DomainSecici aktifID={aktifDomainID} />
          </div>
        )}

        <nav className="flex-1 px-4 py-5 overflow-y-auto">
          {aktifNav.map((grup, gi) => (
            <div key={gi} className="mb-2">
              {grup.baslik && (
                <button
                  type="button"
                  onClick={() => toggle(grup.baslikKey ?? grup.baslik!)}
                  aria-expanded={grupAcik(grup.baslikKey ?? grup.baslik!)}
                  className="w-full flex items-center justify-between px-3 py-2 mt-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-400 dark:text-slate-500 hover:text-slate-600 dark:hover:text-slate-300 transition"
                >
                  <span>{grup.baslik}</span>
                  <svg
                    className={`w-3 h-3 transition-transform ${grupAcik(grup.baslikKey ?? grup.baslik!) ? '' : '-rotate-90'}`}
                    fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
              )}
              {(!grup.baslik || grupAcik(grup.baslikKey ?? grup.baslik)) && (
                <ul className="space-y-0.5">
                  {grup.items.map((it) => {
                    const ustPath = grup.items.some(
                      (it2) => it2.to !== it.to && it2.to.startsWith(it.to + '/')
                    )
                    return (
                    <li key={it.to}>
                      <NavLink
                        to={it.to}
                        end={it.end || it.to === '/' || ustPath}
                        onClick={() => setMobilAcik(false)}
                        className={({ isActive }) =>
                          `group relative flex items-center px-3 py-2.5 rounded-xl text-sm transition-all duration-150 ${
                            isActive
                              ? 'bg-brand-50 dark:bg-brand-500/15 text-brand-700 dark:text-brand-300 font-semibold'
                              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800/60 hover:text-slate-900 dark:hover:text-slate-100'
                          }`
                        }
                      >
                        {({ isActive }) => (
                          <>
                            {isActive && (
                              <span className="absolute left-0 top-2 bottom-2 w-1 rounded-r-full bg-brand-500" aria-hidden />
                            )}
                            <svg className={`w-4 h-4 mr-2.5 flex-shrink-0 transition ${
                              isActive ? 'text-brand-600 dark:text-brand-400' : 'text-slate-400 dark:text-slate-500 group-hover:text-slate-600 dark:group-hover:text-slate-300'
                            }`} fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.7}>
                              <path strokeLinecap="round" strokeLinejoin="round" d={it.ikon} />
                            </svg>
                            <span className="truncate">{it.etiket}</span>
                          </>
                        )}
                      </NavLink>
                    </li>
                  )})}
                </ul>
              )}
            </div>
          ))}
        </nav>

        {/* Profil, menü uzunluğundan bağımsız olsun diye kenar çubuğunun dibine sabit */}
        <div className="border-t border-slate-100 dark:border-slate-800 px-4 py-4">
          <NavLink
            to="/profil"
            onClick={() => setMobilAcik(false)}
            className={({ isActive }) =>
              `group flex items-center px-3 py-2 lg:py-1.5 rounded-lg text-sm transition-all duration-150 ${
                isActive
                  ? 'bg-slate-100 dark:bg-slate-800 text-slate-900 dark:text-slate-100 font-medium'
                  : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800/60 hover:text-slate-900 dark:hover:text-slate-100'
              }`
            }
          >
            {({ isActive }) => (
              <>
                <svg className={`w-4 h-4 mr-2.5 flex-shrink-0 transition ${
                  isActive ? 'text-brand-600 dark:text-brand-400' : 'text-slate-400 dark:text-slate-500 group-hover:text-slate-600 dark:group-hover:text-slate-300'
                }`} fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.7}>
                  <path strokeLinecap="round" strokeLinejoin="round" d={ICONS.profil} />
                </svg>
                <span className="truncate">{t('DashboardLayout:profile_prefs')}</span>
              </>
            )}
          </NavLink>
        </div>
      </aside>

      <div className="flex-1 flex flex-col min-w-0">
        <TopBar onMenuAc={() => setMobilAcik(true)} menuAcik={mobilAcik} />

        {surum?.guncelleme_var && !duyuruKapali && (
          <div className={`border-b px-4 py-2 text-sm ${
            surum.kritik
              ? 'border-red-700 bg-red-600 text-white'
              : 'border-brand-200 bg-brand-50 text-brand-900 dark:border-brand-800/60 dark:bg-brand-900/20 dark:text-brand-100'
          }`}>
            <div className="flex items-start gap-3">
              <svg className="mt-0.5 h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                {surum.kritik
                  ? <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0 3.75h.008M10.363 3.591 2.257 17.657a1.5 1.5 0 0 0 1.302 2.25h16.882a1.5 1.5 0 0 0 1.302-2.25L13.638 3.591a1.5 1.5 0 0 0-2.598 0Z" />
                  : <path strokeLinecap="round" strokeLinejoin="round" d="M9 13.5 12 16.5 22 6.5M2 12a10 10 0 1 1 6.6 9.4" />}
              </svg>
              <div className="min-w-0 flex-1">
                <div className="truncate font-semibold">
                  {surum.kritik
                    ? t('DashboardLayout:critical_announcement_prefix', { version: surum.son })
                    : t('DashboardLayout:update_available_prefix', { version: surum.son })}
                </div>
                <div className={`text-xs ${surum.kritik ? 'text-red-100' : 'text-brand-700 dark:text-brand-300'} ${duyuruGenis ? '' : 'truncate'}`}>
                  {surum.duyuru || t('DashboardLayout:version_update_recommended')}
                </div>
              </div>
              {surum.duyuru && (
                <button
                  type="button"
                  onClick={() => setDuyuruGenis((v) => !v)}
                  aria-expanded={duyuruGenis}
                  aria-label={duyuruGenis ? t('DashboardLayout:collapse_announcement_aria') : t('DashboardLayout:expand_announcement_aria')}
                  className={`shrink-0 -m-1 rounded-md p-1 ${surum.kritik ? 'hover:bg-red-700' : 'hover:bg-brand-100 dark:hover:bg-brand-900/30'}`}
                >
                  <svg className={`h-4 w-4 transition-transform ${duyuruGenis ? 'rotate-180' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
              )}
              <button
                type="button"
                onClick={() => {
                  localStorage.setItem(SURUM_UYARI_KAPALI_KEY, `${surum.son}:${surum.duyuru}`)
                  setDuyuruKapali(true)
                }}
                className={`shrink-0 -m-1 rounded-md p-1 ${surum.kritik ? 'hover:bg-red-700' : 'hover:bg-brand-100 dark:hover:bg-brand-900/30'}`}
                aria-label={t('DashboardLayout:close_announcement_aria')}
              >
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>
        )}

        <main id="sp-main-content" tabIndex={-1} className="flex-1 min-w-0 pb-[calc(4rem+env(safe-area-inset-bottom))] lg:pb-0 flex flex-col">
          <div className="flex-1 min-w-0">
            <Suspense fallback={
              <div className="px-6 py-10 text-sm text-slate-400 dark:text-slate-500" role="status">
                {t('DashboardLayout:page_loading')}
              </div>
            }>
              <Outlet />
            </Suspense>
          </div>
          <footer className="py-4 text-center text-xs text-slate-400 dark:text-slate-600">
            SanalCP {surumBilgi?.mevcut ? `v${surumBilgi.mevcut}` : ''}
            {surumBilgi?.build_tarihi ? ` · Build: ${surumBilgi.build_tarihi}` : ''}
          </footer>
        </main>
      </div>

      <AltNavBar onMenuAc={() => setMobilAcik(true)} />
    </div>
  )
}
