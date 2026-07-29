// Bayinin kendi kaynak özeti — /bayi/ozet. Yalnız 'reseller' rolüne açık
// (bkz. cmd/server/main.go RequireRole). Admin'in karşılığı yok: admin zaten
// tüm bayileri Kullanıcılar + Bayi Paketleri üzerinden görür.
import { useEffect, useState } from 'react'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Ozet = {
  paket_ad: string
  fazla_satis: boolean
  musteri_adet: number
  musteri_limit: number
  domain_adet: number
  domain_limit: number
  askida_adet: number
  disk_kullanim_mb: number
  disk_taahhut_mb: number
  disk_limit_mb: number
  trafik_kullanim_mb: number
  trafik_taahhut_mb: number
  trafik_limit_mb: number
  izinli_plan_sayisi: number
}

export default function BayiOzetPage() {
  const [ozet, setOzet] = useState<Ozet | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)

  useEffect(() => {
    api.get<Ozet>('/bayi/ozet')
      .then(r => setOzet(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }, [])

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: 'Anasayfa', href: '/' }, { etiket: 'Kaynak Özetim' }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">Kaynak Özetim</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
        Bayi hesabınızın limitleri, gerçek kullanımı ve müşterilerinize dağıttığınız kotaların
        toplamı (taahhüt).
      </p>

      {hata && <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Yükleniyor…</div>
      ) : ozet ? (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <div className="lg:col-span-2 space-y-4">
            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Kaynak Kullanımı</h3>
                {ozet.paket_ad && (
                  <span className="text-[11px] font-mono font-semibold bg-slate-100 dark:bg-slate-700/60 text-slate-600 dark:text-slate-300 px-2 py-0.5 rounded">{ozet.paket_ad}</span>
                )}
              </div>
              <Bar etiket="Disk" k={ozet.disk_kullanim_mb} t={ozet.disk_taahhut_mb} l={ozet.disk_limit_mb} birim="MB" renk="indigo" fazlaSatis={ozet.fazla_satis} />
              <Bar etiket="Trafik (aylık)" k={ozet.trafik_kullanim_mb} t={ozet.trafik_taahhut_mb} l={ozet.trafik_limit_mb} birim="MB" renk="sky" fazlaSatis={ozet.fazla_satis} />
              <Bar etiket="Müşteri" k={ozet.musteri_adet} l={ozet.musteri_limit} birim="adet" renk="emerald" />
              <Bar etiket="Domain" k={ozet.domain_adet} l={ozet.domain_limit} birim="adet" renk="violet" />
              {!ozet.fazla_satis && (
                <p className="mt-3 text-[11px] text-amber-600 dark:text-amber-400">
                  Fazla satış kapalı: müşterilerinize atadığınız disk/trafik kotalarının toplamı (taahhüt) limitin üstüne çıkamaz.
                </p>
              )}
            </div>
          </div>

          <div className="space-y-4">
            <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-4">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">Durum</h3>
              <Mini etiket="Askıdaki hesap" deger={ozet.askida_adet} uyari={ozet.askida_adet > 0} />
              <Mini etiket="Kullanabileceği hizmet planı" deger={ozet.izinli_plan_sayisi > 0 ? `${ozet.izinli_plan_sayisi} plan (kısıtlı)` : 'Tümü'} />
              <Mini etiket="Fazla satış" deger={ozet.fazla_satis ? 'Açık' : 'Kapalı'} />
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function Bar({ etiket, k, t, l, birim, renk, fazlaSatis }: { etiket: string; k: number; t?: number; l: number; birim: string; renk: string; fazlaSatis?: boolean }) {
  const sinirsiz = l === 0
  const oranK = sinirsiz ? 0 : Math.min(100, Math.round((k / l) * 100))
  const oranT = sinirsiz || t === undefined ? 0 : Math.min(100, Math.round((t / l) * 100))
  const grad: Record<string, string> = {
    indigo: 'from-indigo-400 to-indigo-600',
    sky: 'from-sky-400 to-sky-600',
    emerald: 'from-emerald-400 to-emerald-600',
    violet: 'from-violet-400 to-violet-600',
  }
  const fill = oranK >= 90 ? 'from-red-400 to-red-600' : (oranK >= 75 ? 'from-amber-400 to-amber-600' : (grad[renk] || 'from-slate-400 to-slate-600'))
  const fade = 'linear-gradient(to right, black 0%, black 35%, transparent 96%)'
  return (
    <div className="mb-4 last:mb-0">
      <div className="flex items-baseline justify-between mb-1">
        <span className="text-xs font-medium text-slate-600 dark:text-slate-300">{etiket}</span>
        <span className="text-[11px] font-mono text-slate-500 dark:text-slate-400">
          {sinirsiz ? (
            <><span className="text-slate-700 dark:text-slate-200 font-semibold">{fmt(k)}</span> {birim} · <span className="text-emerald-500 font-bold">∞</span></>
          ) : t !== undefined ? (
            <><span className="text-slate-700 dark:text-slate-200 font-semibold">{fmt(k)}</span> kullanım{fazlaSatis === false ? <> · {fmt(t)} taahhüt</> : null} / {fmt(l)} {birim}</>
          ) : (
            <><span className="text-slate-700 dark:text-slate-200 font-semibold">{fmt(k)}</span> / {fmt(l)} {birim}</>
          )}
        </span>
      </div>
      <div className="h-2 rounded-full bg-slate-100 dark:bg-slate-700/50 overflow-hidden relative">
        {sinirsiz ? (
          <div
            className={`h-full rounded-full bg-gradient-to-r ${grad[renk] || 'from-slate-400 to-slate-600'}`}
            style={{ width: '100%', maskImage: fade, WebkitMaskImage: fade }}
          />
        ) : (
          <>
            <div className={`h-full rounded-full bg-gradient-to-r ${fill}`} style={{ width: Math.max(oranK, 3) + '%' }} />
            {fazlaSatis === false && oranT > oranK && (
              <div className="absolute top-0 h-2 border-r-2 border-amber-500" style={{ left: `min(${oranT}%, 100%)` }} title="Taahhüt sınırı" />
            )}
          </>
        )}
      </div>
    </div>
  )
}
function fmt(n: number) {
  if (n >= 1024) return (n / 1024).toFixed(1) + 'k'
  return String(n)
}
function Mini({ etiket, deger, uyari }: { etiket: string; deger: number | string; uyari?: boolean }) {
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-slate-50 dark:border-slate-800 last:border-0">
      <span className="text-xs text-slate-500 dark:text-slate-500">{etiket}</span>
      <span className={`text-xs font-mono font-medium ${uyari ? 'text-amber-600 dark:text-amber-400' : 'text-slate-700 dark:text-slate-300'}`}>{deger}</span>
    </div>
  )
}
