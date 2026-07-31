// sanal-dark-swept
// sanal-dark-swept-v2
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import Modal from './Modal'

export default function ConfirmDialog({
  acik, baslik, mesaj, onayMetni, tehlikeli = false,
  onOnay, onIptal,
}: {
  acik: boolean
  baslik: string
  mesaj: string
  onayMetni?: string
  tehlikeli?: boolean
  onOnay: () => Promise<void> | void
  onIptal: () => void
}) {
  const { t } = useTranslation(['common'])
  const [yukleniyor, setYukleniyor] = useState(false)

  async function onaylaTetik() {
    setYukleniyor(true)
    try { await onOnay() } finally { setYukleniyor(false) }
  }

  return (
    <Modal acik={acik} baslik={baslik} onKapat={onIptal} genislik="sm">
      <p className="text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-5">{mesaj}</p>
      <div className="flex justify-end gap-2">
        <button onClick={onIptal} disabled={yukleniyor} className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 rounded-md text-sm">
          {t('common:cancel')}
        </button>
        <button
          onClick={onaylaTetik}
          disabled={yukleniyor}
          className={`px-4 py-2 text-white rounded-md text-sm font-medium ${
            tehlikeli ? 'bg-red-600 hover:bg-red-700 disabled:bg-red-300' : 'bg-brand-600 hover:bg-brand-700 disabled:opacity-60'
          }`}
        >
          {yukleniyor ? t('common:processing') : (onayMetni ?? t('common:confirm'))}
        </button>
      </div>
    </Modal>
  )
}