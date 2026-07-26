// Bayi için salt-okunur sunucu durumu.
//
// Mevcut İzleme sayfası admin uçları çağırıyor (/system/processes, /domains,
// /admin/system/loglar) ve bayi için büyük kısmı 403 döner. Bu sayfa yalnız
// Faz 5A'da bayiye açılan uçları kullanır: /system/usage, /system/servisler,
// /system/load-history, /system/guncelleme.
//
// Admin de açabilir (aynı veriyi özet olarak görür); asıl derinlik hâlâ
// İzleme sayfasındadır.
import { useEffect, useState } from 'react'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Usage = {
  cpu_yuzde?: number
  ram_kullanilan_mb?: number
  ram_toplam_mb?: number
  disk_kullanilan_gb?: number
  disk_toplam_gb?: number
  yuk?: number[]
  uptime?: string
}
type Servis = { birim: string; etiket: string; grup: string; durum: string }
type Guncelleme = { mevcut?: string; son?: string; guncelleme_var?: boolean }

const DURUM_STIL: Record<string, string> = {
  active: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
  inactive: 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
  failed: 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300',
  absent: 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
}
const DURUM_ETIKET: Record<string, string> = {
  active: 'Çalışıyor', inactive: 'Durmuş', failed: 'Hatalı', absent: 'Kurulu değil',
}

function Kart({ baslik, deger, alt }: { baslik: string; deger: string; alt?: string }) {
  return (
    <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 px-4 py-3">
      <div className="text-xs font-medium uppercase tracking-wider text-slate-500 dark:text-slate-400">{baslik}</div>
      <div className="mt-1 text-xl font-semibold text-slate-900 dark:text-slate-100">{deger}</div>
      {alt && <div className="mt-0.5 text-xs text-slate-400">{alt}</div>}
    </div>
  )
}

export default function SunucuDurumuPage() {
  const [usage, setUsage] = useState<Usage | null>(null)
  const [servisler, setServisler] = useState<Servis[]>([])
  const [guncelleme, setGuncelleme] = useState<Guncelleme | null>(null)
  const [hata, setHata] = useState<string | null>(null)
  const [yukleniyor, setYukleniyor] = useState(true)

  useEffect(() => {
    let iptal = false
    Promise.all([
      api.get<Usage>('/system/usage').then((r) => { if (!iptal) setUsage(r.data) }),
      api.get<Servis[]>('/system/servisler').then((r) => { if (!iptal) setServisler(Array.isArray(r.data) ? r.data : []) }),
      api.get<Guncelleme>('/system/guncelleme').then((r) => { if (!iptal) setGuncelleme(r.data) }).catch(() => {}),
    ])
      .catch((e) => { if (!iptal) setHata(apiHata(e, 'Sunucu durumu alınamadı')) })
      .finally(() => { if (!iptal) setYukleniyor(false) })
    return () => { iptal = true }
  }, [])

  const ramYuzde = usage?.ram_toplam_mb
    ? Math.round(((usage.ram_kullanilan_mb ?? 0) / usage.ram_toplam_mb) * 100)
    : null
  const diskYuzde = usage?.disk_toplam_gb
    ? Math.round(((usage.disk_kullanilan_gb ?? 0) / usage.disk_toplam_gb) * 100)
    : null

  return (
    <div className="max-w-5xl mx-auto px-4 py-6">
      <Breadcrumb items={[{ etiket: 'Anasayfa', href: '/' }, { etiket: 'Sunucu Durumu' }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Sunucu Durumu</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          Sunucunun anlık durumu. Bu ekran salt-okunurdur — servis yönetimi yöneticiye aittir.
        </p>
      </div>

      {hata && <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{hata}</div>}

      {yukleniyor ? (
        <div className="py-16 text-center text-sm text-slate-400">Yükleniyor…</div>
      ) : (
        <>
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-6">
            <Kart baslik="CPU" deger={usage?.cpu_yuzde != null ? `%${Math.round(usage.cpu_yuzde)}` : '—'}
                  alt={usage?.yuk?.length ? `yük ${usage.yuk.map((y) => y.toFixed(2)).join(' · ')}` : undefined} />
            <Kart baslik="Bellek" deger={ramYuzde != null ? `%${ramYuzde}` : '—'}
                  alt={usage?.ram_toplam_mb ? `${usage.ram_kullanilan_mb ?? 0} / ${usage.ram_toplam_mb} MB` : undefined} />
            <Kart baslik="Disk" deger={diskYuzde != null ? `%${diskYuzde}` : '—'}
                  alt={usage?.disk_toplam_gb ? `${usage.disk_kullanilan_gb ?? 0} / ${usage.disk_toplam_gb} GB` : undefined} />
            <Kart baslik="Panel Sürümü" deger={guncelleme?.mevcut ? `v${guncelleme.mevcut}` : '—'}
                  alt={guncelleme?.guncelleme_var ? `güncelleme var: v${guncelleme.son}` : 'güncel'} />
          </div>

          <h2 className="text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-2">Servisler</h2>
          {servisler.length === 0 ? (
            <div className="py-8 text-center text-sm text-slate-400">Servis bilgisi yok.</div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
              {servisler.map((s) => (
                <div key={s.birim} className="flex items-center justify-between gap-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 px-3 py-2">
                  <div className="min-w-0">
                    <div className="truncate text-sm text-slate-900 dark:text-slate-100">{s.etiket}</div>
                    <div className="truncate text-[11px] text-slate-400">{s.grup}</div>
                  </div>
                  <span className={`shrink-0 px-2 py-0.5 rounded text-xs ${DURUM_STIL[s.durum] ?? DURUM_STIL.absent}`}>
                    {DURUM_ETIKET[s.durum] ?? s.durum}
                  </span>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
