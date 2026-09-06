import { modalKopyala } from './dialog'

export async function panoYaz(metin: string, mesaj: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(metin)
      return true
    }
  } catch { /* Eski tarayıcı/izin kısıtı: aşağıdaki yöntemi dene. */ }

  const oncekiOdak = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const ta = document.createElement('textarea')
  try {
    ta.value = metin
    ta.readOnly = true
    ta.style.position = 'fixed'
    ta.style.top = '0'
    ta.style.left = '0'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.focus({ preventScroll: true })
    ta.select()
    ta.setSelectionRange(0, metin.length)
    if (document.execCommand('copy')) return true
  } catch { /* Elle kopyalama modalına geç. */ }
  finally {
    ta.remove()
    oncekiOdak?.focus()
  }

  await modalKopyala(mesaj, metin)
  // Elle kopyalanıp kopyalanmadığını bilemeyiz; yanlış başarı rozeti gösterme.
  return false
}
