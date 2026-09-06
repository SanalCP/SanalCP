import { StrictMode } from 'react'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { Link, MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import DialogHost from './DialogHost'
import Modal from './Modal'
import { modalGirdi, modalOnay, modalUyari } from '@/lib/dialog'
import { panoYaz } from '@/lib/pano'

function kur() {
  return render(<StrictMode><MemoryRouter><Link to="/sonraki">Sonraki</Link><DialogHost /></MemoryRouter></StrictMode>)
}

describe('ortak modal akışı', () => {
  it('onay gelmeden işlemi başlatmaz; iptal hiçbir işlem yapmaz', async () => {
    kur()
    const islem = vi.fn()
    async function sil() { if (await modalOnay('Dosyayı sil?')) islem() }
    let sonuc!: Promise<void>
    act(() => { sonuc = sil() })
    expect(islem).not.toHaveBeenCalled()
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'cancel' }))
    fireEvent.click(screen.getByRole('button', { name: 'cancel' }))
    await sonuc
    expect(islem).not.toHaveBeenCalled()

    act(() => { sonuc = sil() })
    fireEvent.click(screen.getByRole('button', { name: 'confirm' }))
    await sonuc
    expect(islem).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it.each(['close', 'escape', 'backdrop'])('%s ile kapatınca onayı reddeder', async yontem => {
    kur()
    let sonuc!: Promise<boolean>
    act(() => { sonuc = modalOnay('Sil?') })
    if (yontem === 'close') fireEvent.click(screen.getByRole('button', { name: 'close' }))
    if (yontem === 'escape') fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }))
    if (yontem === 'backdrop') fireEvent.click(screen.getByRole('dialog'), { clientX: -10, clientY: -10 })
    expect(await sonuc).toBe(false)
  })

  it('Tab ve Shift+Tab odağı modal içinde döndürür', async () => {
    kur()
    let sonuc!: Promise<boolean>
    act(() => { sonuc = modalOnay('Onay?') })
    const ilk = screen.getByRole('button', { name: 'close' })
    const son = screen.getByRole('button', { name: 'confirm' })
    son.focus()
    fireEvent.keyDown(son, { key: 'Tab' })
    expect(document.activeElement).toBe(ilk)
    fireEvent.keyDown(ilk, { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(son)
    fireEvent.click(screen.getByRole('button', { name: 'cancel' }))
    expect(await sonuc).toBe(false)
  })

  it('girdi varsayılanını seçer; boş metni iptalden ayırır', async () => {
    kur()
    let sonuc!: Promise<string | null>
    act(() => { sonuc = modalGirdi('Klasör adı', 'eski') })
    const input = screen.getByLabelText('dialog.value') as HTMLInputElement
    expect(document.activeElement).toBe(input)
    expect(input.value.slice(input.selectionStart!, input.selectionEnd!)).toBe('eski')
    fireEvent.change(input, { target: { value: '' } })
    fireEvent.submit(input.closest('form')!)
    expect(await sonuc).toBe('')

    act(() => { sonuc = modalGirdi('Klasör adı') })
    fireEvent.click(screen.getByRole('button', { name: 'cancel' }))
    expect(await sonuc).toBeNull()
  })

  it('gizli değer girişini parola alanında gösterir', async () => {
    kur()
    let sonuc!: Promise<string | null>
    act(() => { sonuc = modalGirdi('APP_KEY', '', { gizli: true }) })
    const input = screen.getByLabelText('dialog.value') as HTMLInputElement
    expect(input.type).toBe('password')
    fireEvent.change(input, { target: { value: 'gizli-deger' } })
    fireEvent.submit(input.closest('form')!)
    expect(await sonuc).toBe('gizli-deger')
  })

  it('eşzamanlı uyarıları sırayla gösterir, metni HTML olarak çalıştırmaz', async () => {
    kur()
    let ilk!: Promise<void>, ikinci!: Promise<void>
    act(() => {
      ilk = modalUyari('İlk hata\n<img src=x onerror=evil()>')
      ikinci = modalUyari('İkinci hata')
    })
    expect(screen.getAllByRole('dialog')).toHaveLength(1)
    expect(screen.queryByText('İkinci hata')).toBeNull()
    expect(screen.getByRole('dialog').querySelector('img')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'ok' }))
    await ilk
    expect(screen.getByText('İkinci hata')).toBeTruthy()
    expect(document.body.style.overflow).toBe('hidden')
    fireEvent.click(screen.getByRole('button', { name: 'ok' }))
    await ikinci
    expect(document.body.style.overflow).toBe('')
  })

  it('sayfa değişiminde açık ve bekleyen istekleri iptal eder', async () => {
    kur()
    let onay!: Promise<boolean>, girdi!: Promise<string | null>
    act(() => { onay = modalOnay('Sil?'); girdi = modalGirdi('Değer?') })
    fireEvent.click(screen.getByRole('link', { name: 'Sonraki' }))
    expect(await onay).toBe(false)
    expect(await girdi).toBeNull()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('host kaldırıldığında bekleyen onayı iptal eder', async () => {
    const { unmount } = kur()
    let sonuc!: Promise<boolean>
    act(() => { sonuc = modalOnay('Sil?') })
    unmount()
    expect(await sonuc).toBe(false)
    expect(document.body.style.overflow).toBe('')
  })

  it('üst modalda Escape alttaki formu kapatmaz ve odağı geri verir', async () => {
    const kapat = vi.fn()
    render(<MemoryRouter><Modal acik baslik="Form" onKapat={kapat}>
      <button onClick={() => { void modalUyari('Hata') }}>Kaydet</button>
    </Modal><DialogHost /></MemoryRouter>)
    const kaydet = screen.getByRole('button', { name: 'Kaydet' })
    kaydet.focus()
    fireEvent.click(kaydet)
    const ust = screen.getByRole('dialog', { name: 'dialog.uyari_title' })
    fireEvent.keyDown(within(ust).getByRole('button', { name: 'ok' }), { key: 'Escape' })
    fireEvent(ust, new Event('cancel', { cancelable: true }))
    expect(kapat).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog', { name: 'Form' })).toBeTruthy()
    expect(document.activeElement).toBe(kaydet)
    expect(document.body.style.overflow).toBe('hidden')
  })

  it('panoya yazma başarısızsa seçili metinli modal açar, başarı bildirmez', async () => {
    kur()
    let sonuc!: Promise<boolean>
    act(() => { sonuc = panoYaz('ilk satır\nikinci satır', 'Elle kopyalayın') })
    const metin = await screen.findByLabelText('dialog.copy_value') as HTMLTextAreaElement
    expect(metin.readOnly).toBe(true)
    expect(metin.value).toBe('ilk satır\nikinci satır')
    expect(metin.selectionStart).toBe(0)
    expect(metin.selectionEnd).toBe(metin.value.length)
    fireEvent.submit(metin.closest('form')!)
    expect(await sonuc).toBe(false)
  })
})
