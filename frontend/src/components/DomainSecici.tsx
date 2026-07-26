// sanal-dark-swept-v2
import { useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '@/lib/api'

type DomainOzet = { id: number; alan_adi: string }

/**
 * Kenar çubuğunun domain kipinde görünen alan adı seçici.
 *
 * Seçim yapılınca yalnız yoldaki domain kimliği değişir; alt sayfa korunur —
 * /abonelikler/12/dns üzerindeyken başka domaine geçmek /abonelikler/7/dns
 * açar. Birden çok domainin aynı ekranını karşılaştırmak bu sayede tek tık.
 */
export default function DomainSecici({ aktifID }: { aktifID: string }) {
  const [domainler, setDomainler] = useState<DomainOzet[]>([])
  const [acik, setAcik] = useState(false)
  const [arama, setArama] = useState('')
  const kutuRef = useRef<HTMLDivElement>(null)
  const aramaRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const konum = useLocation()

  useEffect(() => {
    api.get<DomainOzet[]>('/domains')
      .then((r) => setDomainler(Array.isArray(r.data) ? r.data : []))
      .catch(() => {})
  }, [])

  // Dışarı tıklama ve Esc ile kapat
  useEffect(() => {
    if (!acik) return
    function onTikla(e: MouseEvent) {
      if (kutuRef.current && !kutuRef.current.contains(e.target as Node)) setAcik(false)
    }
    function onTus(e: KeyboardEvent) { if (e.key === 'Escape') setAcik(false) }
    document.addEventListener('mousedown', onTikla)
    document.addEventListener('keydown', onTus)
    return () => {
      document.removeEventListener('mousedown', onTikla)
      document.removeEventListener('keydown', onTus)
    }
  }, [acik])

  useEffect(() => { if (acik) aramaRef.current?.focus() }, [acik])

  const aktif = domainler.find((d) => String(d.id) === aktifID)
  const suzulmus = useMemo(() => {
    const t = arama.trim().toLowerCase()
    if (!t) return domainler
    return domainler.filter((d) => d.alan_adi.toLowerCase().includes(t))
  }, [domainler, arama])

  function gec(id: number) {
    // Yoldaki kimliği değiştir, alt sayfayı olduğu gibi taşı.
    const altYol = konum.pathname.replace(/^\/abonelikler\/\d+/, '')
    setAcik(false)
    setArama('')
    navigate(`/abonelikler/${id}${altYol}`)
  }

  return (
    <div ref={kutuRef} className="relative px-2 pt-2">
      <button
        type="button"
        onClick={() => setAcik((s) => !s)}
        aria-haspopup="listbox"
        aria-expanded={acik}
        className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800 transition text-left"
      >
        <svg className="w-4 h-4 flex-shrink-0 text-brand-600 dark:text-brand-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.7}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span className="min-w-0 flex-1 truncate text-sm font-medium text-slate-900 dark:text-slate-100">
          {aktif?.alan_adi || 'Alan adı seç'}
        </span>
        <svg className={`w-3.5 h-3.5 flex-shrink-0 text-slate-400 transition-transform ${acik ? 'rotate-180' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {acik && (
        <div className="absolute left-2 right-2 top-full mt-1 z-30 rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 shadow-lg overflow-hidden">
          {domainler.length > 8 && (
            <div className="p-1.5 border-b border-slate-100 dark:border-slate-800">
              <input
                ref={aramaRef}
                value={arama}
                onChange={(e) => setArama(e.target.value)}
                placeholder="Ara…"
                className="w-full px-2 py-1.5 text-sm rounded-md bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
          )}
          <ul role="listbox" className="max-h-64 overflow-y-auto py-1">
            {suzulmus.length === 0 && (
              <li className="px-3 py-2 text-sm text-slate-400">Sonuç yok</li>
            )}
            {suzulmus.map((d) => {
              const secili = String(d.id) === aktifID
              return (
                <li key={d.id}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={secili}
                    onClick={() => gec(d.id)}
                    className={`w-full text-left px-3 py-1.5 text-sm truncate transition ${
                      secili
                        ? 'bg-slate-100 dark:bg-slate-800 text-slate-900 dark:text-slate-100 font-medium'
                        : 'text-slate-600 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/60'
                    }`}
                  >
                    {d.alan_adi}
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}
