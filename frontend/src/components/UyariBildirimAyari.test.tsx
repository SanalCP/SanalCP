import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UyariBildirimAyari from './UyariBildirimAyari'

const { get, put, post } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn() }))
vi.mock('@/lib/api', () => ({
  api: { get, put, post },
  apiHata: (e: { response?: { data?: { hata?: string } } }, v: string) => e?.response?.data?.hata || v,
}))

beforeEach(() => {
  get.mockReset(); put.mockReset(); post.mockReset()
  get.mockResolvedValue({ data: { acik: true, telegram_token_ayarli: true, telegram_chat_id: '42', eposta: '' } })
})

describe('UyariBildirimAyari', () => {
  it('kayıtlı token\'ı göstermez, boş bırakılınca "" gönderir', async () => {
    put.mockResolvedValue({ data: { ok: true } })
    render(<UyariBildirimAyari />)
    const token = await screen.findByLabelText('UyariBildirimAyari:token') as HTMLInputElement
    expect(token.value).toBe('')
    expect(token.placeholder).toBe('UyariBildirimAyari:token_saved')
    fireEvent.click(screen.getByText('UyariBildirimAyari:save'))
    await waitFor(() => expect(put).toHaveBeenCalledWith('/system/uyari-bildirim',
      { acik: true, telegram_token: '', telegram_chat_id: '42', eposta: '' }))
  })

  it('test butonu hatayı gösterir', async () => {
    post.mockRejectedValue({ response: { data: { hata: 'bildirim kanalı ayarlı değil' } } })
    render(<UyariBildirimAyari />)
    fireEvent.click(await screen.findByText('UyariBildirimAyari:test'))
    expect(await screen.findByText('bildirim kanalı ayarlı değil')).toBeTruthy()
  })
})
