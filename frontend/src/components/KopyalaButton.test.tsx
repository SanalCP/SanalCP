import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import KopyalaButton from './KopyalaButton'
import DialogHost from './DialogHost'

const writeText = vi.fn()
const execCommand = vi.fn()
const eskiPano = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
const eskiKomut = Object.getOwnPropertyDescriptor(document, 'execCommand')

beforeEach(() => {
  writeText.mockReset().mockResolvedValue(undefined)
  execCommand.mockReset().mockReturnValue(false)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
  vi.stubGlobal('isSecureContext', true)
})

afterEach(() => {
  if (eskiPano) Object.defineProperty(navigator, 'clipboard', eskiPano)
  else Reflect.deleteProperty(navigator, 'clipboard')
  if (eskiKomut) Object.defineProperty(document, 'execCommand', eskiKomut)
  else Reflect.deleteProperty(document, 'execCommand')
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('kopyalama geri bildirimi', () => {
  it('panoya yazma bitmeden başarı göstermez ve çift tıklamayı engeller', async () => {
    let tamamla!: () => void
    writeText.mockImplementationOnce(() => new Promise<void>(resolve => { tamamla = resolve }))
    render(<KopyalaButton metin="ssh test@sunucu" />)
    const button = screen.getByRole('button', { name: 'copy' }) as HTMLButtonElement
    fireEvent.click(button)
    expect(button.disabled).toBe(true)
    expect(screen.getByRole('status').textContent).toBe('copying')
    fireEvent.click(button)
    expect(writeText).toHaveBeenCalledTimes(1)
    await act(async () => { tamamla() })
    expect(writeText).toHaveBeenCalledWith('ssh test@sunucu')
    expect(screen.getByRole('status').textContent).toContain('copied')
    expect(button.className).toContain('ring-emerald')
    expect(button.disabled).toBe(false)
  })

  it('başarı iki saniye görünür, tekrar kopyalama süreyi baştan başlatır', async () => {
    vi.useFakeTimers()
    render(<KopyalaButton metin="parola" />)
    const button = screen.getByRole('button')
    await act(async () => { fireEvent.click(button) })
    act(() => { vi.advanceTimersByTime(1500) })
    await act(async () => { fireEvent.click(button) })
    act(() => { vi.advanceTimersByTime(600) })
    expect(screen.getByRole('status').textContent).toContain('copied')
    act(() => { vi.advanceTimersByTime(1400) })
    expect(button.textContent).toBe('copy')
  })

  it('aynı değere sahip satırlarda yalnız tıklanan butonu günceller', async () => {
    render(<><KopyalaButton metin="localhost" /><KopyalaButton metin="localhost" /></>)
    const buttons = screen.getAllByRole('button')
    await act(async () => { fireEvent.click(buttons[0]) })
    expect(buttons[0].textContent).toContain('copied')
    expect(buttons[1].textContent).toBe('copy')
  })

  it('gizlenmiş parolayı açığa çıkarmadan gerçek değeri kopyalar', async () => {
    render(<KopyalaButton metin="gizli-parola" icerigiKoru>••••••</KopyalaButton>)
    await act(async () => { fireEvent.click(screen.getByRole('button')) })
    expect(writeText).toHaveBeenCalledWith('gizli-parola')
    expect(screen.getByRole('button').textContent).toContain('••••••')
    expect(screen.queryByText('gizli-parola')).toBeNull()
  })

  it('değer değiştiğinde eski başarıyı ve bekleyen sonucu yeni değere taşımaz', async () => {
    let tamamla!: () => void
    writeText.mockImplementationOnce(() => new Promise<void>(resolve => { tamamla = resolve }))
    const { rerender } = render(<KopyalaButton metin="eski-token" />)
    fireEvent.click(screen.getByRole('button'))
    rerender(<KopyalaButton metin="yeni-token" />)
    await act(async () => { tamamla() })
    expect(screen.getByRole('button').textContent).toBe('copy')
    await act(async () => { fireEvent.click(screen.getByRole('button')) })
    expect(writeText).toHaveBeenLastCalledWith('yeni-token')
    expect(screen.getByRole('status').textContent).toContain('copied')
    rerender(<KopyalaButton metin="baska-token" />)
    expect(screen.getByRole('button').textContent).toBe('copy')
  })

  it('buton kaldırılınca geri bildirim zamanlayıcısını temizler', async () => {
    vi.useFakeTimers()
    const { unmount } = render(<KopyalaButton metin="test" />)
    await act(async () => { fireEvent.click(screen.getByRole('button')) })
    expect(vi.getTimerCount()).toBe(1)
    unmount()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('clipboard izni reddedilince alternatif kopyalamanın sonucunu bekler', async () => {
    writeText.mockRejectedValueOnce(new Error('izin yok'))
    execCommand.mockImplementationOnce(() => {
      expect((document.activeElement as HTMLTextAreaElement).value).toBe('test')
      return true
    })
    render(<KopyalaButton metin="test" />)
    await act(async () => { fireEvent.click(screen.getByRole('button')) })
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(screen.getByRole('status').textContent).toContain('copied')
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('HTTP ortamında Clipboard API olmadan kopyalar ve sonucu bildirir', async () => {
    vi.stubGlobal('isSecureContext', false)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    execCommand.mockReturnValueOnce(true)
    render(<KopyalaButton metin="test" />)
    await act(async () => { fireEvent.click(screen.getByRole('button')) })
    expect(writeText).not.toHaveBeenCalled()
    expect(screen.getByRole('status').textContent).toContain('copied')
  })

  it('otomatik kopyalama başarısızsa modal açar ve yanlış başarı göstermez', async () => {
    writeText.mockRejectedValueOnce(new Error('izin yok'))
    render(<MemoryRouter><KopyalaButton metin="elle kopyalanacak" /><DialogHost /></MemoryRouter>)
    const button = screen.getByRole('button', { name: 'copy' })
    await act(async () => { fireEvent.click(button) })
    const dialog = screen.getByRole('dialog')
    expect((within(dialog).getByLabelText('dialog.copy_value') as HTMLTextAreaElement).value).toBe('elle kopyalanacak')
    await act(async () => { fireEvent.submit(dialog.querySelector('form')!) })
    expect(button.textContent).toBe('copy_manual')
    expect(button.className).not.toContain('ring-emerald')
  })
})
