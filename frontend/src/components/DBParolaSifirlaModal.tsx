// Veritabanı kullanıcı parolasını sıfırlama modalı — domain-özel ve sunucu
// geneli veritabanı sayfaları arasında paylaşılır (ikisi de aynı /databases/:id/password ucunu çağırır).
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Modal from './Modal'

type DB = { id: number; db_adi: string; db_kullanici: string }

export default function DBParolaSifirlaModal({ db, onKapat, onTamam }: {
  db: DB
  onKapat: () => void
  onTamam: () => void
}) {
  const { t } = useTranslation(['DBParolaSifirlaModal'])
  const [ozelPw, setOzelPw] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [yeniPw, setYeniPw] = useState<string | null>(null)

  async function sifirla(rastgele: boolean) {
    if (!rastgele && ozelPw.length < 6) {
      setHata(t('DBParolaSifirlaModal:custom_password_too_short'))
      return
    }
    setIsleniyor(true); setHata(null)
    try {
      const body = rastgele ? {} : { parola: ozelPw }
      const { data } = await api.put(`/databases/${db.id}/password`, body)
      setYeniPw(data.db_parola)
    } catch (e) {
      setHata(apiHata(e, t('DBParolaSifirlaModal:reset_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={true} baslik={t('DBParolaSifirlaModal:reset_modal_title', { ad: db.db_adi })} onKapat={yeniPw ? onTamam : onKapat} genislik="md">
      {!yeniPw ? (
        <div className="space-y-4">
          <div className="text-sm text-slate-600 dark:text-slate-400">
            {t('DBParolaSifirlaModal:reset_user_desc', { kullanici: db.db_kullanici })}
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DBParolaSifirlaModal:custom_password_label')}</label>
            <input
              type="text"
              value={ozelPw}
              onChange={e => setOzelPw(e.target.value)}
              placeholder={t('DBParolaSifirlaModal:custom_password_placeholder')}
              className="ta-input w-full font-mono"
            />
          </div>
          {hata && <div className="ta-form-error">{hata}</div>}
          <div className="ta-form-actions">
            <button onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('DBParolaSifirlaModal:cancel')}</button>
            <button onClick={() => sifirla(false)} disabled={isleniyor || !ozelPw} className="ta-secondary-button border-brand-600 text-brand-700 dark:text-brand-300">{t('DBParolaSifirlaModal:set_this')}</button>
            <button onClick={() => sifirla(true)} disabled={isleniyor} className="ta-primary-button">{isleniyor ? t('DBParolaSifirlaModal:resetting') : t('DBParolaSifirlaModal:generate_random')}</button>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="ta-form-success">
            <p className="font-medium mb-2">{t('DBParolaSifirlaModal:password_updated')}</p>
            <p className="text-xs mb-2 opacity-90">{t('DBParolaSifirlaModal:password_updated_save')}</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-white dark:bg-slate-900 px-3 py-2 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{yeniPw}</code>
              <button onClick={() => navigator.clipboard.writeText(yeniPw)} className="px-3 py-2 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded">{t('DBParolaSifirlaModal:copy')}</button>
            </div>
          </div>
          <div className="flex justify-end">
            <button onClick={onTamam} className="ta-primary-button">{t('DBParolaSifirlaModal:ok')}</button>
          </div>
        </div>
      )}
    </Modal>
  )
}
