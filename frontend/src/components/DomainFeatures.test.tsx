import { useEffect } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Link, MemoryRouter, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DomainFeatureGate, DomainFeaturesProvider } from './DomainFeatures'
import { useDomainFeatures } from '@/lib/domainFeaturesContext'
import { allPlanFeatures } from '@/lib/planFeatures'
import { useAuth } from '@/store/auth'

const { get, mounted } = vi.hoisted(() => ({ get: vi.fn(), mounted: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))

function ManagedPage() {
  const { pathname } = useLocation()
  useEffect(() => { mounted(pathname) }, [pathname])
  return <div>managed page {pathname}</div>
}

function Harness() {
  const { allows } = useDomainFeatures()
  return <>
    <Link to="/abonelikler/2/dosyalar">switch domain</Link>
    <Link to="/abonelikler/1/veritabanlari/example">database detail</Link>
    {allows('files') && <span>files navigation</span>}
    <DomainFeatureGate><ManagedPage /></DomainFeatureGate>
  </>
}

function open(path = '/abonelikler/1/dosyalar') {
  render(<MemoryRouter initialEntries={[path]}><DomainFeaturesProvider><Harness /></DomainFeaturesProvider></MemoryRouter>)
}

beforeEach(() => {
  get.mockReset()
  mounted.mockReset()
  localStorage.clear()
  useAuth.setState({ kullanici: { id: 3, adi: 'customer', rol: 'user' } })
})

describe('domain feature authorization transitions', () => {
  it('does not mount module effects while unknown or disabled, then retries successfully', async () => {
    let resolve!: (value: unknown) => void
    get.mockReturnValueOnce(new Promise(done => { resolve = done }))
    open()
    expect(mounted).not.toHaveBeenCalled()
    expect(screen.queryByText('files navigation')).toBeNull()
    await act(async () => resolve({ data: { features: { ...allPlanFeatures(), files: false }, enforced: true } }))
    expect(await screen.findByText('plan_features.blocked_title')).toBeTruthy()
    expect(mounted).not.toHaveBeenCalled()
    get.mockResolvedValueOnce({ data: { features: allPlanFeatures(), enforced: true } })
    fireEvent.click(screen.getByRole('button', { name: 'plan_features.retry' }))
    expect(await screen.findByText('managed page /abonelikler/1/dosyalar')).toBeTruthy()
    expect(mounted).toHaveBeenCalledTimes(1)
  })

  it('drops the previous domain grant immediately and ignores a late previous response', async () => {
    let first!: (value: unknown) => void
    let second!: (value: unknown) => void
    get.mockReturnValueOnce(new Promise(done => { first = done }))
    get.mockReturnValueOnce(new Promise(done => { second = done }))
    open()
    fireEvent.click(screen.getByText('switch domain'))
    await waitFor(() => expect(get).toHaveBeenCalledWith('/domains/2/features', expect.anything()))
    await act(async () => first({ data: { features: allPlanFeatures(), enforced: true } }))
    expect(mounted).not.toHaveBeenCalled()
    expect(screen.queryByText('files navigation')).toBeNull()
    await act(async () => second({ data: { features: { ...allPlanFeatures(), files: false }, enforced: true } }))
    expect(await screen.findByText('plan_features.blocked_title')).toBeTruthy()
    expect(mounted).not.toHaveBeenCalled()
  })

  it('rechecks navigation in the same domain and blocks a nested database bookmark after plan changes', async () => {
    get.mockResolvedValueOnce({ data: { features: allPlanFeatures(), enforced: true } })
    open()
    await screen.findByText('files navigation')
    get.mockResolvedValueOnce({ data: { features: { ...allPlanFeatures(), databases: false }, enforced: true } })
    fireEvent.click(screen.getByText('database detail'))
    expect(await screen.findByText('plan_features.blocked_title')).toBeTruthy()
    expect(mounted).not.toHaveBeenCalledWith('/abonelikler/1/veritabanlari/example')
    expect(get).toHaveBeenCalledTimes(2)
  })
  it('does not reuse an already granted module when switching to another domain', async () => {
    get.mockResolvedValueOnce({ data: { features: allPlanFeatures(), enforced: true } })
    open()
    await screen.findByText('managed page /abonelikler/1/dosyalar')
    let resolve!: (value: unknown) => void
    get.mockReturnValueOnce(new Promise(done => { resolve = done }))
    fireEvent.click(screen.getByText('switch domain'))
    expect(screen.queryByText('files navigation')).toBeNull()
    expect(screen.queryByText('managed page /abonelikler/2/dosyalar')).toBeNull()
    expect(mounted).not.toHaveBeenCalledWith('/abonelikler/2/dosyalar')
    await act(async () => resolve({ data: { features: { ...allPlanFeatures(), files: false }, enforced: true } }))
    expect(await screen.findByText('plan_features.blocked_title')).toBeTruthy()
    expect(mounted).not.toHaveBeenCalledWith('/abonelikler/2/dosyalar')
  })


  it('uses the server enforcement flag rather than restricting support sessions by plan flags', async () => {
    const features = Object.fromEntries(Object.keys(allPlanFeatures()).map(key => [key, false]))
    get.mockResolvedValueOnce({ data: { features, enforced: false } })
    open('/abonelikler/1/veritabanlari/example')
    expect(await screen.findByText('managed page /abonelikler/1/veritabanlari/example')).toBeTruthy()
  })

  it.each([null, { features: { files: true }, enforced: true }])('fails closed for invalid feature responses: %s', async data => {
    get.mockResolvedValueOnce({ data })
    open()
    expect(await screen.findByText('plan_features.load_failed')).toBeTruthy()
    expect(mounted).not.toHaveBeenCalled()
  })

  it('fails closed on request errors and permits retry without logging in again', async () => {
    get.mockRejectedValueOnce(new Error('offline'))
    open()
    expect(await screen.findByText('plan_features.load_failed')).toBeTruthy()
    expect(mounted).not.toHaveBeenCalled()
    get.mockResolvedValueOnce({ data: { features: allPlanFeatures(), enforced: true } })
    fireEvent.click(screen.getByRole('button', { name: 'plan_features.retry' }))
    expect(await screen.findByText('managed page /abonelikler/1/dosyalar')).toBeTruthy()
  })
})
