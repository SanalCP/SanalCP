import { lazy } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/store/auth'
import LoginPage from '@/pages/LoginPage'
import DashboardLayout from '@/components/DashboardLayout'
import CPanelGirisPage from '@/pages/CPanelGirisPage'

// Giriş kabuğu küçük ve ilk ekranda gerekli; panel sayfalarının tamamı ise
// yalnız ilgili route açıldığında indirilir. Böylece dosya yöneticisi, kod
// editörü ve grafikler gibi ağır bağımlılıklar ilk panel bundle'ına girmez.
const HomePage = lazy(() => import('@/pages/HomePage'))
const DomainsPage = lazy(() => import('@/pages/DomainsPage'))
const SubscriptionDetailPage = lazy(() => import('@/pages/SubscriptionDetailPage'))
const ServicePlansPage = lazy(() => import('@/pages/ServicePlansPage'))
const SettingsPage = lazy(() => import('@/pages/SettingsPage'))
const PlaceholderPage = lazy(() => import('@/pages/PlaceholderPage'))
const ToolPage = lazy(() => import('@/pages/ToolPage'))
const DomainFilesPage = lazy(() => import('@/pages/DomainFilesPage'))
const DomainSSLPage = lazy(() => import('@/pages/DomainSSLPage'))
const DomainSSHPage = lazy(() => import('@/pages/DomainSSHPage'))
const DomainStatsPage = lazy(() => import('@/pages/DomainStatsPage'))
const DomainPerformansPage = lazy(() => import('@/pages/DomainPerformansPage'))
const DomainComposerPage = lazy(() => import('@/pages/DomainComposerPage'))
const DomainSifreKorumaPage = lazy(() => import('@/pages/DomainSifreKorumaPage'))
const DomainAntivirusPage = lazy(() => import('@/pages/DomainAntivirusPage'))
const DomainKopyaPage = lazy(() => import('@/pages/DomainKopyaPage'))
const DomainIceAktarimPage = lazy(() => import('@/pages/DomainIceAktarimPage'))
const DomainCronPage = lazy(() => import('@/pages/DomainCronPage'))
const DomainLogsPage = lazy(() => import('@/pages/DomainLogsPage'))
const DomainDNSPage = lazy(() => import('@/pages/DomainDNSPage'))
const RedisPage = lazy(() => import('@/pages/RedisPage'))
const DomainConnectionPage = lazy(() => import('@/pages/DomainConnectionPage'))
const DomainDatabasesPage = lazy(() => import('@/pages/DomainDatabasesPage'))
const DomainDatabaseYonetPage = lazy(() => import('@/pages/DomainDatabaseYonetPage'))
const DomainFTPPage = lazy(() => import('@/pages/DomainFTPPage'))
const DomainMailPage = lazy(() => import('@/pages/DomainMailPage'))
const DomainPHPPage = lazy(() => import('@/pages/DomainPHPPage'))
const DomainBackupsPage = lazy(() => import('@/pages/DomainBackupsPage'))
const DomainGitPage = lazy(() => import('@/pages/DomainGitPage'))
const DomainWebSunucuPage = lazy(() => import('@/pages/DomainWebSunucuPage'))
const DomainWafPage = lazy(() => import('@/pages/DomainWafPage'))
const PHPModuleriPage = lazy(() => import('@/pages/PHPModuleriPage'))
const PaketlerPage = lazy(() => import('@/pages/PaketlerPage'))
const PaketDetayPage = lazy(() => import('@/pages/PaketDetayPage'))
const PHPSurumleriPage = lazy(() => import('@/pages/PHPSurumleriPage'))
const AraclarAyarlarPage = lazy(() => import('@/pages/AraclarAyarlarPage'))
const DNSSablonuPage = lazy(() => import('@/pages/DNSSablonuPage'))
const ServislerPage = lazy(() => import('@/pages/ServislerPage'))
const AppsPage = lazy(() => import('@/pages/AppsPage'))
const FirewallPage = lazy(() => import('@/pages/FirewallPage'))
const BackupYonetimiPage = lazy(() => import('@/pages/BackupYonetimiPage'))
const DomainWordPressPage = lazy(() => import('@/pages/DomainWordPressPage'))
const DomainAppsPage = lazy(() => import('@/pages/DomainAppsPage'))
const DomainSubdomainlerPage = lazy(() => import('@/pages/DomainSubdomainlerPage'))
const DomainEkAlanlarPage = lazy(() => import('@/pages/DomainEkAlanlarPage'))
const DomainErisimKontrolPage = lazy(() => import('@/pages/DomainErisimKontrolPage'))
const IstatistiklerPage = lazy(() => import('@/pages/IstatistiklerPage'))
const IzlemePage = lazy(() => import('@/pages/IzlemePage'))
const YakindaPage = lazy(() => import('@/pages/YakindaPage'))
const DNSGenelPage = lazy(() => import('@/pages/DNSGenelPage'))
const SSLGenelPage = lazy(() => import('@/pages/SSLGenelPage'))
const MailGenelPage = lazy(() => import('@/pages/MailGenelPage'))
const VeritabanlariGenelPage = lazy(() => import('@/pages/VeritabanlariGenelPage'))
const MusterilerPage = lazy(() => import('@/pages/MusterilerPage'))
const GuvenlikGunluguPage = lazy(() => import('@/pages/GuvenlikGunluguPage'))
const KullanicilarPage = lazy(() => import('@/pages/KullanicilarPage'))
const BayiPaketleriPage = lazy(() => import('@/pages/BayiPaketleriPage'))
const BayiOzetPage = lazy(() => import('@/pages/BayiOzetPage'))
const SunucuDurumuPage = lazy(() => import('@/pages/SunucuDurumuPage'))
const HesapAktarimiPage = lazy(() => import('@/pages/HesapAktarimiPage'))

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
        <Route path="abonelikler/:id/uygulamalar"   element={<DomainAppsPage />} />
        <Route path="abonelikler/:id/subdomainler"  element={<DomainSubdomainlerPage />} />
        <Route path="abonelikler/:id/ek-alanlar"    element={<DomainEkAlanlarPage />} />
        <Route path="abonelikler/:id/erisim-kontrol" element={<DomainErisimKontrolPage />} />
        <Route path="abonelikler/:id/cron"          element={<DomainCronPage />} />
        <Route path="abonelikler/:id/gunlukler"     element={<DomainLogsPage />} />
        <Route path="abonelikler/:id/dns"           element={<DomainDNSPage />} />
        <Route path="abonelikler/:id/redis"         element={<RedisPage />} />
        <Route path="abonelikler/:id/mail"          element={<DomainMailPage />} />
        <Route path="abonelikler/:id/yedekler"      element={<DomainBackupsPage />} />
        <Route path="abonelikler/:id/git"           element={<DomainGitPage />} />
        <Route path="abonelikler/:id/web-sunucu"    element={<DomainWebSunucuPage />} />
        <Route path="abonelikler/:id/waf"           element={<DomainWafPage />} />
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
