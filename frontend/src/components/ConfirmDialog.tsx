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
      <p className="mb-5 text-sm text-slate-600 dark:text-slate-400">{mesaj}</p>
      <div className="ta-form-actions">
        <button onClick={onIptal} disabled={yukleniyor} className="ta-secondary-button">
          {t('common:cancel')}
        </button>
        <button
          onClick={onaylaTetik}
          disabled={yukleniyor}
          className={tehlikeli ? 'ta-danger-button' : 'ta-primary-button'}
        >
          {yukleniyor ? t('common:processing') : (onayMetni ?? t('common:confirm'))}
        </button>
      </div>
    </Modal>
  )
}
