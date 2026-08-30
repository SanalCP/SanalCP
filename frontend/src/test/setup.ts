import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

afterEach(cleanup)

const { translate } = vi.hoisted(() => ({ translate: (key: string, vars?: Record<string, unknown>) => vars?.count === undefined ? key : `${key}:${vars.count}` }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: translate,
    i18n: { language: 'tr', changeLanguage: vi.fn() },
  }),
}))
