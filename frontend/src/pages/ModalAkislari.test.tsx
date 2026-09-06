import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DialogHost from '@/components/DialogHost'
import DomainCronPage from './DomainCronPage'
import DomainFilesPage from './DomainFilesPage'
import DomainMailPage from './DomainMailPage'

const { get, post, sil } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), sil: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get, post, delete: sil }, apiHata: () => 'Sunucu işlemi reddetti' }))

beforeEach(() => {
  get.mockReset()
  post.mockReset().mockResolvedValue({ data: {} })
  sil.mockReset().mockResolvedValue({ data: {} })
  get.mockImplementation(async (url: string) => {
    if (url === '/domains/7') return { data: { id: 7, alan_adi: 'ornek.test', sistem_kullanici: 'c_ornek' } }
    if (url.endsWith('/cron')) return { data: { gorevler: [{ idx: 2, dakika: '*', saat: '*', gun: '*', ay: '*', hafta: '*', komut: 'php artisan schedule:run' }] } }
    if (url.endsWith('/files')) return { data: { yol: '/public_html', icerik: [], toplam: 0 } }
    if (url.endsWith('/mail/durum')) return { data: { etkin: true } }
    if (url.endsWith('/mail/spam')) return { data: { settings: { enabled: true, greylist_score: 4, add_header_score: 6, reject_score: 15 }, rspamd: false } }
    return { data: [] }
  })
})

function sayfa(element: React.ReactNode, slug: string) {
  render(<MemoryRouter initialEntries={[`/abonelikler/7/${slug}`]}>
    <Routes><Route path={`/abonelikler/:id/${slug}`} element={element} /></Routes>
    <DialogHost />
  </MemoryRouter>)
}

describe('sayfalarda modal ve API sıralaması', () => {
  it('cron silme onayı bekler, iptal eder ve API hatasını modalda gösterir', async () => {
    sayfa(<DomainCronPage />, 'cron')
    const silButonu = await screen.findByRole('button', { name: 'DomainCronPage:table.delete' })
    fireEvent.click(silButonu)
    expect(sil).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'cancel' }))
    expect(sil).not.toHaveBeenCalled()
    sil.mockRejectedValueOnce(new Error('reddedildi'))
    fireEvent.click(silButonu)
    fireEvent.click(screen.getByRole('button', { name: 'confirm' }))
    await waitFor(() => expect(sil).toHaveBeenCalledWith('/domains/7/cron/2'))
    expect(await screen.findByText('Sunucu işlemi reddetti')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'ok' }))
  })

  it('klasör oluşturmada iptal API çağırmaz, girilen ad doğru yola gönderilir', async () => {
    sayfa(<DomainFilesPage />, 'dosyalar')
    const yeni = await screen.findByRole('button', { name: 'DomainFilesPage:newMenu.button' })
    function ac() {
      fireEvent.click(yeni)
      fireEvent.click(screen.getByRole('button', { name: 'DomainFilesPage:newMenu.newFolder' }))
    }
    ac()
    expect(post).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'cancel' }))
    expect(post).not.toHaveBeenCalled()
    ac()
    const input = screen.getByLabelText('dialog.value')
    fireEvent.change(input, { target: { value: 'yeni-klasor' } })
    fireEvent.submit(input.closest('form')!)
    await waitFor(() => expect(post).toHaveBeenCalledWith('/domains/7/files/mkdir', { yol: '/public_html/yeni-klasor' }))
  })

  it('posta hizmetini kaldırmak için doğru alan adının yazılmasını zorunlu tutar', async () => {
    sayfa(<DomainMailPage />, 'mail')
    const kaldir = await screen.findByRole('button', { name: 'DomainMailPage:purge.button' })
    fireEvent.click(kaldir)
    fireEvent.click(screen.getByRole('button', { name: 'cancel' }))
    expect(sil).not.toHaveBeenCalled()
    fireEvent.click(kaldir)
    let input = screen.getByLabelText('dialog.value')
    fireEvent.change(input, { target: { value: 'yanlis.test' } })
    fireEvent.submit(input.closest('form')!)
    expect(await screen.findByText('DomainMailPage:purge.confirm_mismatch')).toBeTruthy()
    expect(sil).not.toHaveBeenCalled()
    fireEvent.click(kaldir)
    input = screen.getByLabelText('dialog.value')
    fireEvent.change(input, { target: { value: 'ornek.test' } })
    fireEvent.submit(input.closest('form')!)
    await waitFor(() => expect(sil).toHaveBeenCalledWith('/domains/7/mail/hizmet'))
    expect(sil).toHaveBeenCalledTimes(1)
  })
})
