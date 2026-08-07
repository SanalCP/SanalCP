// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useId, useRef } from 'react'

export default function Modal({
  acik, baslik, onKapat, children, genislik = 'md', kapatEtiketi = 'Kapat',
}: {
  acik: boolean
  baslik: string
  onKapat: () => void
  children: React.ReactNode
  genislik?: 'sm' | 'md' | 'lg'
  kapatEtiketi?: string
}) {
  const baslikId = useId()
  const dialogRef = useRef<HTMLDivElement>(null)
  const onKapatRef = useRef(onKapat)
  onKapatRef.current = onKapat

  useEffect(() => {
    if (!acik) return
    const oncekiOdak = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const oncekiOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    requestAnimationFrame(() => dialogRef.current?.focus())

    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault()
        onKapatRef.current()
        return
      }
      if (e.key !== 'Tab' || !dialogRef.current) return
      const odaklanabilir = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )).filter((el) => !el.hasAttribute('hidden'))
      if (odaklanabilir.length === 0) {
        e.preventDefault()
        dialogRef.current.focus()
        return
      }
      const ilk = odaklanabilir[0]
      const son = odaklanabilir[odaklanabilir.length - 1]
      if (e.shiftKey && document.activeElement === ilk) {
        e.preventDefault()
        son.focus()
      } else if (!e.shiftKey && document.activeElement === son) {
        e.preventDefault()
        ilk.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = oncekiOverflow
      oncekiOdak?.focus()
    }
  }, [acik])

  if (!acik) return null
  const w = genislik === 'sm' ? 'max-w-sm' : genislik === 'lg' ? 'max-w-2xl' : 'max-w-md'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center px-4">
      <div className="absolute inset-0 bg-slate-950/50 backdrop-blur-sm" onClick={onKapat} aria-hidden="true" />
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={baslikId}
        tabIndex={-1}
        className={`relative max-h-[90dvh] w-full ${w} overflow-auto rounded-2xl border border-white/20 bg-white shadow-2xl outline-none dark:border-slate-700 dark:bg-slate-900`}
      >
        <div className="flex items-center justify-between px-6 py-5 border-b border-slate-100 dark:border-slate-800">
          <h3 id={baslikId} className="text-base font-semibold text-slate-900 dark:text-slate-100">{baslik}</h3>
          <button type="button" onClick={onKapat} className="ta-icon-button h-9 w-9 border-0 shadow-none" aria-label={kapatEtiketi}>
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="p-6">{children}</div>
      </div>
    </div>
  )
}
