export default function AyarKartDurumu({ yukleniyor, hata, tekrar, yukleniyorMetni = 'Yükleniyor…', tekrarMetni = 'Tekrar dene' }: { yukleniyor: boolean; hata: string; tekrar: () => void; yukleniyorMetni?: string; tekrarMetni?: string }) {
  if (yukleniyor) return <div role="status" className="mt-3 animate-pulse rounded-xl bg-slate-100 px-3 py-6 text-center text-xs text-slate-500 dark:bg-slate-800">{yukleniyorMetni}</div>
  if (hata) return <div role="alert" className="mt-3 rounded-xl border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-900 dark:bg-red-900/20 dark:text-red-300"><p>{hata}</p><button type="button" onClick={tekrar} className="mt-2 font-medium underline underline-offset-2">{tekrarMetni}</button></div>
  return null
}
