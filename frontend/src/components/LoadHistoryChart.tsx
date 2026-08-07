import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'

type Nokta = { ts: string; yuk1: number; yuk5: number; yuk15: number; bellek: number }
type Yanit = { saat: number; cekirdek: number; noktalar: Nokta[] }

function araliklar(t: (k: string) => string) {
  return [
    { et: t('LoadHistoryChart:range_1h'), saat: 1 },
    { et: t('LoadHistoryChart:range_6h'), saat: 6 },
    { et: t('LoadHistoryChart:range_24h'), saat: 24 },
    { et: t('LoadHistoryChart:range_7d'), saat: 168 },
  ]
}

// SVG düzlemi
const W = 1000, H = 260, ML = 44, MR = 14, MT = 14, MB = 28

export default function LoadHistoryChart() {
  const { t } = useTranslation(['LoadHistoryChart'])
  const ARALIKLAR = araliklar(t)
  const [saat, setSaat] = useState(24)
  const [d, setD] = useState<Yanit | null>(null)
  const [yetkiYok, setYetkiYok] = useState(false)

  useEffect(() => {
    let on = true
    async function tick() {
      try {
        const r = await api.get<Yanit>(`/system/load-history?saat=${saat}`)
        if (on) { setD(r.data); setYetkiYok(false) }
      } catch (e: any) {
        if (on && e?.response?.status === 403) setYetkiYok(true)
      }
    }
    tick()
    const t = setInterval(tick, 30000)
    return () => { on = false; clearInterval(t) }
  }, [saat])

  if (yetkiYok) return null // yalnız yönetici

  const pts = d?.noktalar || []
  const cek = d?.cekirdek || 0
  const son = pts[pts.length - 1]

  // Y ölçeği: en yüksek yük ile çekirdek sayısı arasında, %15 boşluk
  const maxVal = Math.max(0.5, cek, ...pts.flatMap(p => [p.yuk1, p.yuk5, p.yuk15]))
  const yMax = Math.ceil(maxVal * 1.15 * 10) / 10
  const cw = W - ML - MR, ch = H - MT - MB
  const xAt = (i: number) => ML + (pts.length <= 1 ? 0 : (i / (pts.length - 1)) * cw)
  const yAt = (v: number) => MT + (1 - Math.min(v, yMax) / yMax) * ch
  const line = (key: keyof Nokta) => pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${xAt(i).toFixed(1)},${yAt(p[key] as number).toFixed(1)}`).join(' ')
  const area1 = pts.length
    ? `M${xAt(0).toFixed(1)},${(H - MB).toFixed(1)} ` +
      pts.map((p, i) => `L${xAt(i).toFixed(1)},${yAt(p.yuk1).toFixed(1)}`).join(' ') +
      ` L${xAt(pts.length - 1).toFixed(1)},${(H - MB).toFixed(1)} Z`
    : ''

  // eksen etiketleri
  const yTicks = [0, yMax / 2, yMax]
  const xLabel = (ts: string) => saat > 24 ? ts.slice(5, 10).replace('-', '/') : ts.slice(11, 16)
  const xTickIdx = pts.length > 1
    ? [0, Math.floor(pts.length / 3), Math.floor((2 * pts.length) / 3), pts.length - 1]
    : []

  return (
    <div className="ta-card p-5">
      <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
        <div>
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('LoadHistoryChart:title')}</h3>
          <p className="text-[11px] text-slate-400 dark:text-slate-500">
            {t('LoadHistoryChart:subtitle')}{cek ? t('LoadHistoryChart:cores_suffix', { count: cek }) : ''}
          </p>
        </div>
        <div className="flex items-center gap-1 bg-slate-100 dark:bg-slate-900/50 rounded-lg p-0.5">
          {ARALIKLAR.map(a => (
            <button key={a.saat} onClick={() => setSaat(a.saat)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${saat === a.saat
                ? 'bg-white dark:bg-slate-700 text-brand-700 dark:text-brand-300 shadow-sm'
                : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'}`}>
              {a.et}
            </button>
          ))}
        </div>
      </div>

      {/* anlık değerler */}
      {son && (
        <div className="flex flex-wrap gap-4 mb-3 text-xs">
          <Deger renk="#6366f1" etiket={t('LoadHistoryChart:min1')} v={son.yuk1} cek={cek} />
          <Deger renk="#f59e0b" etiket={t('LoadHistoryChart:min5')} v={son.yuk5} cek={cek} />
          <Deger renk="#94a3b8" etiket={t('LoadHistoryChart:min15')} v={son.yuk15} cek={cek} />
        </div>
      )}

      {pts.length === 0 ? (
        <div className="h-[220px] flex flex-col items-center justify-center text-center text-sm text-slate-400 dark:text-slate-500">
          <div className="text-2xl mb-2">📈</div>
          {t('LoadHistoryChart:no_data')}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <svg viewBox={`0 0 ${W} ${H}`} className="w-full" style={{ minWidth: 320 }} preserveAspectRatio="none">
            <defs>
              <linearGradient id="yuk1grad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#6366f1" stopOpacity="0.28" />
                <stop offset="100%" stopColor="#6366f1" stopOpacity="0.02" />
              </linearGradient>
            </defs>
            {/* yatay ızgara + Y etiketleri */}
            {yTicks.map((v, i) => (
              <g key={i}>
                <line x1={ML} y1={yAt(v)} x2={W - MR} y2={yAt(v)} className="stroke-slate-100 dark:stroke-slate-700/60" strokeWidth="1" />
                <text x={ML - 6} y={yAt(v) + 3} textAnchor="end" className="fill-slate-400" fontSize="11">{v.toFixed(1)}</text>
              </g>
            ))}
            {/* çekirdek referans çizgisi (=%100 kapasite) */}
            {cek > 0 && cek <= yMax && (
              <g>
                <line x1={ML} y1={yAt(cek)} x2={W - MR} y2={yAt(cek)} stroke="#ef4444" strokeWidth="1" strokeDasharray="4 4" opacity="0.6" />
                <text x={W - MR} y={yAt(cek) - 4} textAnchor="end" fill="#ef4444" fontSize="10" opacity="0.8">{t('LoadHistoryChart:cores_label', { count: cek })}</text>
              </g>
            )}
            {/* X etiketleri */}
            {xTickIdx.map((idx, i) => (
              <text key={i} x={xAt(idx)} y={H - 8} textAnchor={i === 0 ? 'start' : i === xTickIdx.length - 1 ? 'end' : 'middle'}
                className="fill-slate-400" fontSize="11">{xLabel(pts[idx].ts)}</text>
            ))}
            {/* seriler */}
            <path d={area1} fill="url(#yuk1grad)" />
            <path d={line('yuk15')} fill="none" stroke="#94a3b8" strokeWidth="1.4" strokeLinejoin="round" opacity="0.85" />
            <path d={line('yuk5')} fill="none" stroke="#f59e0b" strokeWidth="1.6" strokeLinejoin="round" opacity="0.9" />
            <path d={line('yuk1')} fill="none" stroke="#6366f1" strokeWidth="2" strokeLinejoin="round" />
          </svg>
        </div>
      )}
    </div>
  )
}

function Deger({ renk, etiket, v, cek }: { renk: string; etiket: string; v: number; cek: number }) {
  const asiri = cek > 0 && v > cek
  return (
    <div className="flex items-center gap-1.5">
      <span className="w-2.5 h-2.5 rounded-sm" style={{ background: renk }} />
      <span className="text-slate-500 dark:text-slate-400">{etiket}</span>
      <span className={`font-mono font-semibold ${asiri ? 'text-red-600 dark:text-red-400' : 'text-slate-800 dark:text-slate-100'}`}>
        {v.toFixed(2)}
      </span>
    </div>
  )
}
