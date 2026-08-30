import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PanelErisimAyari from './PanelErisimAyari'
import PanelHizLimitiAyari from './PanelHizLimitiAyari'
import DomainRateLimitPage from '@/pages/DomainRateLimitPage'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get, put: vi.fn() }, apiHata: () => 'api-hatasi' }))

beforeEach(() => get.mockReset())

describe('boş liste API sözleşmeleri', () => {
  it('panel erişim cidrler=null yanıtında çökmüyor', async () => {
    get.mockResolvedValueOnce({ data: { cidrler: null, istemci_ip: '127.0.0.1', gecici_cidr: '', gecici_bitis: '' } })
    const { container } = render(<PanelErisimAyari />)
    await waitFor(() => expect(get).toHaveBeenCalledWith('/system/panel-erisim'))
    expect(container.querySelector('textarea')?.value).toBe('')
  })

  it('panel hız limiti null listeleri boş diziye çeviriyor', async () => {
    get.mockResolvedValueOnce({ data: { ayar: { profil: 'dengeli', istek_dakika: 600, burst: 100, ip_istisnalari: null }, olaylar: null, istemci_ip: '127.0.0.1' } })
    const { container } = render(<PanelHizLimitiAyari />)
    await waitFor(() => expect(screen.getByText('PanelHizLimitiAyari:title')).toBeTruthy())
    expect(container.querySelector('textarea')?.value).toBe('')
  })

  it('domain rate limit null istisnalarıyla render oluyor', async () => {
    get.mockResolvedValueOnce({ data: { alan_adi: 'ornek.test', ayar: { profil: 'kapali', istek_dakika: 120, burst: 30, bot_engelle: false, ip_istisnalari: null, yol_istisnalari: null }, olaylar: null } })
    const { container } = render(<MemoryRouter initialEntries={['/abonelikler/7/rate-limit']}><Routes><Route path="/abonelikler/:id/rate-limit" element={<DomainRateLimitPage/>}/></Routes></MemoryRouter>)
    await waitFor(() => expect(screen.getAllByText('ornek.test').length).toBeGreaterThan(0))
    expect([...container.querySelectorAll('textarea')].map(x => x.value)).toEqual(['', ''])
  })
})
