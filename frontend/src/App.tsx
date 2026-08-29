import { lazySayfa } from '@/lib/chunk'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/store/auth'
import LoginPage from '@/pages/LoginPage'
import DashboardLayout from '@/components/DashboardLayout'
import CPanelGirisPage from '@/pages/CPanelGirisPage'

// Giriş kabuğu küçük ve ilk ekranda gerekli; panel sayfalarının tamamı ise
// yalnız ilgili route açıldığında indirilir. Böylece dosya yöneticisi, kod
// editörü ve grafikler gibi ağır bağımlılıklar ilk panel bundle'ına girmez.
const HomePage = lazySayfa(() => import('@/pages/HomePage'))
const DomainsPage = lazySayfa(() => import('@/pages/DomainsPage'))
const SubscriptionDetailPage = lazySayfa(() => import('@/pages/SubscriptionDetailPage'))
const ServicePlansPage = lazySayfa(() => import('@/pages/ServicePlansPage'))
const SettingsPage = lazySayfa(() => import('@/pages/SettingsPage'))
const PlaceholderPage = lazySayfa(() => import('@/pages/PlaceholderPage'))
const ToolPage = lazySayfa(() => import('@/pages/ToolPage'))
const DomainFilesPage = lazySayfa(() => import('@/pages/DomainFilesPage'))
const DomainSSLPage = lazySayfa(() => import('@/pages/DomainSSLPage'))
const DomainSSHPage = lazySayfa(() => import('@/pages/DomainSSHPage'))
const DomainStatsPage = lazySayfa(() => import('@/pages/DomainStatsPage'))
const DomainPerformansPage = lazySayfa(() => import('@/pages/DomainPerformansPage'))
const DomainComposerPage = lazySayfa(() => import('@/pages/DomainComposerPage'))
const DomainSifreKorumaPage = lazySayfa(() => import('@/pages/DomainSifreKorumaPage'))
const DomainAntivirusPage = lazySayfa(() => import('@/pages/DomainAntivirusPage'))
const DomainKopyaPage = lazySayfa(() => import('@/pages/DomainKopyaPage'))
const DomainIceAktarimPage = lazySayfa(() => import('@/pages/DomainIceAktarimPage'))
const DomainCronPage = lazySayfa(() => import('@/pages/DomainCronPage'))
const DomainLogsPage = lazySayfa(() => import('@/pages/DomainLogsPage'))
const DomainDNSPage = lazySayfa(() => import('@/pages/DomainDNSPage'))
const DomainCloudflarePage = lazySayfa(() => import('@/pages/DomainCloudflarePage'))
const RedisPage = lazySayfa(() => import('@/pages/RedisPage'))
const DomainConnectionPage = lazySayfa(() => import('@/pages/DomainConnectionPage'))
const DomainDatabasesPage = lazySayfa(() => import('@/pages/DomainDatabasesPage'))
const DomainDatabaseYonetPage = lazySayfa(() => import('@/pages/DomainDatabaseYonetPage'))
const DomainFTPPage = lazySayfa(() => import('@/pages/DomainFTPPage'))
const DomainMailPage = lazySayfa(() => import('@/pages/DomainMailPage'))
const DomainPHPPage = lazySayfa(() => import('@/pages/DomainPHPPage'))
const DomainBackupsPage = lazySayfa(() => import('@/pages/DomainBackupsPage'))
const DomainGitPage = lazySayfa(() => import('@/pages/DomainGitPage'))
const DomainWebSunucuPage = lazySayfa(() => import('@/pages/DomainWebSunucuPage'))
const DomainWafPage = lazySayfa(() => import('@/pages/DomainWafPage'))
const DomainRateLimitPage = lazySayfa(() => import('@/pages/DomainRateLimitPage'))
const PHPModuleriPage = lazySayfa(() => import('@/pages/PHPModuleriPage'))
const PaketlerPage = lazySayfa(() => import('@/pages/PaketlerPage'))
const PaketDetayPage = lazySayfa(() => import('@/pages/PaketDetayPage'))
const PHPSurumleriPage = lazySayfa(() => import('@/pages/PHPSurumleriPage'))
const AraclarAyarlarPage = lazySayfa(() => import('@/pages/AraclarAyarlarPage'))
const DNSSablonuPage = lazySayfa(() => import('@/pages/DNSSablonuPage'))
const ServislerPage = lazySayfa(() => import('@/pages/ServislerPage'))
const AppsPage = lazySayfa(() => import('@/pages/AppsPage'))
const FirewallPage = lazySayfa(() => import('@/pages/FirewallPage'))
const BackupYonetimiPage = lazySayfa(() => import('@/pages/BackupYonetimiPage'))
const DomainWordPressPage = lazySayfa(() => import('@/pages/DomainWordPressPage'))
const DomainPrestaShopPage = lazySayfa(() => import('@/pages/DomainPrestaShopPage'))
const DomainLaravelPage = lazySayfa(() => import('@/pages/DomainLaravelPage'))
const DomainAppsPage = lazySayfa(() => import('@/pages/DomainAppsPage'))
const DomainSubdomainlerPage = lazySayfa(() => import('@/pages/DomainSubdomainlerPage'))
const DomainEkAlanlarPage = lazySayfa(() => import('@/pages/DomainEkAlanlarPage'))
const DomainErisimKontrolPage = lazySayfa(() => import('@/pages/DomainErisimKontrolPage'))
const IstatistiklerPage = lazySayfa(() => import('@/pages/IstatistiklerPage'))
const IzlemePage = lazySayfa(() => import('@/pages/IzlemePage'))
const YakindaPage = lazySayfa(() => import('@/pages/YakindaPage'))
const DNSGenelPage = lazySayfa(() => import('@/pages/DNSGenelPage'))
const SSLGenelPage = lazySayfa(() => import('@/pages/SSLGenelPage'))
const MailGenelPage = lazySayfa(() => import('@/pages/MailGenelPage'))
const VeritabanlariGenelPage = lazySayfa(() => import('@/pages/VeritabanlariGenelPage'))
const MusterilerPage = lazySayfa(() => import('@/pages/MusterilerPage'))
const GuvenlikGunluguPage = lazySayfa(() => import('@/pages/GuvenlikGunluguPage'))
const GuvenlikBildirimleriPage = lazySayfa(() => import('@/pages/GuvenlikBildirimleriPage'))
const KullanicilarPage = lazySayfa(() => import('@/pages/KullanicilarPage'))
const BayiPaketleriPage = lazySayfa(() => import('@/pages/BayiPaketleriPage'))
const BayiOzetPage = lazySayfa(() => import('@/pages/BayiOzetPage'))
const SunucuDurumuPage = lazySayfa(() => import('@/pages/SunucuDurumuPage'))
const HesapAktarimiPage = lazySayfa(() => import('@/pages/HesapAktarimiPage'))

function GuardedRoute({ children }: { children: React.ReactNode }) {
  // Yalnız yönlendirme kararı: gerçek yetki her istekte sunucuda, oturum
  // çerezinden çözülür. Bu bayrağın elle değiştirilmesi hiçbir kapı açmaz.
  const oturumVar = useAuth((s) => s.oturumVar)
  if (!oturumVar) return <Navigate to="/giris" replace />
  return <>{children}</>
}

export default function App() {
  const { t } = useTranslation('YakindaPage')
  return (
    <Routes>
      <Route path="/giris" element={<LoginPage />} />
        <Route path="/cp/giris" element={<CPanelGirisPage />} />
        <Route path="/cp" element={<CPanelGirisPage />} />
      <Route
        path="/"
        element={
          <GuardedRoute>
            <DashboardLayout />
          </GuardedRoute>
        }
      >
        <Route index                       element={<HomePage />} />
        <Route path="domainler"            element={<DomainsPage />} />
        <Route path="abonelikler"          element={<Navigate to="/domainler" replace />} />
        <Route path="abonelikler/:id"      element={<SubscriptionDetailPage />} />
        <Route path="abonelikler/:id/baglanti"      element={<DomainConnectionPage />} />
        <Route path="abonelikler/:id/dosyalar"      element={<DomainFilesPage />} />
        <Route path="abonelikler/:id/veritabanlari" element={<DomainDatabasesPage />} />
        <Route path="abonelikler/:id/veritabanlari/:dbAdi" element={<DomainDatabaseYonetPage />} />
        <Route path="abonelikler/:id/ftp"           element={<DomainFTPPage />} />
        <Route path="abonelikler/:id/php"           element={<DomainPHPPage />} />
        <Route path="abonelikler/:id/ssl"           element={<DomainSSLPage />} />
        <Route path="abonelikler/:id/ssh-erisim"    element={<DomainSSHPage />} />
        <Route path="abonelikler/:id/istatistik"    element={<DomainStatsPage />} />
        <Route path="abonelikler/:id/performans"    element={<DomainPerformansPage />} />
        <Route path="abonelikler/:id/composer"      element={<DomainComposerPage />} />
        <Route path="abonelikler/:id/sifre-koruma"  element={<DomainSifreKorumaPage />} />
        <Route path="abonelikler/:id/imunify"       element={<DomainAntivirusPage />} />
        <Route path="abonelikler/:id/kopyala"       element={<DomainKopyaPage />} />
        <Route path="abonelikler/:id/ice-aktarim"   element={<DomainIceAktarimPage />} />
        <Route path="abonelikler/:id/wordpress"     element={<DomainWordPressPage />} />
        <Route path="abonelikler/:id/prestashop"    element={<DomainPrestaShopPage />} />
        <Route path="abonelikler/:id/laravel"       element={<DomainLaravelPage />} />
        <Route path="abonelikler/:id/uygulamalar"   element={<DomainAppsPage />} />
        <Route path="abonelikler/:id/subdomainler"  element={<DomainSubdomainlerPage />} />
        <Route path="abonelikler/:id/ek-alanlar"    element={<DomainEkAlanlarPage />} />
        <Route path="abonelikler/:id/erisim-kontrol" element={<DomainErisimKontrolPage />} />
        <Route path="abonelikler/:id/cron"          element={<DomainCronPage />} />
        <Route path="abonelikler/:id/gunlukler"     element={<DomainLogsPage />} />
        <Route path="abonelikler/:id/dns"           element={<DomainDNSPage />} />
        <Route path="abonelikler/:id/cloudflare"    element={<DomainCloudflarePage />} />
        <Route path="abonelikler/:id/redis"         element={<RedisPage />} />
        <Route path="abonelikler/:id/mail"          element={<DomainMailPage />} />
        <Route path="abonelikler/:id/yedekler"      element={<DomainBackupsPage />} />
        <Route path="abonelikler/:id/git"           element={<DomainGitPage />} />
        <Route path="abonelikler/:id/web-sunucu"    element={<DomainWebSunucuPage />} />
        <Route path="abonelikler/:id/waf"           element={<DomainWafPage />} />
        <Route path="abonelikler/:id/rate-limit"    element={<DomainRateLimitPage />} />
        <Route path="sistem/php-modulleri"           element={<PHPModuleriPage />} />
        <Route path="araclar/paketler"               element={<PaketlerPage />} />
        <Route path="araclar/paketler/:id"           element={<PaketDetayPage />} />
        <Route path="araclar/php-surumler"           element={<PHPSurumleriPage />} />
        <Route path="araclar/servisler"              element={<ServislerPage />} />
        <Route path="araclar/dns-sablonu"            element={<DNSSablonuPage />} />
        <Route path="abonelikler/:id/:slug" element={<ToolPage />} />
        <Route path="hizmet-planlari"      element={<ServicePlansPage />} />

        {/* Sunucu geneli özet listeler (Faz 3) — domain kapsamlı sayfaların
            karşılığı; düzenleme hâlâ /abonelikler/:id/* altında yapılır. */}
        <Route path="dns"            element={<DNSGenelPage />} />
        <Route path="ssl"            element={<SSLGenelPage />} />
        <Route path="mail"           element={<MailGenelPage />} />
        <Route path="veritabanlari"  element={<VeritabanlariGenelPage />} />

        {/* Faz 4 — yönetim ekranları */}
        <Route path="kullanicilar"      element={<KullanicilarPage />} />
        <Route path="bayi-paketleri"    element={<BayiPaketleriPage />} />
        <Route path="bayi-ozet"         element={<BayiOzetPage />} />
        <Route path="sunucu-durumu"     element={<SunucuDurumuPage />} />
        <Route path="musteriler"        element={<MusterilerPage />} />
        <Route path="guvenlik-gunlugu"  element={<GuvenlikGunluguPage />} />
        <Route path="guvenlik-bildirimleri" element={<GuvenlikBildirimleriPage />} />

        <Route path="araclar-ayarlar" element={<AraclarAyarlarPage />} />
        <Route path="istatistikler" element={<IstatistiklerPage />} />
        <Route path="eklentiler" element={<YakindaPage baslik={t('YakindaPage:eklentiler.title')} ikon="🧩" aciklama={t('YakindaPage:eklentiler.description')} ozellikler={t('YakindaPage:eklentiler.features', { returnObjects: true }) as string[]} />} />
        <Route path="uygulamalar" element={<AppsPage />} />
        <Route path="wordpress" element={<Navigate to="/uygulamalar" replace />} />
        <Route path="firewall" element={<FirewallPage />} />
        <Route path="backup-yonetimi" element={<BackupYonetimiPage />} />
        <Route path="hesap-aktarimi" element={<HesapAktarimiPage />} />
        <Route path="izleme" element={<IzlemePage />} />

        <Route path="profil"          element={<SettingsPage />} />
        <Route path="parola-degistir" element={<Navigate to="/profil" replace />} />
        <Route path="ayarlar"         element={<Navigate to="/profil" replace />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
