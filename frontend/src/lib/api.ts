import axios, { AxiosError } from 'axios'
import { useAuth } from '@/store/auth'
import i18n from '@/i18n'

const baseURL = (import.meta.env.VITE_API_BASE as string) || '/api/v1'

export const api = axios.create({
  baseURL,
  timeout: 30_000,
})

api.interceptors.request.use((cfg) => {
  const tok = useAuth.getState().token
  if (tok) {
    cfg.headers = cfg.headers || {}
    cfg.headers.Authorization = `Bearer ${tok}`
  }
  return cfg
})

api.interceptors.response.use(
  (r) => r,
  (err: AxiosError<{ hata?: string }>) => {
    if (err.response?.status === 401) {
      const s = useAuth.getState()
      if (s.token) s.cikis()
    }
    return Promise.reject(err)
  },
)

export function apiHata(err: unknown, varsayilan = i18n.t('common:error_generic')): string {
  const e = err as AxiosError<{ hata?: string }>
  if (e?.response?.data?.hata) return e.response.data.hata
  if (e?.message) return e.message
  return varsayilan
}
