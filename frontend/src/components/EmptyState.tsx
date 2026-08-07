// sanal-dark-swept
// sanal-dark-swept-v2
export default function EmptyState({
  baslik, aciklama, buton,
}: {
  baslik: string
  aciklama?: string
  buton?: { etiket: string; onClick?: () => void; disabled?: boolean; ipucu?: string }
}) {
  return (
    <div className="ta-card py-16 flex flex-col items-center text-center px-4">
      <svg viewBox="0 0 240 160" className="w-48 h-32 mb-5">
        <defs>
          <linearGradient id="ebg" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#7592ff" />
            <stop offset="100%" stopColor="#3641f5" />
          </linearGradient>
        </defs>
        <ellipse cx="120" cy="135" rx="100" ry="14" fill="#0f172a" opacity="0.08" />
        <path d="M40 80 Q40 50 70 50 Q70 30 100 30 Q120 20 140 30 Q170 30 170 50 Q200 50 200 80 Q200 110 170 110 L70 110 Q40 110 40 80 Z" fill="url(#ebg)" />
        <rect x="95" y="55" width="50" height="40" rx="3" fill="#ffffff" opacity="0.9" />
        <rect x="100" y="63" width="40" height="3" fill="#fff" opacity="0.5" />
        <rect x="100" y="69" width="32" height="2" fill="#fff" opacity="0.35" />
        <rect x="100" y="74" width="36" height="2" fill="#fff" opacity="0.35" />
        <circle cx="155" cy="65" r="6" fill="#fbbf24" />
        <path d="M68 95 L72 87 L76 95 Z" fill="#465fff" />
      </svg>
      <h3 className="text-base font-semibold text-slate-700 dark:text-slate-300 mb-1">{baslik}</h3>
      {aciklama && <p className="text-sm text-slate-500 dark:text-slate-500 max-w-md mb-5">{aciklama}</p>}
      {buton && (
        <button
          onClick={buton.onClick}
          disabled={buton.disabled}
          title={buton.ipucu}
          className="ta-primary-button"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          {buton.etiket}
        </button>
      )}
    </div>
  )
}
