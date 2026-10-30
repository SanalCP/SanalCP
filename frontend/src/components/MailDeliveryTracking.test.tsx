import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MailDeliveryTracking from './MailDeliveryTracking'
import type { Delivery, DeliveryDetail, DeliveryList } from '@/lib/mailDelivery'
import { useAuth } from '@/store/auth'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({
  api: { get },
  apiHata: (error: { message?: string }, fallback: string) => error.message || fallback,
}))

type ListResponse = { data: DeliveryList }
type DetailResponse = { data: DeliveryDetail }

const collector = { state: 'ok' as const, warning: '' as const, last_collected_at: '2026-10-29T10:00:00Z' }
function delivery(id = 1, recipient = 'first@example.test'): Delivery {
  return {
    id, queue_id: `Q${id}`, message_id: `<message-${id}@example.test>`, sender: 'sender@example.test', recipient,
    original_recipient: '', status: 'deferred', first_seen_at: '2026-10-29T09:00:00Z',
    last_seen_at: '2026-10-29T10:00:00Z', relay: 'mx.example.test', dsn: '4.2.0', response: 'try later',
  }
}
function list(items = [delivery()], total = items.length): ListResponse {
  return { data: { items, total, page: 1, page_size: 25, collector } }
}
function detail(item = delivery(), response = 'recipient response'): DetailResponse {
  return { data: {
    delivery: { ...item, response }, collector,
    events: [{ id: 1, at: item.first_seen_at, status: 'accepted', relay: '', dsn: '', response: '' },
      { id: 2, at: item.last_seen_at, status: 'deferred', relay: item.relay, dsn: item.dsn, response, delay_seconds: 5 }],
  } }
}
function pending<T>() {
  // The frontend targets ES2022, which does not provide Promise.withResolvers.
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
function openDetails(recipient: string) {
  fireEvent.click(within(screen.getByText(recipient).closest('tr')!).getByRole('button'))
}

beforeEach(() => {
  get.mockReset()
  useAuth.setState({ kullanici: { id: 1, adi: 'customer', rol: 'user' } })
})

describe('delivery tracking scope and request transitions', () => {
  it('drops filters, records and an in-flight recipient detail immediately when switching domains', async () => {
    const oldDetail = pending<DetailResponse>()
    const newList = pending<ListResponse>()
    get.mockResolvedValueOnce(list())
    const view = render(<MailDeliveryTracking domainID="1" />)
    await screen.findByText('first@example.test')
    get.mockResolvedValueOnce(list())
    fireEvent.change(screen.getByLabelText('sender'), { target: { value: 'sender' } })
    await screen.findByText('first@example.test')
    get.mockReturnValueOnce(oldDetail.promise)
    openDetails('first@example.test')
    get.mockReturnValueOnce(newList.promise)
    view.rerender(<MailDeliveryTracking domainID="2" />)
    expect(screen.queryByText('first@example.test')).toBeNull()
    expect((screen.getByLabelText('sender') as HTMLInputElement).value).toBe('')
    expect(screen.queryByRole('region', { name: 'detail_title' })).toBeNull()
    await act(async () => oldDetail.resolve(detail(delivery(), 'private previous domain response')))
    expect(screen.queryByText('private previous domain response')).toBeNull()
    await act(async () => newList.resolve(list([delivery(2, 'second@other.test')])))
    expect(await screen.findByText('second@other.test')).toBeTruthy()
    expect(screen.queryByText('first@example.test')).toBeNull()
  })

  it('ignores a late list result after filters change', async () => {
    const oldList = pending<ListResponse>()
    const filteredList = pending<ListResponse>()
    get.mockReturnValueOnce(oldList.promise).mockReturnValueOnce(filteredList.promise)
    render(<MailDeliveryTracking domainID="1" />)
    fireEvent.change(screen.getByLabelText('recipient'), { target: { value: 'second' } })
    await act(async () => filteredList.resolve(list([delivery(2, 'second@example.test')])))
    await screen.findByText('second@example.test')
    await act(async () => oldList.resolve(list()))
    expect(screen.queryByText('first@example.test')).toBeNull()
    expect(screen.getByText('second@example.test')).toBeTruthy()
  })

  it('does not expose a late response from a previously selected recipient', async () => {
    const first = delivery()
    const second = delivery(2, 'second@example.test')
    const firstDetail = pending<DetailResponse>()
    get.mockResolvedValueOnce(list([first, second])).mockReturnValueOnce(firstDetail.promise).mockResolvedValueOnce(detail(second, 'second recipient result'))
    render(<MailDeliveryTracking domainID="1" />)
    await screen.findByText('first@example.test')
    openDetails(first.recipient)
    openDetails(second.recipient)
    await screen.findAllByText('second recipient result')
    await act(async () => firstDetail.resolve(detail(first, 'private first recipient result')))
    expect(screen.queryByText('private first recipient result')).toBeNull()
    expect(screen.getAllByText('second recipient result')).toHaveLength(2)
  })

  it('resets pagination and selection on filtering, and never leaves the old page visible while loading', async () => {
    const secondPage = pending<ListResponse>()
    get.mockResolvedValueOnce(list([delivery()], 26)).mockReturnValueOnce(secondPage.promise)
    render(<MailDeliveryTracking domainID="1" />)
    await screen.findByText('first@example.test')
    fireEvent.click(screen.getByRole('button', { name: 'next' }))
    expect(screen.queryByText('first@example.test')).toBeNull()
    expect(screen.getByRole('status').textContent).toBe('loading')
    await act(async () => secondPage.resolve(list([delivery(2, 'last@example.test')], 26)))
    await screen.findByText('last@example.test')
    get.mockResolvedValueOnce(detail(delivery(2, 'last@example.test'), 'old page detail'))
    openDetails('last@example.test')
    await screen.findAllByText('old page detail')
    get.mockResolvedValueOnce(list([delivery(3, 'match@example.test')]))
    fireEvent.change(screen.getByLabelText('recipient'), { target: { value: 'match' } })
    expect(screen.queryByText('old page detail')).toBeNull()
    expect(screen.queryByText('last@example.test')).toBeNull()
    await screen.findByText('match@example.test')
    expect((screen.getByRole('button', { name: 'previous' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: 'next' }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('returns to a valid page when a refreshed result set shrinks', async () => {
    get.mockResolvedValueOnce(list([delivery()], 26)).mockResolvedValueOnce(list([delivery(2, 'last@example.test')], 26))
    render(<MailDeliveryTracking domainID="1" />)
    await screen.findByText('first@example.test')
    fireEvent.click(screen.getByRole('button', { name: 'next' }))
    await screen.findByText('last@example.test')
    get.mockResolvedValueOnce(list([], 1)).mockResolvedValueOnce(list([delivery(3, 'remaining@example.test')]))
    fireEvent.click(screen.getByRole('button', { name: 'refresh' }))
    await screen.findByText('remaining@example.test')
    expect((screen.getByRole('button', { name: 'previous' }) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.queryByText('last@example.test')).toBeNull()
  })

  it('shows a failed refresh as an error, not empty success, and can recover', async () => {
    get.mockResolvedValueOnce(list()).mockRejectedValueOnce(new Error('<img src=x onerror=alert(1)>'))
    const view = render(<MailDeliveryTracking domainID="1" />)
    await screen.findByText('first@example.test')
    fireEvent.click(screen.getByRole('button', { name: 'refresh' }))
    expect((await screen.findByRole('alert')).textContent).toBe('<img src=x onerror=alert(1)>')
    expect(screen.queryByText('first@example.test')).toBeNull()
    expect(screen.queryByText('empty')).toBeNull()
    expect(view.container.querySelector('img')).toBeNull()
    get.mockResolvedValueOnce(list([]))
    fireEvent.click(screen.getByRole('button', { name: 'refresh' }))
    expect(await screen.findByText('empty')).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('renders remote responses as text and clears a failed detail before retrying it', async () => {
    const unsafe = '<img src=x onerror=alert(1)>'
    const retry = pending<DetailResponse>()
    get.mockResolvedValueOnce(list()).mockRejectedValueOnce(new Error('detail unavailable'))
    const view = render(<MailDeliveryTracking domainID="1" />)
    await screen.findByText('first@example.test')
    openDetails('first@example.test')
    await screen.findByText('detail unavailable')
    get.mockReturnValueOnce(retry.promise)
    fireEvent.click(screen.getByRole('button', { name: 'refresh_detail' }))
    expect(screen.queryByText('detail unavailable')).toBeNull()
    await act(async () => retry.resolve(detail(delivery(), unsafe)))
    expect(await screen.findAllByText(unsafe)).toHaveLength(2)
    expect(view.container.querySelector('img')).toBeNull()
    expect(view.container.querySelector('[onerror]')).toBeNull()
  })
})
