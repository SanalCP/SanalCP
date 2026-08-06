// sanal-dark-swept
// sanal-dark-swept-v2
// sp-mobil-v1
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import { useAuth } from '@/store/auth'
import Modal from '@/components/Modal'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'
import { T } from '@/lib/tablo'

type Domain = {
  id: number; alan_adi: string; sistem_kullanici: string
  boyut_kb: number; trafik_kb: number; durum: string
  php_surum?: string; is_demo?: boolean
  olusturulma?: string; plan_id?: number; plan_ad?: string
  ssl?: boolean; ssl_bitis?: string; ssl_kaynak?: string
  bayi_adi?: string; bayi_paket_adi?: string
  ipv4?: string
}
type Plan = { id: number; ad: string; disk_kota_mb?: number }
type PHPVer = { surum: string; aciklama?: string }
type OlusturmaSonuc = {
  id: number
  alan_adi: string; sistem_kullanici: string; ftp_user: string; ftp_host: string
  db_host: string; db_user: string; db_adi: string
  olusturulan_parolalar: { ftp: string; db: string }
  // Sunucuda ortak nameserver tanımlı değilse backend bu alanı hiç göndermez
  // (vanity değerler müşteriye verilemez) → bölüm gösterilmez.
  nameserver?: { ns1: string; ns2: string }
}

function fmtKB(kb: number) {
  if (kb < 1024) return kb + ' KB'
  if (kb < 1024 * 1024) return (kb / 1024).toFixed(1) + ' MB'
  return (kb / 1024 / 1024).toFixed(2) + ' GB'
}

export default function DomainsPage() {
  const { t } = useTranslation(['DomainsPage', 'common'])
  const [items, setItems] = useState<Domain[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  // uyari: işlem teknik olarak tamamlandı ama SONUÇ istenen değil (ör. Let's
  // Encrypt alınamadı, self-signed'a düşüldü). Bunu yeşil "başarılı" kutusunda
  // göstermek kullanıcıyı yanıltıyordu — tarayıcı sertifika uyarısı verirken
  // panel başarı diyordu. DomainSSLPage ile aynı desen.
  const [uyari, setUyari] = useState<string | null>(null)
  const [q, setQ] = useState('')
  const [secili, setSecili] = useState<Set<number>>(new Set())
  const [isleniyor, setIsleniyor] = useState(false)
  const [yenileniyor, setYenileniyor] = useState(false)
  // Sahip değiştirme (domain transferi). Uç AdminOnly olduğu için buton yalnız
  // admin'e gösterilir — bayiye göstermek tıklayınca 403 almasına yol açardı.
  const benimRolum = useAuth((s) => s.kullanici?.rol)
  const adminMiyim = benimRolum === 'admin'
  const [sahipAcik, setSahipAcik] = useState(false)
  const [musteriler, setMusteriler] = useState<{ id: number; ad: string; eposta: string }[]>([])
  const [sahipHedef, setSahipHedef] = useState<string>('')
  const [silOnay, setSilOnay] = useState(false)
  const [silOnayMetin, setSilOnayMetin] = useState('')

  const [planlar, setPlanlar] = useState<Plan[]>([])
  const [phpSurumler, setPhpSurumler] = useState<PHPVer[]>([])
  const [modalVeriYuk, setModalVeriYuk] = useState(false) // plan+php sürüm yüklemesi (listeyi BLOKLAMAZ)
  const [modalVeriGeldi, setModalVeriGeldi] = useState(false)
  const [olusturAcik, setOlusturAcik] = useState(false)
  const [olusturuluyor, setOlusturuluyor] = useState(false)
  const [olusturmaSonuc, setOlusturmaSonuc] = useState<OlusturmaSonuc | null>(null)
  const [sonucKopyalandi, setSonucKopyalandi] = useState(false)
  const [wpYonlendir, setWpYonlendir] = useState<{ id: number; alanAdi: string } | null>(null)
  const [fAlanAdi, setFAlanAdi] = useState('')
  const [fPHPSurum, setFPHPSurum] = useState('8.3')
  const [fSiteTipi, setFSiteTipi] = useState<'php'|'wordpress'|'statik'>('php')
  const [fPlanID, setFPlanID] = useState<number | ''>('')
  const [fSSL, setFSSL] = useState(false)
  const [fWWW, setFWWW] = useState(false)

  // Liste yalnızca /domains'e bağlıdır. /plans + /php/versions (yavaş olabilen dnf keşfi)
  // listeyi BLOKLAMAZ — modal açılınca lazy çekilir. Böylece dnf yavaş/kilitliyken bile
  // "Domainler" gelir gelmez render olur.
  // sessiz=true: tabloyu "Yükleniyor" ile DEĞİŞTİRMEDEN tazeler. Arka plan
  // yenilemeleri (ör. SSL kurulduktan sonra rozeti güncellemek) için — aksi
  // hâlde dolu bir liste bir anlığına boşalıp geri gelir, göz tırmalar.
  function yukle(sessiz = false) {
    if (!sessiz) setYuk(true)
    return api.get<Domain[]>('/domains')
      .then(r => setItems(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => { if (!sessiz) setYuk(false) })
  }
  useEffect(() => { yukle() }, [])

  // Mobil alt gezinme çubuğundaki "Yeni" eylemi buraya ?yeni=1 ile gelir.
  // Kipi açıp parametreyi TEMİZLİYORUZ: aksi halde geri/yenilemede kip
  // tekrar tekrar açılır ve kullanıcı sıkışır.
  const [aramaParam, setAramaParam] = useSearchParams()
  useEffect(() => {
    if (aramaParam.get('yeni') !== '1') return
    olusturAc()
    const kalan = new URLSearchParams(aramaParam)
    kalan.delete('yeni')
    setAramaParam(kalan, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [aramaParam])

  // Modal için gereken plan + php sürümleri — listeyi bloklamayan ayrı yükleme.
  // Modal ilk açılışında lazy çekilir; bir kez geldiyse tekrar çekmez.
  function modalVeriYukle() {
    if (modalVeriGeldi || modalVeriYuk) return
    setModalVeriYuk(true)
    Promise.all([
      api.get<Plan[]>('/plans').catch(() => ({ data: [] })),
      api.get<PHPVer[]>('/php/versions').catch(() => ({ data: [] })),
    ]).then(([pr, phpr]) => {
      const pl = pr.data as Plan[]
      setPlanlar(pl)
      setPhpSurumler(phpr.data as PHPVer[])
      setModalVeriGeldi(true)
      // Plan henüz seçilmediyse (modal veri gelmeden açıldıysa) varsayılanı ata.
      setFPlanID(prev => {
        if (prev !== '') return prev
        const v = pl.find(p => p.ad === 'Başlangıç') || pl[0]
        return v ? v.id : ''
      })
    }).finally(() => setModalVeriYuk(false))
  }

  function olusturAc() {
    setHata(null); setBasari(null); setUyari(null); setOlusturmaSonuc(null)
    // varsayılan plan = "Başlangıç" (yoksa ilk plan, o da yoksa boş) — veri geldiyse hemen ata,
    // gelmediyse modalVeriYukle tamamlanınca atanır.
    const varsayilan = planlar.find(p => p.ad === 'Başlangıç') || planlar[0]
    setFAlanAdi(''); setFPHPSurum('8.3'); setFSiteTipi('php'); setFPlanID(varsayilan ? varsayilan.id : ''); setFSSL(false); setFWWW(false)
    setOlusturAcik(true)
    modalVeriYukle() // lazy: plan/php sürümleri henüz gelmediyse şimdi çek (listeyi bloklamaz)
  }

  async function olusturGonder(e: React.FormEvent) {
    e.preventDefault()
    setHata(null); setUyari(null)
    const alanAdi = fAlanAdi.trim().toLowerCase()
    if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/.test(alanAdi)) {
      setHata(t('DomainsPage:create_modal.validation_error'))
      return
    }
    setOlusturuluyor(true)
    try {
      const body: any = { alan_adi: alanAdi, php_surum: fPHPSurum, site_tipi: fSiteTipi }
      if (fPlanID !== '') body.plan_id = fPlanID
      const r = await api.post<OlusturmaSonuc>('/domains', body)
      setOlusturAcik(false)
      setOlusturmaSonuc(r.data)
      // 🔴 Listeyi HEMEN tazele. Bu çağrı eskiden SSL bloğunun ALTINDAydı:
      // "Let's Encrypt kur" işaretliyse domain çoktan oluşmuş olmasına rağmen
      // liste, sertifika alınana kadar (120sn timeout, tipik ~10sn) eski hâlde
      // kalıyordu — kullanıcı domaini "eklenmedi" sanıyordu.
      yukle()
      let mesaj = t('DomainsPage:create_modal.success', { name: alanAdi })
      if (fSSL) {
        try {
          const sslR = await api.post<{ tip: string; uyari?: string }>(`/domains/${r.data.id}/ssl/issue`, { tip: 'letsencrypt' }, { timeout: 120_000 })
          // Sunucu self-signed'a düştüyse bunu AYRI bir uyarı kutusunda göster;
          // başarı mesajına iliştirmek yeşil kutuda kaybolmasına yol açıyordu.
          if (sslR.data.uyari) setUyari(sslR.data.uyari)
          else mesaj += t('DomainsPage:create_modal.ssl_success')
        } catch (e) {
          setUyari(t('DomainsPage:create_modal.ssl_error_detail'))
        }
        if (fWWW) {
          try {
            await api.put(`/domains/${r.data.id}/www-yonlendir`, { aktif: true })
            mesaj += t('DomainsPage:create_modal.www_success')
          } catch (e) {
            mesaj += t('DomainsPage:create_modal.www_error')
          }
        }
      }
      setBasari(mesaj)
      setTimeout(() => setBasari(null), 8000)
      // SSL/www adımları listeyi değiştirmiş olabilir (SSL rozeti) — sessiz
      // tazele, tablo boşalmasın.
      if (fSSL) yukle(true)
      // WordPress tipinde kurulumu BURADA çalıştırmıyoruz: WP kurulumu site
      // başlığı + admin kullanıcı/e-posta ister ve admin parolası üretilip
      // kullanıcıya gösterilmek zorundadır. Bunları uydurmak yerine kullanıcıyı
      // kendi kurulum ekranına gönderiyoruz.
      if (fSiteTipi === 'wordpress') {
        setWpYonlendir({ id: r.data.id, alanAdi })
      }
    } catch (e: any) {
      setHata(apiHata(e, t('DomainsPage:create_modal.create_error')))
    } finally {
      setOlusturuluyor(false)
    }
  }

  async function panoYaz(metin: string) {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(metin); return true
      }
    } catch {}
    try {
      const ta = document.createElement('textarea')
      ta.value = metin; ta.style.position = 'fixed'; ta.style.opacity = '0'
      document.body.appendChild(ta); ta.select(); document.execCommand('copy')
      document.body.removeChild(ta); return true
    } catch {}
    try { window.prompt(t('DomainsPage:result_modal.clipboard_prompt'), metin); return true } catch {}
    return false
  }

  function sonucMetni(s: OlusturmaSonuc) {
    return [
      `SanalCP — ${s.alan_adi}`,
      '',
      t('DomainsPage:result_modal.ftp'),
      `  ${t('DomainsPage:result_modal.host')}: ${s.ftp_host || '—'}`,
      `  ${t('DomainsPage:result_modal.user')}: ${s.ftp_user}`,
      `  ${t('DomainsPage:result_modal.password')}: ${s.olusturulan_parolalar.ftp}`,
      '',
      t('DomainsPage:result_modal.mysql'),
      `  ${t('DomainsPage:result_modal.host')}: ${s.db_host || 'localhost'}`,
      `  ${t('DomainsPage:result_modal.database')}: ${s.db_adi}`,
      `  ${t('DomainsPage:result_modal.user')}: ${s.db_user}`,
      `  ${t('DomainsPage:result_modal.password')}: ${s.olusturulan_parolalar.db}`,
      '',
      ...(s.nameserver ? [
        t('DomainsPage:result_modal.nameservers'),
        `  NS1: ${s.nameserver.ns1}`,
        `  NS2: ${s.nameserver.ns2}`,
        `  ${t('DomainsPage:result_modal.ns_note')}`,
        '',
      ] : []),
      `${t('DomainsPage:result_modal.system_user')} ${s.sistem_kullanici}`,
    ].join('\n')
  }

  function sonucTxtIndir(s: OlusturmaSonuc) {
    const blob = new Blob([sonucMetni(s)], { type: 'text/plain;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${s.alan_adi}-erisim-bilgileri.txt`
    a.click()
    URL.revokeObjectURL(a.href)
  }

  const filtreli = useMemo(() => {
    const s = q.trim().toLowerCase()
    if (!s) return items
    return items.filter(d => d.alan_adi.toLowerCase().includes(s) || d.sistem_kullanici.toLowerCase().includes(s)
      || (d.bayi_adi || '').toLowerCase().includes(s))
  }, [items, q])

  function togga(id: number) {
    setSecili(prev => {
      const yeni = new Set(prev)
      if (yeni.has(id)) yeni.delete(id); else yeni.add(id)
      return yeni
    })
  }
  function tumunuSec(secVar: boolean) {
    if (secVar) setSecili(new Set(filtreli.map(d => d.id)))
    else setSecili(new Set())
  }

  async function topluSil() {
    setSilOnay(false); setSilOnayMetin(''); setIsleniyor(true); setHata(null)
    const ids = Array.from(secili); let basarili = 0
    for (const id of ids) {
      try { await api.delete(`/domains/${id}`); basarili++ } catch {}
    }
    setSecili(new Set()); setBasari(t('DomainsPage:delete_modal.success', { success: basarili, total: ids.length }))
    setTimeout(() => setBasari(null), 4000)
    setIsleniyor(false); yukle()
  }

  async function durumDegistir(yeniDurum: 'aktif' | 'pasif') {
    setIsleniyor(true); setHata(null)
    const ids = Array.from(secili)
    const statusLabel = yeniDurum === 'aktif' ? t('common:active') : t('common:inactive')
    try {
      await api.post('/domains/toplu/durum', { ids, durum: yeniDurum })
      setBasari(t('DomainsPage:bulk.status_changed', { count: ids.length, status: statusLabel }))
      setTimeout(() => setBasari(null), 4000)
      setSecili(new Set()); yukle()
    } catch (e) { setHata(apiHata(e, t('DomainsPage:bulk.status_error'))) }
    finally { setIsleniyor(false) }
  }

  async function sahipAc() {
    setSahipHedef(''); setSahipAcik(true)
    try {
      const { data } = await api.get<{ id: number; ad: string; eposta: string }[]>('/customers')
      setMusteriler(data || [])
    } catch (e) { setHata(apiHata(e, t('DomainsPage:owner_modal.load_error'))) }
  }

  async function sahipUygula() {
    const ids = Array.from(secili)
    // Boş seçim = sahipliği KALDIR (customer_id NULL). Backend bunu destekliyor;
    // domain sahipsiz kalır ve doğrudan admin'e döner.
    const customer_id = sahipHedef === '' ? null : Number(sahipHedef)
    setIsleniyor(true); setHata(null)
    try {
      await api.post('/domains/toplu/sahip', { ids, customer_id })
      setBasari(t('DomainsPage:owner_modal.success', { count: ids.length }))
      setTimeout(() => setBasari(null), 4000)
      setSahipAcik(false); setSecili(new Set()); yukle()
    } catch (e) { setHata(apiHata(e, t('DomainsPage:owner_modal.error'))) }
    finally { setIsleniyor(false) }
  }

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('DomainsPage:title') }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">{t('DomainsPage:title')}</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        {t('DomainsPage:subtitle')}
      </p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}
      {uyari && (
        <div className="mb-3 px-3 py-2.5 bg-amber-50 dark:bg-amber-900/20 border border-amber-300 dark:border-amber-800 rounded-md text-sm text-amber-800 dark:text-amber-300 flex items-start gap-2">
          <span className="leading-none mt-0.5">⚠</span>
          <span className="flex-1">{uyari}</span>
          <button onClick={() => setUyari(null)} className="text-xs hover:underline shrink-0">{t('common:close')}</button>
        </div>
      )}

      {/* WordPress tipi seçildiyse kurulum tek tıkla erişilebilir olmalı —
          domain açıldı ama site henüz boş. */}
      {wpYonlendir && (
        <div className="mb-3 px-3 py-2.5 bg-sky-50 dark:bg-sky-900/20 border border-sky-200 dark:border-sky-800 rounded-md flex items-center gap-3 flex-wrap">
          <span className="text-sm text-sky-800 dark:text-sky-200">
            {t('DomainsPage:wp_redirect.text', { domain: wpYonlendir.alanAdi })}
          </span>
          <Link to={`/abonelikler/${wpYonlendir.id}/wordpress`}
            className="text-xs px-3 py-1.5 bg-sky-600 hover:bg-sky-700 text-white rounded font-medium">
            {t('DomainsPage:wp_redirect.button')}
          </Link>
          <button onClick={() => setWpYonlendir(null)}
            className="text-xs px-2 py-1.5 text-sky-700 dark:text-sky-300 hover:underline ml-auto">
            {t('common:close')}
          </button>
        </div>
      )}

      {/* Toolbar */}
      <div className="flex items-center gap-2 mb-3 flex-wrap">
        <div className="flex-1 max-w-md">
          <input type="text" value={q} onChange={e => setQ(e.target.value)}
            placeholder={t('DomainsPage:search_placeholder')}
            className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 outline-none" />
        </div>
        <span className="text-xs text-slate-500 dark:text-slate-500">{filtreli.length} / {items.length}</span>
        {/* Yenile — sağlama arka planda süren işler içerir (DNS, SSL, FPM
            geçişi); liste kendiliğinden tazelenmediğinde elle tetiklenebilsin.
            Sessiz tazeleme: tablo boşalmaz, yalnız simge döner. */}
        <button onClick={() => { setYenileniyor(true); yukle(true).finally(() => setYenileniyor(false)) }}
          disabled={yenileniyor}
          title={t('DomainsPage:refresh')}
          aria-label={t('DomainsPage:refresh')}
          className="inline-flex items-center gap-1.5 text-sm px-2.5 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 rounded-md disabled:opacity-50">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round"
            className={`h-4 w-4 ${yenileniyor ? 'animate-spin' : ''}`}>
            <path d="M21 12a9 9 0 1 1-2.64-6.36M21 3v6h-6" />
          </svg>
          <span className="hidden sm:inline">{t('DomainsPage:refresh')}</span>
        </button>
        <button onClick={olusturAc}
          className="ml-auto inline-flex items-center gap-1.5 text-sm px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md font-medium shadow-sm">
          <span className="text-base leading-none">+</span> {t('DomainsPage:new_domain')}
        </button>
      </div>

      {/* Toplu işlem barı */}
      {secili.size > 0 && (
        <div className="mb-3 px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-300 dark:border-amber-700 rounded-md flex items-center gap-2 flex-wrap">
          <span className="text-sm font-semibold text-amber-800 dark:text-amber-200">{t('DomainsPage:bulk.selected', { count: secili.size })}</span>
          <button onClick={() => durumDegistir('aktif')} disabled={isleniyor}
            className="text-xs px-3 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white rounded">
            {t('DomainsPage:bulk.activate')}
          </button>
          <button onClick={() => durumDegistir('pasif')} disabled={isleniyor}
            className="text-xs px-3 py-1.5 bg-slate-600 hover:bg-slate-700 text-white rounded">
            {t('DomainsPage:bulk.deactivate')}
          </button>
          {adminMiyim && (
            <button onClick={sahipAc} disabled={isleniyor}
              className="text-xs px-3 py-1.5 bg-brand-600 hover:bg-brand-700 text-white rounded">
              {t('DomainsPage:bulk.change_owner')}
            </button>
          )}
          <button onClick={() => { setSilOnayMetin(''); setSilOnay(true) }} disabled={isleniyor}
            className="text-xs px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white rounded font-medium">
            {t('DomainsPage:bulk.delete', { count: secili.size })}
          </button>
          <button onClick={() => setSecili(new Set())} disabled={isleniyor}
            className="text-xs px-3 py-1.5 border border-amber-300 dark:border-amber-700 text-amber-800 dark:text-amber-200 hover:bg-amber-100 dark:bg-amber-900/30 rounded">
            {t('DomainsPage:bulk.clear_selection')}
          </button>
        </div>
      )}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div>
      ) : items.length === 0 ? (
        <EmptyState baslik={t('DomainsPage:empty.title')}
          aciklama={t('DomainsPage:empty.desc')}
          buton={{ etiket: t('DomainsPage:empty.button'), onClick: olusturAc }} />
      ) : (
        <div className="lg:bg-white dark:lg:bg-slate-800 lg:border lg:border-slate-200 dark:lg:border-slate-700 lg:rounded-2xl lg:overflow-hidden">
          <div className="lg:overflow-x-auto">
            <table className={T.tablo}>
              <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700`}>
                <tr>
                  <th className={`${T.baslik} w-10 text-center`}>
                    <input type="checkbox"
                      checked={filtreli.length > 0 && secili.size === filtreli.length}
                      ref={ref => { if (ref) ref.indeterminate = secili.size > 0 && secili.size < filtreli.length }}
                      onChange={e => tumunuSec(e.target.checked)}
                      className="cursor-pointer" />
                  </th>
                  <th className={T.baslik}>{t('DomainsPage:table.domain_name')}</th>
                  <th className={T.baslik}>{t('DomainsPage:table.system_user')}</th>
                  <th className={T.baslik}>{t('DomainsPage:table.reseller')}</th>
                  <th className={T.baslik}>{t('DomainsPage:table.plan')}</th>
                  <th className={T.baslik}>{t('DomainsPage:table.php')}</th>
                  <th className={T.baslik}>{t('DomainsPage:table.disk')}</th>
                  <th className={T.baslik}>{t('DomainsPage:table.status')}</th>
                  <th className={T.baslik}>{t('DomainsPage:table.created')}</th>
                  <th className={`${T.baslik} text-right`}>{t('DomainsPage:table.actions')}</th>
                </tr>
              </thead>
              <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
                {filtreli.map(d => {
                  return (
                    <tr key={d.id} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800 transition ${secili.has(d.id) ? 'lg:bg-brand-50 dark:lg:bg-brand-900/20' : ''}`}>
                      <td className={T.hucreSecim}>
                        <input type="checkbox" checked={secili.has(d.id)}
                          onChange={() => togga(d.id)} className="cursor-pointer" />
                      </td>
                      <td className={T.hucreBaslikSecimli}>
                        <span className="inline-flex items-center gap-1.5">
                          <Link to={`/abonelikler/${d.id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300 font-medium">
                            {d.alan_adi}
                          </Link>
                          <a
                            href={`https://${d.alan_adi}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            title={t('DomainsPage:table.open_site')}
                            onClick={e => e.stopPropagation()}
                            className="text-slate-300 dark:text-slate-600 hover:text-brand-600 dark:hover:text-brand-400 transition"
                          >
                            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                              <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                            </svg>
                          </a>
                          {/* SSL rozeti ÜÇ durumlu. Eskiden yalnız var/yok vardı ve
                              self-signed sertifika da yeşil görünüyordu — oysa ziyaretçi
                              tam sayfa tarayıcı uyarısı alır, yani site fiilen açılmaz.
                              ssl_kaynak BOŞ ise (sütun eklenmeden önceki kayıtlar) kaynak
                              bilinmiyor demektir; kırmızı göstermek yanlış alarm olurdu,
                              o yüzden yeşilde bırakılır. */}
                          {(() => {
                            const kilitli = 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z'
                            const acik = 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zM9 11V7a3 3 0 016 0'
                            const selfSigned = d.ssl && d.ssl_kaynak === 'self-signed'
                            const baslik = !d.ssl
                              ? t('DomainsPage:table.ssl_none')
                              : selfSigned
                                ? t('DomainsPage:table.ssl_self_signed')
                                : t('DomainsPage:table.ssl_active') + (d.ssl_bitis ? t('DomainsPage:table.ssl_expires', { date: d.ssl_bitis }) : '')
                            const renk = !d.ssl ? 'text-slate-300 dark:text-slate-600'
                              : selfSigned ? 'text-red-500' : 'text-emerald-500'
                            return (
                              <span title={baslik}>
                                <svg className={`w-3.5 h-3.5 ${renk}`} fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
                                  <path strokeLinecap="round" strokeLinejoin="round" d={d.ssl ? kilitli : acik} />
                                </svg>
                              </span>
                            )
                          })()}
                        </span>
                        {d.is_demo && <span className="ml-2 text-[10px] uppercase tracking-wider bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 px-1.5 py-0.5 rounded">{t('DomainsPage:table.demo')}</span>}
                      </td>
                      <td className={T.hucre} data-etiket={t('DomainsPage:table.system_user')}>
                        <span className="font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500">{d.sistem_kullanici}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('DomainsPage:table.reseller')}>
                        {d.bayi_adi ? (
                          <span className="inline-flex flex-col leading-tight">
                            <span className="text-[11px] font-medium bg-sky-50 dark:bg-sky-900/20 text-sky-700 dark:text-sky-300 px-1.5 py-0.5 rounded w-fit">{d.bayi_adi}</span>
                            {d.bayi_paket_adi && <span className="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5">{d.bayi_paket_adi}</span>}
                          </span>
                        ) : (
                          <span className="text-[11px] font-medium bg-violet-50 dark:bg-violet-900/20 text-violet-700 dark:text-violet-300 px-1.5 py-0.5 rounded w-fit">{t('DomainsPage:table.admin')}</span>
                        )}
                      </td>
                      <td className={T.hucre} data-etiket={t('DomainsPage:table.plan')}>
                        {d.plan_ad ? <span className="text-slate-700 dark:text-slate-300">{d.plan_ad}</span> : <span className="text-slate-400 dark:text-slate-500 italic">—</span>}
                      </td>
                      <td className={T.hucre} data-etiket={t('DomainsPage:table.php')}>
                        <span className="font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500">{d.php_surum || '-'}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('DomainsPage:table.disk')}>
                        <span className="font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500">{fmtKB(d.boyut_kb)}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('DomainsPage:table.status')}>
                        <span className={`text-[10px] uppercase tracking-wider px-2 py-0.5 rounded font-semibold ${
                          d.durum === 'aktif' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-500'
                        }`}>{d.durum === 'aktif' ? t('common:active') : t('common:inactive')}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('DomainsPage:table.created')}>
                        <span className="font-mono text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500 whitespace-nowrap">{d.olusturulma || '-'}</span>
                      </td>
                      <td className={`${T.hucreAksiyon} lg:text-right`}>
                        <Link to={`/abonelikler/${d.id}/subdomainler`} className="text-xs text-slate-500 dark:text-slate-400 hover:text-brand-600 dark:hover:text-brand-400 lg:mr-3">{t('DomainsPage:table.add_subdomain')}</Link>
                        <Link to={`/abonelikler/${d.id}`} className="text-xs text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-300">{t('DomainsPage:table.manage')}</Link>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Domain Oluştur Modal */}
      {olusturAcik && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => !olusturuluyor && setOlusturAcik(false)}>
          <form onSubmit={olusturGonder} className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-lg p-5 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainsPage:create_modal.title')}</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">
              {t('DomainsPage:create_modal.desc')}
            </p>

            {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

            <div className="space-y-3">
              {/* Site tipi — ilk karar. Statik HTML'de veritabanı hiç açılmaz;
                  WordPress domaini açar ve kurulum ekranına yönlendirir (admin
                  kullanıcı/parola orada sorulur, otomatik üretilmez). */}
              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1.5">{t('DomainsPage:create_modal.type_label')}</label>
                <div className="grid grid-cols-3 gap-2">
                  {([
                    ['php', '🐘', t('DomainsPage:create_modal.type_php'), t('DomainsPage:create_modal.type_php_desc')],
                    ['wordpress', '📝', t('DomainsPage:create_modal.type_wordpress'), t('DomainsPage:create_modal.type_wordpress_desc')],
                    ['statik', '📄', t('DomainsPage:create_modal.type_statik'), t('DomainsPage:create_modal.type_statik_desc')],
                  ] as const).map(([tip, ikon, ad, ac]) => (
                    <button key={tip} type="button" disabled={olusturuluyor}
                      onClick={() => setFSiteTipi(tip)}
                      className={`text-left px-3 py-2.5 rounded-lg border transition ${
                        fSiteTipi === tip
                          ? 'border-brand-500 bg-brand-50 dark:bg-brand-900/20 ring-1 ring-brand-500'
                          : 'border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800'
                      }`}>
                      <div className="text-lg leading-none mb-1">{ikon}</div>
                      <div className="text-xs font-semibold text-slate-800 dark:text-slate-100">{ad}</div>
                      <div className="text-[10px] leading-snug text-slate-500 dark:text-slate-400 mt-0.5">{ac}</div>
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">{t('DomainsPage:create_modal.domain_label')} <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  value={fAlanAdi}
                  onChange={e => setFAlanAdi(e.target.value)}
                  placeholder={t('DomainsPage:create_modal.domain_placeholder')}
                  autoFocus
                  required
                  disabled={olusturuluyor}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/15 outline-none"
                />
                <div className="text-[11px] text-slate-400 dark:text-slate-500 mt-1">{t('DomainsPage:create_modal.domain_hint')}</div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">
                  {t('DomainsPage:create_modal.php_label')}
                  {modalVeriYuk && phpSurumler.length === 0 && <span className="ml-2 text-[11px] text-slate-400 dark:text-slate-500">{t('common:loading')}</span>}
                </label>
                <select
                  value={fPHPSurum}
                  onChange={e => setFPHPSurum(e.target.value)}
                  disabled={olusturuluyor}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 outline-none bg-white dark:bg-slate-800"
                >
                  {phpSurumler.length === 0
                    ? <option value="8.3">{t('DomainsPage:create_modal.php_default')}</option>
                    : phpSurumler.map(p => (
                        <option key={p.surum} value={p.surum}>{t('DomainsPage:create_modal.php_option', { version: p.surum, separator: p.aciklama ? t('DomainsPage:create_modal.php_separator') : '', description: p.aciklama || '' })}</option>
                      ))
                  }
                </select>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 dark:text-slate-500 mb-1">
                  {t('DomainsPage:create_modal.plan_label')}
                  {modalVeriYuk && planlar.length === 0 && <span className="ml-2 text-[11px] text-slate-400 dark:text-slate-500">{t('common:loading')}</span>}
                </label>
                <select
                  value={fPlanID}
                  onChange={e => setFPlanID(e.target.value === '' ? '' : Number(e.target.value))}
                  disabled={olusturuluyor}
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 outline-none bg-white dark:bg-slate-800"
                >
                  <option value="">{t('DomainsPage:create_modal.plan_none')}</option>
                  {planlar.map(p => (
                    <option key={p.id} value={p.id}>{p.ad}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
                  <input type="checkbox" checked={fSSL} onChange={e => { setFSSL(e.target.checked); if (!e.target.checked) setFWWW(false) }} disabled={olusturuluyor} className="rounded" />
                  {t('DomainsPage:create_modal.ssl_label')}
                </label>
                {fSSL && (
                  <>
                    <p className="mt-1.5 text-[11px] text-amber-600 dark:text-amber-400 leading-relaxed">
                      {items[0]?.ipv4
                        ? t('DomainsPage:create_modal.ssl_warning_with_ip', { ip: items[0].ipv4 })
                        : t('DomainsPage:create_modal.ssl_warning_no_ip')
                      }
                    </p>
                    <label className="mt-2.5 flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer pl-1">
                      <input type="checkbox" checked={fWWW} onChange={e => setFWWW(e.target.checked)} disabled={olusturuluyor} className="rounded" />
                      {t('DomainsPage:create_modal.www_label', { domain: fAlanAdi.trim() || t('DomainsPage:create_modal.www_placeholder_domain') })}
                    </label>
                    {fWWW && (
                      <p className="mt-1 text-[11px] text-slate-500 dark:text-slate-500 leading-relaxed pl-1">
                        {t('DomainsPage:create_modal.www_hint', { domain: fAlanAdi.trim() || t('DomainsPage:create_modal.www_placeholder_domain') })}
                      </p>
                    )}
                  </>
                )}
              </div>
            </div>

            <div className="flex justify-end gap-2 mt-5">
              <button type="button" onClick={() => setOlusturAcik(false)} disabled={olusturuluyor}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">{t('common:cancel')}</button>
              <button type="submit" disabled={olusturuluyor || !fAlanAdi.trim()}
                className="px-4 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm rounded font-medium inline-flex items-center gap-2">
                {olusturuluyor && (
                  <svg className="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
                    <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" opacity="0.3"/>
                    <path d="M22 12a10 10 0 0 1-10 10" stroke="currentColor" strokeWidth="3"/>
                  </svg>
                )}
                {olusturuluyor ? t('common:creating') : t('common:create')}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Oluşturma Sonucu Modal (FTP + DB parolaları) */}
      {olusturmaSonuc && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setOlusturmaSonuc(null)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-lg p-5 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-emerald-700 dark:text-emerald-300 mb-1">{t('DomainsPage:result_modal.title')}</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-4">
              <span className="font-mono text-slate-700 dark:text-slate-300">{olusturmaSonuc.alan_adi}</span>{t('DomainsPage:result_modal.desc_before')}<strong>{t('DomainsPage:result_modal.desc_strong')}</strong>{t('DomainsPage:result_modal.desc_after')}
            </p>

            <div className="space-y-3">
              <div className="border border-slate-200 dark:border-slate-700 rounded-md p-3 bg-slate-50 dark:bg-slate-900">
                <div className="text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-500 font-semibold mb-2">{t('DomainsPage:result_modal.ftp')}</div>
                <KopyaSatir e={t('DomainsPage:result_modal.host')} v={olusturmaSonuc.ftp_host || '—'} kopyala={panoYaz} />
                <KopyaSatir e={t('DomainsPage:result_modal.user')} v={olusturmaSonuc.ftp_user} kopyala={panoYaz} />
                <KopyaSatir e={t('DomainsPage:result_modal.password')} v={olusturmaSonuc.olusturulan_parolalar.ftp} kopyala={panoYaz} parola />
              </div>

              <div className="border border-slate-200 dark:border-slate-700 rounded-md p-3 bg-slate-50 dark:bg-slate-900">
                <div className="text-[10px] uppercase tracking-wider text-slate-500 dark:text-slate-500 font-semibold mb-2">{t('DomainsPage:result_modal.mysql')}</div>
                <KopyaSatir e={t('DomainsPage:result_modal.host')} v={olusturmaSonuc.db_host || 'localhost'} kopyala={panoYaz} />
                <KopyaSatir e={t('DomainsPage:result_modal.database')} v={olusturmaSonuc.db_adi} kopyala={panoYaz} />
                <KopyaSatir e={t('DomainsPage:result_modal.user')} v={olusturmaSonuc.db_user} kopyala={panoYaz} />
                <KopyaSatir e={t('DomainsPage:result_modal.password')} v={olusturmaSonuc.olusturulan_parolalar.db} kopyala={panoYaz} parola />
              </div>

              {olusturmaSonuc.nameserver && (
                <div className="border border-emerald-200 dark:border-emerald-800 rounded-md p-3 bg-emerald-50 dark:bg-emerald-900/20">
                  <div className="text-[10px] uppercase tracking-wider text-emerald-700 dark:text-emerald-300 font-semibold mb-2">{t('DomainsPage:result_modal.nameservers')}</div>
                  <KopyaSatir e="NS1" v={olusturmaSonuc.nameserver.ns1} kopyala={panoYaz} />
                  <KopyaSatir e="NS2" v={olusturmaSonuc.nameserver.ns2} kopyala={panoYaz} />
                  <p className="text-[11px] text-emerald-800 dark:text-emerald-300 mt-2">{t('DomainsPage:result_modal.ns_note')}</p>
                </div>
              )}

              <div className="text-[11px] text-slate-500 dark:text-slate-500 italic">
                {t('DomainsPage:result_modal.system_user')} <span className="font-mono">{olusturmaSonuc.sistem_kullanici}</span>
              </div>
            </div>

            <div className="flex justify-end gap-2 mt-5">
              <button onClick={async () => {
                  const ok = await panoYaz(sonucMetni(olusturmaSonuc))
                  if (ok) { setSonucKopyalandi(true); setTimeout(() => setSonucKopyalandi(false), 1500) }
                }}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">
                {sonucKopyalandi ? t('DomainsPage:result_modal.copied') : t('DomainsPage:result_modal.copy_all')}
              </button>
              <button onClick={() => sonucTxtIndir(olusturmaSonuc)}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">
                {t('DomainsPage:result_modal.save_txt')}
              </button>
              <button onClick={() => setOlusturmaSonuc(null)}
                className="px-4 py-1.5 bg-slate-700 hover:bg-slate-800 text-white text-sm rounded">{t('common:ok')}</button>
            </div>
          </div>
        </div>
      )}

      {/* Toplu Sil Onay */}
      {silOnay && (() => {
        const tekId = secili.size === 1 ? Array.from(secili)[0] : undefined
        const tekDomain = tekId !== undefined ? items.find(x => x.id === tekId)?.alan_adi : undefined
        const beklenenMetin = tekDomain || t('DomainsPage:delete_modal.confirm_word')
        const silOnaylandi = silOnayMetin === beklenenMetin
        return (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setSilOnay(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-md p-5 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-red-700 dark:text-red-300 mb-2">{t('DomainsPage:delete_modal.title')}</h3>
            <p className="text-sm text-slate-700 dark:text-slate-300 mb-3">
              {t('DomainsPage:delete_modal.desc', { count: secili.size })}
            </p>
            <ul className="text-xs font-mono text-slate-500 dark:text-slate-500 bg-slate-50 dark:bg-slate-900 rounded p-2 max-h-40 overflow-auto mb-4">
              {Array.from(secili).slice(0, 8).map(id => {
                const d = items.find(x => x.id === id)
                return <li key={id} className="truncate">{d?.alan_adi || '?'}</li>
              })}
              {secili.size > 8 && <li className="text-slate-400 dark:text-slate-500 italic">{t('DomainsPage:delete_modal.more', { count: secili.size - 8 })}</li>}
            </ul>
            <label className="block text-xs text-slate-500 dark:text-slate-500 mb-1.5">
              {t('DomainsPage:delete_modal.confirm_label', { text: beklenenMetin })}
            </label>
            <input
              type="text"
              autoFocus
              value={silOnayMetin}
              onChange={e => setSilOnayMetin(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && silOnaylandi && !isleniyor) topluSil() }}
              placeholder={beklenenMetin}
              autoComplete="off"
              spellCheck={false}
              className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono bg-white dark:bg-slate-900 text-slate-800 dark:text-slate-200 mb-4 focus:outline-none focus:ring-2 focus:ring-red-500"
            />
            <div className="flex justify-end gap-2">
              <button onClick={() => setSilOnay(false)}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">{t('common:cancel')}</button>
              <button onClick={topluSil} disabled={isleniyor || !silOnaylandi}
                className="px-3 py-1.5 bg-red-600 hover:bg-red-700 disabled:opacity-40 disabled:cursor-not-allowed text-white text-sm rounded font-medium">
                {t('DomainsPage:delete_modal.confirm_yes')}
              </button>
            </div>
          </div>
        </div>
        )
      })()}

      {/* Sahip değiştir — domain transferinin tek yolu. Sahiplik zinciri
          domains.customer_id -> customers.owner_user_id üzerinden yürüdüğü
          için, domaini başka bir bayiye geçirmek demek onu O BAYİYE AİT bir
          müşteri kaydına bağlamak demektir. */}
      <Modal acik={sahipAcik} baslik={t('DomainsPage:owner_modal.title')} onKapat={() => setSahipAcik(false)}>
        <div className="space-y-3">
          <p className="text-sm text-slate-600 dark:text-slate-400">
            {t('DomainsPage:owner_modal.desc', { count: secili.size })}
          </p>
          <select value={sahipHedef} onChange={e => setSahipHedef(e.target.value)}
            className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 text-slate-900 dark:text-slate-100">
            <option value="">{t('DomainsPage:owner_modal.none')}</option>
            {musteriler.map(m => (
              <option key={m.id} value={m.id}>{m.ad}{m.eposta ? ` — ${m.eposta}` : ''}</option>
            ))}
          </select>
          <p className="text-xs text-slate-500 dark:text-slate-400">
            {t('DomainsPage:owner_modal.hint')}
          </p>
          <div className="flex justify-end gap-2 pt-1">
            <button onClick={() => setSahipAcik(false)}
              className="text-sm px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300">
              {t('common:giveUp')}
            </button>
            <button onClick={sahipUygula} disabled={isleniyor}
              className="text-sm px-3 py-1.5 rounded-lg bg-brand-600 hover:bg-brand-700 text-white disabled:opacity-50">
              {isleniyor ? t('DomainsPage:owner_modal.applying') : t('DomainsPage:owner_modal.apply')}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

function KopyaSatir({ e, v, kopyala, parola }: { e: string; v: string; kopyala: (m: string) => Promise<boolean>; parola?: boolean }) {
  const { t } = useTranslation('DomainsPage')
  const [kopyalandi, setKopyalandi] = useState(false)
  const [acik, setAcik] = useState(!parola)
  async function tikla() {
    const ok = await kopyala(v)
    if (ok) { setKopyalandi(true); setTimeout(() => setKopyalandi(false), 1500) }
  }
  return (
    <div className="flex items-center gap-2 text-xs py-1">
      <span className="w-24 text-slate-500 dark:text-slate-500 shrink-0">{e}</span>
      <code
        onClick={tikla}
        className={`flex-1 font-mono px-2 py-1 rounded border cursor-pointer select-all transition ${
          kopyalandi ? 'border-emerald-300 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300' : 'border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:border-brand-400 text-slate-800 dark:text-slate-200'
        }`}
        title={t('DomainsPage:result_modal.copy_hint')}
      >
        {parola && !acik ? '••••••••••' : v}
      </code>
      {parola && (
        <button type="button" onClick={() => setAcik(s => !s)}
          className="text-[10px] px-1.5 py-0.5 rounded border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400 dark:text-slate-500 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800">
          {acik ? t('DomainsPage:result_modal.hide') : t('DomainsPage:result_modal.show')}
        </button>
      )}
      {kopyalandi && <span className="text-[10px] text-emerald-600 dark:text-emerald-400 font-semibold">{t('DomainsPage:result_modal.copied')}</span>}
    </div>
  )
}
