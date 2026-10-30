export const planFeatureKeys = ['dns', 'mail', 'databases', 'files', 'ftp', 'cron', 'backups', 'git'] as const
export type PlanFeature = typeof planFeatureKeys[number]
export type PlanFeatures = Record<PlanFeature, boolean>

export function allPlanFeatures(): PlanFeatures {
  return { dns: true, mail: true, databases: true, files: true, ftp: true, cron: true, backups: true, git: true }
}

export function isPlanFeatures(value: unknown): value is PlanFeatures {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const flags = value as Record<string, unknown>
  return Object.keys(flags).length === planFeatureKeys.length && planFeatureKeys.every(key => typeof flags[key] === 'boolean')
}

// Only these management pages are plan-scoped; cross-module tools stay independent.
export function domainPathFeature(path: string): PlanFeature | undefined {
  const slug = path.match(/^\/abonelikler\/\d+\/([^/?#]+)/)?.[1]
  switch (slug) {
    case 'dns': case 'cloudflare': return 'dns'
    case 'mail': return 'mail'
    case 'veritabanlari': return 'databases'
    case 'dosyalar': return 'files'
    case 'ftp': return 'ftp'
    case 'cron': return 'cron'
    case 'yedekler': return 'backups'
    case 'git': return 'git'
    default: return undefined
  }
}
