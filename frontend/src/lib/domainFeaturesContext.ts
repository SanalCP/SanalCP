import { createContext, useContext } from 'react'
import type { PlanFeature } from './planFeatures'

type FeatureContext = {
  domainId: string
  ready: boolean
  failed: boolean
  allows: (feature: PlanFeature) => boolean
  allowsPath: (path: string) => boolean
  retry: () => void
}

export const DomainFeaturesContext = createContext<FeatureContext | null>(null)

export function useDomainFeatures() {
  const value = useContext(DomainFeaturesContext)
  if (!value) throw new Error('DomainFeaturesProvider is required')
  return value
}
