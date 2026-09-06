import { useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore } from 'react'
import { createPortal } from 'react-dom'
import { useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { aktifDialog, dialogBitir, dialogDinle, dialoglariIptalEt, type DialogIstek } from '@/lib/dialog'

// Uygulama genelinde tek kuyruk: eşzamanlı hatalar birbirini ezmez.
export default function DialogHost() {
  const istek = useSyncExternalStore(dialogDinle, aktifDialog)
  const { key } = useLocation()

  // Sayfadan ayrılınca eski bir işlemin onayını sonraki sayfaya taşımayız.
  useLayoutEffect(() => () => dialoglariIptalEt(), [key])

  return istek ? createPortal(<DialogPencere key={istek.id} istek={istek} />, document.body) : null
}

function DialogPencere({ istek }: { istek: DialogIstek }) {
  const { t } = useTranslation('common')
  const baslikId = useId()
  const mesajId = useId()
  const girdiId = useId()
  const dialogRef = useRef<HTMLDialogElement>(null)
  const girdiRef = useRef<HTMLInputElement>(null)
  const kopyaRef = useRef<HTMLTextAreaElement>(null)
  const guvenliButonRef = useRef<HTMLButtonElement>(null)
  const [deger, setDeger] = useState(istek.deger)
  const iptal = () => dialogBitir(istek.id, null)
  const girdi = istek.tip === 'girdi'
  const kopya = istek.tip === 'kopyala'
  const onay = istek.tip === 'onay'

  useEffect(() => {
    const dialog = dialogRef.current!
    const oncekiOdak = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const oncekiOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    // Tarayıcının üst katmanı, mevcut formların/modalların üstünde görünür;
    // arka planı etkileşime kapatır ve klavye odağını pencerede tutar.
    dialog.showModal()
    const odak = girdiRef.current ?? kopyaRef.current ?? guvenliButonRef.current
    odak?.focus()
    girdiRef.current?.select()
    kopyaRef.current?.select()
    return () => {
      dialog.close()
      document.body.style.overflow = oncekiOverflow
      if (oncekiOdak?.isConnected) oncekiOdak.focus()
    }
  }, [])

  return (
    // Escape ve arka plan tıklaması, erişilebilir kapatma düğmesinin alternatifleridir.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <dialog
      ref={dialogRef}
      aria-modal="true"
      aria-labelledby={baslikId}
      aria-describedby={mesajId}
      onCancel={e => { e.preventDefault(); iptal() }}
      onKeyDown={e => {
        // Alttaki mevcut Modal'ın window Escape dinleyicisini tetikleme.
        if (e.key === 'Escape') e.stopPropagation()
        if (e.key === 'Tab') {
          const alanlar = Array.from(e.currentTarget.querySelectorAll<HTMLElement>(
            'button:not([disabled]), input:not([disabled]), textarea:not([disabled])',
          ))
          const ilk = alanlar[0]
          const son = alanlar[alanlar.length - 1]
          if (e.shiftKey && document.activeElement === ilk) {
            e.preventDefault()
            son?.focus()
          } else if (!e.shiftKey && document.activeElement === son) {
            e.preventDefault()
            ilk?.focus()
          }
        }
      }}
      onClick={e => {
        if (e.target !== e.currentTarget) return
        const rect = e.currentTarget.getBoundingClientRect()
        if (e.clientX < rect.left || e.clientX > rect.right || e.clientY < rect.top || e.clientY > rect.bottom) iptal()
      }}
      className="m-auto max-h-[90dvh] w-[calc(100%_-_2rem)] max-w-md overflow-auto rounded-2xl border border-white/20 bg-white p-0 text-slate-900 shadow-2xl backdrop:bg-slate-950/50 backdrop:backdrop-blur-sm dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
    >
      <div className="flex items-center justify-between gap-4 border-b border-slate-100 px-6 py-5 dark:border-slate-800">
        <h2 id={baslikId} className="text-base font-semibold">{t(`dialog.${istek.tip}_title`)}</h2>
        <button type="button" onClick={iptal} className="ta-icon-button h-9 w-9 shrink-0 border-0 shadow-none" aria-label={t('close')}>
          <svg aria-hidden="true" className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <form className="p-6" onSubmit={e => {
        e.preventDefault()
        dialogBitir(istek.id, girdi ? deger : true)
      }}>
        <p id={mesajId} className="whitespace-pre-wrap break-words text-sm text-slate-600 dark:text-slate-400">{istek.mesaj}</p>
        {girdi && <div className="mt-4">
          <label htmlFor={girdiId} className="ta-label">{t('dialog.value')}</label>
          <input ref={girdiRef} id={girdiId} type={istek.gizli ? 'password' : 'text'} value={deger} onChange={e => setDeger(e.target.value)} autoComplete="off" className="ta-input w-full" />
        </div>}
        {kopya && <div className="mt-4">
          <label htmlFor={girdiId} className="ta-label">{t('dialog.copy_value')}</label>
          <textarea ref={kopyaRef} id={girdiId} readOnly value={istek.deger} rows={6} className="ta-input w-full font-mono text-sm" onFocus={e => e.target.select()} />
          <p className="ta-hint mt-2">{t('dialog.copy_hint')}</p>
        </div>}
        <div className="ta-form-actions mt-6">
          {(girdi || onay) && <button ref={guvenliButonRef} type="button" onClick={iptal} className="ta-secondary-button">{t('cancel')}</button>}
          <button ref={girdi || onay ? undefined : guvenliButonRef} type="submit" className="ta-primary-button">{t(onay ? 'confirm' : kopya ? 'close' : 'ok')}</button>
        </div>
      </form>
    </dialog>
  )
}
