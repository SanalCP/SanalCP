import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DomainHealthCard from './DomainHealthCard'

const { get, features } = vi.hoisted(() => ({
  get: vi.fn(),
  features: { ready: true, failed: false, dns: true, mail: true, backups: true },
}))
vi.mock('@/lib/api', () => ({ api: { get } }))
vi.mock('@/lib/domainFeaturesContext', () => ({
  useDomainFeatures: () => ({ ...features, allows: (key: 'dns' | 'mail' | 'backups') => features[key] }),
}))

const dnsChecks = [
  { anahtar: 'ns', durum: 'ok' }, { anahtar: 'a', durum: 'ok' },
  { anahtar: 'mx', durum: 'hata' }, { anahtar: 'spf', durum: 'hata' },
  { anahtar: 'dkim', durum: 'hata' },
]
let responses: Record<string, unknown>
function show(domainId = 1, suspended = false) {
  return render(<MemoryRouter><DomainHealthCard domainId={domainId} suspended={suspended} /></MemoryRouter>)
}
function row(id: string) { return screen.getByText(`labels.${id}`).closest('li')! }
async function finished() { await screen.findByText('checked_at') }

beforeEach(() => {
  Object.assign(features, { ready: true, failed: false, dns: true, mail: true, backups: true })
  responses = {
    health: { erisilebilir: true, durum_kodu: 200, yanit_suresi_ms: 42 },
    ssl: { aktif: true, kaynak: 'letsencrypt', bitis_iso: new Date(Date.now() + 30 * 86_400_000).toISOString() },
    'dns/dogrula': { kontroller: dnsChecks },
    'mail/durum': { etkin: false },
    mail: [],
    backups: [{ olusturma: new Date().toISOString(), boyut_b: 1024, dogrulama_durum: 'dogrulandi' }],
    'backup-schedule': { freq: 'daily' },
    kaynak: { disk_mb: { kullanim: 50, limit: 100 } },
  }
  get.mockReset().mockImplementation((url: string) => {
    const path = url.replace(/^\/domains\/\d+\//, '')
    if (!(path in responses)) return Promise.reject(new Error(`Unexpected URL ${url}`))
    return Promise.resolve({ data: responses[path] })
  })
})

describe('domain health checks', () => {
  it('hides inactive email, skips mailbox queries and excludes mail DNS problems from website DNS', async () => {
    show()
    await finished()
    expect(screen.queryByText('labels.mail')).toBeNull()
    expect(get.mock.calls.some(([url]) => url === '/domains/1/mail')).toBe(false)
    expect(within(row('dns')).getByText('statuses.ok')).toBeTruthy()
    expect(screen.getByText('all_ok')).toBeTruthy()
  })

  it('does not request or show services outside the plan and waits for feature permissions', async () => {
    features.ready = false
    const view = show()
    expect(get).not.toHaveBeenCalled()
    Object.assign(features, { ready: true, dns: false, mail: false, backups: false })
    view.rerender(<MemoryRouter><DomainHealthCard domainId={1} /></MemoryRouter>)
    await finished()
    expect(get.mock.calls.map(([url]) => url).sort()).toEqual(['/domains/1/health', '/domains/1/kaynak', '/domains/1/ssl'])
    for (const id of ['dns', 'mail', 'backups']) expect(screen.queryByText(`labels.${id}`)).toBeNull()
  })

  it('shares one DNS check and reports active email DNS errors with a link to email settings', async () => {
    responses['mail/durum'] = { etkin: true }
    show()
    await finished()
    expect(within(row('mail')).getByText('statuses.hata')).toBeTruthy()
    expect(within(row('mail')).getByRole('link').getAttribute('href')).toBe('/abonelikler/1/mail')
    expect(get.mock.calls.filter(([url]) => url.endsWith('/dns/dogrula'))).toHaveLength(1)
  })

  it('flags full mailboxes even if email DNS is healthy', async () => {
    responses['mail/durum'] = { etkin: true }
    responses['dns/dogrula'] = { kontroller: dnsChecks.map(c => ({ ...c, durum: 'ok' })) }
    responses.mail = [{ used_bytes: 100, quota_bytes: 100 }, { used_bytes: null, quota_bytes: 100 }]
    show()
    await finished()
    expect(within(row('mail')).getByText('mail_quota')).toBeTruthy()
    expect(within(row('mail')).getByText('statuses.hata')).toBeTruthy()
  })

  it('preserves other results when an API fails and retries on refresh', async () => {
    get.mockImplementation((url: string) => url.endsWith('/health')
      ? Promise.reject(new Error('timeout'))
      : Promise.resolve({ data: responses[url.replace(/^\/domains\/\d+\//, '')] }))
    show()
    await finished()
    expect(within(row('web')).getByText('statuses.unknown')).toBeTruthy()
    expect(within(row('ssl')).getByText('statuses.ok')).toBeTruthy()
    expect(screen.getByText('incomplete')).toBeTruthy()
    get.mockImplementation((url: string) => Promise.resolve({ data: responses[url.replace(/^\/domains\/\d+\//, '')] }))
    fireEvent.click(screen.getByRole('button', { name: 'refresh' }))
    await finished()
    expect(within(row('web')).getByText('statuses.ok')).toBeTruthy()
  })

  it('flags server errors, expired certificates and exhausted disk quotas', async () => {
    responses.health = { erisilebilir: true, durum_kodu: 503, yanit_suresi_ms: 20 }
    responses.ssl = { aktif: true, kaynak: 'letsencrypt', bitis_iso: '2020-01-01T00:00:00Z' }
    responses.kaynak = { disk_mb: { kullanim: 110, limit: 100 } }
    show()
    await finished()
    for (const id of ['web', 'ssl', 'disk']) expect(within(row(id)).getByText('statuses.hata')).toBeTruthy()
  })

  it('does not treat a self-signed certificate or an unverified backup as healthy', async () => {
    responses.ssl = { ...(responses.ssl as object), kaynak: 'self-signed' }
    responses.backups = [{ olusturma: new Date().toISOString(), boyut_b: 1024, dogrulama_durum: 'bekliyor' }]
    show()
    await finished()
    expect(within(row('ssl')).getByText('ssl_self_signed')).toBeTruthy()
    expect(within(row('backups')).getByText('backup_missing')).toBeTruthy()
  })

  it('uses the configured backup interval to flag old verified backups', async () => {
    responses.backups = [{ olusturma: new Date(Date.now() - 4 * 86_400_000).toISOString(), boyut_b: 1024, dogrulama_durum: 'dogrulandi' }]
    show()
    await finished()
    expect(within(row('backups')).getByText('backup_stale')).toBeTruthy()
  })

  it.each(['weekly', 'monthly', 'none'])('accepts a four-day-old verified backup for a %s schedule', async freq => {
    responses['backup-schedule'] = { freq }
    responses.backups = [{ olusturma: new Date(Date.now() - 4 * 86_400_000).toISOString(), boyut_b: 1024, dogrulama_durum: 'dogrulandi' }]
    show()
    await finished()
    expect(within(row('backups')).getByText('statuses.ok')).toBeTruthy()
  })

  it('hides unused backups when automatic backups are disabled', async () => {
    responses['backup-schedule'] = { freq: 'none' }
    responses.backups = []
    show()
    await finished()
    expect(screen.queryByText('labels.backups')).toBeNull()
  })

  it('reports a failed latest backup even when an older verified backup exists', async () => {
    responses.backups = [
      { olusturma: new Date().toISOString(), boyut_b: 1024, dogrulama_durum: 'basarisiz' },
      { olusturma: new Date(Date.now() - 86_400_000).toISOString(), boyut_b: 1024, dogrulama_durum: 'dogrulandi' },
    ]
    show()
    await finished()
    expect(within(row('backups')).getByText('statuses.hata')).toBeTruthy()
    expect(within(row('backups')).getByText('backup_failed')).toBeTruthy()
  })

  it('skips the website probe for suspended domains', async () => {
    show(1, true)
    await finished()
    expect(screen.queryByText('labels.web')).toBeNull()
    expect(get.mock.calls.some(([url]) => url.endsWith('/health'))).toBe(false)
  })

  it('aborts previous requests and ignores late results after switching domains', async () => {
    let resolveOld!: (value: unknown) => void
    get.mockImplementation((url: string) => url === '/domains/1/health'
      ? new Promise(resolve => { resolveOld = resolve })
      : Promise.resolve({ data: responses[url.replace(/^\/domains\/\d+\//, '')] }))
    const view = show()
    const oldSignal = get.mock.calls.find(([url]) => url === '/domains/1/health')![1].signal as AbortSignal
    view.rerender(<MemoryRouter><DomainHealthCard domainId={2} /></MemoryRouter>)
    await finished()
    expect(oldSignal.aborted).toBe(true)
    await act(async () => resolveOld({ data: { erisilebilir: false, durum_kodu: 0, yanit_suresi_ms: 0 } }))
    expect(within(row('web')).getByText('statuses.ok')).toBeTruthy()
    expect(within(row('web')).getByRole('link').getAttribute('href')).toBe('/abonelikler/2/gunlukler')
  })
})
