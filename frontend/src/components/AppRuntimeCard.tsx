import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

type Runtime = 'node' | 'python'
type AppConfig = { runtime: Runtime; interpreter: string; entrypoint: string; health_path: string; port: number; enabled: boolean }
type Interpreter = { runtime: Runtime; path: string; version: string }
type Status = { configured: boolean; config: AppConfig; active: boolean; healthy: boolean; proxy_port: number; active_release: string; node_available: boolean; python_available: boolean; node_version: string; python_version: string; interpreters: Interpreter[] }

export default function AppRuntimeCard({ domainId, proxyPort }: { domainId: string; proxyPort: number }) {
  const { t } = useTranslation('AppRuntimeCard')
  const [status, setStatus] = useState<Status | null>(null)
  const [runtime, setRuntime] = useState<Runtime>('node')
  const [entrypoint, setEntrypoint] = useState('public_html/server.js')
  const [interpreter, setInterpreter] = useState('')
  const [healthPath, setHealthPath] = useState('/')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [logs, setLogs] = useState<string | null>(null)
  const [installOutput, setInstallOutput] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const { data } = await api.get<Status>(`/domains/${domainId}/app-runtime`)
      setStatus(data)
      if (data.configured) {
        setRuntime(data.config.runtime)
        setEntrypoint(data.config.entrypoint)
        setInterpreter(data.config.interpreter || '')
        setHealthPath(data.config.health_path || '/')
      }
    } catch (e) { setError(apiHata(e)) }
  }, [domainId])
  useEffect(() => { void load() }, [load])

  async function save() {
    setBusy(true); setError(''); setMessage('')
    try {
      await api.put(`/domains/${domainId}/app-runtime`, { runtime, interpreter, entrypoint: entrypoint.trim(), health_path: healthPath.trim(), port: proxyPort, enabled: status?.configured ? status.config.enabled : false })
      setMessage(t('saved'))
      await load()
    } catch (e) { setError(apiHata(e)) }
    finally { setBusy(false) }
  }
  async function action(name: 'start' | 'stop' | 'restart') {
    setBusy(true); setError(''); setMessage('')
    try {
      await api.post(`/domains/${domainId}/app-runtime/${name}`)
      setMessage(t('done'))
      await load()
    } catch (e) { setError(apiHata(e)) }
    finally { setBusy(false) }
  }
  async function getLogs() {
    try {
      const { data } = await api.get<{ logs: string }>(`/domains/${domainId}/app-runtime/logs`)
      setLogs(data.logs)
    } catch (e) { setError(apiHata(e)) }
  }
  async function installDependencies() {
    setBusy(true); setError(''); setMessage(''); setInstallOutput(null)
    try {
      const { data } = await api.post<{ output: string }>(`/domains/${domainId}/app-runtime/dependencies`, {}, { timeout: 330000 })
      setInstallOutput(data.output)
      setMessage(t('dependencies_installed'))
    } catch (e) { setError(apiHata(e)) }
    finally { setBusy(false) }
  }

  return <section className="mb-6 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
    <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('title')}</h3>
    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 mb-4">{t('description')}</p>
    {status && <p className="text-xs mb-3 text-slate-600 dark:text-slate-300">
      {t('status')}: <strong>{status.configured ? (status.active ? (status.healthy ? t('running') : t('unhealthy')) : t('stopped')) : t('unconfigured')}</strong>
      {' · '}{t('available')}: Node.js {status.node_version || '—'}, Python {status.python_version || '—'}
    </p>}
    {status?.active_release && <p className="text-xs mb-3 text-slate-600 dark:text-slate-300 break-all">{t('active_release')}: <code>{status.active_release}</code></p>}
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <label className="text-xs text-slate-500 dark:text-slate-400">{t('runtime')}
        <select value={runtime} onChange={e => { const value = e.target.value as Runtime; setRuntime(value); setInterpreter(''); setEntrypoint(value === 'node' ? 'public_html/server.js' : 'public_html/app.py') }}
          className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm bg-white dark:bg-slate-900">
          <option value="node">Node.js</option><option value="python">Python</option>
        </select>
      </label>
      <label className="text-xs text-slate-500 dark:text-slate-400">{t('interpreter')}
        <select value={interpreter} onChange={e => setInterpreter(e.target.value)} className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm bg-white dark:bg-slate-900">
          <option value="">{t('default_interpreter')}</option>
          {status?.interpreters?.filter(item => item.runtime === runtime).map(item => <option key={item.path} value={item.path}>{item.version || item.path} ({item.path})</option>)}
        </select>
      </label>
      <label className="text-xs text-slate-500 dark:text-slate-400">{t('entrypoint')}
        <input value={entrypoint} onChange={e => setEntrypoint(e.target.value)} placeholder={runtime === 'node' ? 'public_html/server.js' : 'public_html/app.py'}
          className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm bg-white dark:bg-slate-900" />
      </label>
      <label className="text-xs text-slate-500 dark:text-slate-400">{t('health_path')}
        <input value={healthPath} onChange={e => setHealthPath(e.target.value)} placeholder="/health"
          className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm bg-white dark:bg-slate-900" />
      </label>
    </div>
    <p className="text-xs text-slate-500 dark:text-slate-400 mt-3">{t('port', { port: proxyPort })}</p>
    <div className="flex flex-wrap gap-2 mt-4">
      <button disabled={busy || !status || (runtime === 'node' ? !status.node_available : !status.python_available)} onClick={save}
        className="px-3 py-2 rounded bg-amber-600 text-white text-sm disabled:opacity-50">{t('save')}</button>
      {status?.configured && <>
        <button disabled={busy} onClick={() => action('start')} className="px-3 py-2 rounded border text-sm disabled:opacity-50">{t('start')}</button>
        <button disabled={busy} onClick={() => action('stop')} className="px-3 py-2 rounded border text-sm disabled:opacity-50">{t('stop')}</button>
        <button disabled={busy} onClick={() => action('restart')} className="px-3 py-2 rounded border text-sm disabled:opacity-50">{t('restart')}</button>
        <button disabled={busy} onClick={getLogs} className="px-3 py-2 rounded border text-sm disabled:opacity-50">{t('logs')}</button>
        <button disabled={busy || !!status.active_release} onClick={installDependencies} className="px-3 py-2 rounded border text-sm disabled:opacity-50">{t('install_dependencies')}</button>
      </>}
    </div>
    {error && <p role="alert" className="text-sm text-red-600 mt-3">{error}</p>}
    {status?.active_release && <p className="text-xs text-slate-500 dark:text-slate-400 mt-3">{t('release_dependencies')}</p>}
    {message && <p role="status" className="text-sm text-emerald-700 mt-3">{message}</p>}
    {logs !== null && <pre className="mt-3 p-3 max-h-80 overflow-auto rounded bg-slate-950 text-slate-200 text-xs whitespace-pre-wrap">{logs || t('empty_logs')}</pre>}
    {installOutput !== null && <pre className="mt-3 p-3 max-h-80 overflow-auto rounded bg-slate-950 text-slate-200 text-xs whitespace-pre-wrap">{installOutput}</pre>}
  </section>
}
