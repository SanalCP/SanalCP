import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

afterEach(cleanup)

// jsdom, <dialog> üst katman API'sini uygulamıyor. Gerçek odak/arka plan
// davranışı tarayıcıda; bileşen testlerinde yalnız açılma/kapanmayı taklit et.
HTMLDialogElement.prototype.showModal = function () { this.open = true }
HTMLDialogElement.prototype.close = function () { this.open = false }

const { translate } = vi.hoisted(() => ({ translate: (key: string, vars?: Record<string, unknown>) => vars?.count === undefined ? key : `${key}:${vars.count}` }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: translate,
    i18n: { language: 'tr', changeLanguage: vi.fn() },
  }),
}))
