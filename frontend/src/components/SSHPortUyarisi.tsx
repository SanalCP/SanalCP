import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'

// SSH varsayılan portu (22) açıkken gösterilen kalıcı güvenlik uyarısı.
// Port değiştirildiği anda backend `varsayilan_port: false` döner ve uyarı kaybolur.
//
// Kaynak `sshd -T` (efektif yapılandırma) olduğu için sshd_config.d altındaki
// include dosyalarında yapılan değişiklikler de anında yansır.

type Durum = { portlar: number[]; varsayilan_port: boolean; varsayilan_deger: number }

export default function SSHPortUyarisi() {
  const { t } = useTranslation(['SSHPortUyarisi'])
  const [durum, setDurum] = useState<Durum | null>(null)

  useEffect(() => {
    api.get<Durum>('/system/ssh-guvenlik')
      .then(r => setDurum(r.data))
      .catch(() => { /* yetkisiz/erişilemez — uyarı gösterilmez */ })
  }, [])

  if (!durum?.varsayilan_port) return null

  // 22 dışında da port varsa geçiş dönemindedir: yeni port çalışıyor ama 22 hâlâ açık.
  const digerPortlar = durum.portlar.filter(p => p !== durum.varsayilan_deger)

  return (
    <div role="alert"
      className="mb-6 rounded-2xl border border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-4">
      <div className="flex items-start gap-3">
        <div className="w-10 h-10 rounded-lg bg-red-100 dark:bg-red-900/40 text-red-700 dark:text-red-300 flex items-center justify-center text-xl shrink-0">
          ⚠️
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-semibold text-red-800 dark:text-red-200">
              {t('SSHPortUyarisi:title')}
            </span>
            <span className="text-[10px] uppercase tracking-wider px-2 py-0.5 rounded font-bold bg-red-600 text-white">
              {t('SSHPortUyarisi:badge')}
            </span>
          </div>

          <p className="text-xs text-red-800/90 dark:text-red-300 mt-1">
            {digerPortlar.length > 0
              ? t('SSHPortUyarisi:desc_mixed', { diger: digerPortlar.join(', ') })
              : t('SSHPortUyarisi:desc')}
          </p>

          <details className="mt-2">
            <summary className="text-xs font-medium text-red-800 dark:text-red-200 cursor-pointer hover:underline">
              {t('SSHPortUyarisi:how')}
            </summary>
            <ol className="mt-2 space-y-1.5 text-[11px] text-red-900/90 dark:text-red-300 list-decimal list-inside">
              <li>{t('SSHPortUyarisi:step1')} <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">/etc/ssh/sshd_config.d/99-port.conf</code> → <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">Port 2222</code></li>
              <li>{t('SSHPortUyarisi:step2')} <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">semanage port -a -t ssh_port_t -p tcp 2222</code></li>
              <li>{t('SSHPortUyarisi:step3')} <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">firewall-cmd --permanent --add-port=2222/tcp && firewall-cmd --reload</code></li>
              <li>{t('SSHPortUyarisi:step4')} <code className="font-mono bg-red-100 dark:bg-red-900/40 px-1 rounded">sshd -t && systemctl restart sshd</code></li>
              <li className="font-medium">{t('SSHPortUyarisi:step5')}</li>
            </ol>
            <p className="mt-2 text-[11px] text-red-900/80 dark:text-red-300/80">{t('SSHPortUyarisi:note')}</p>
          </details>
        </div>
      </div>
    </div>
  )
}
