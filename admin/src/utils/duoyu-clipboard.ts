export async function copyDuoyuText(text: string) {
  if (navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return } catch { /* LAN HTTP fallback below. */ }
  }
  const previous = document.activeElement as HTMLElement | null
  const input = document.createElement('textarea')
  input.value = text
  input.readOnly = true
  input.style.cssText = 'position:fixed;left:-9999px;top:0;opacity:0'
  document.body.appendChild(input)
  input.focus(); input.select()
  try { if (!document.execCommand('copy')) throw new Error('浏览器未允许复制，请使用 HTTPS 或下载文档') }
  finally { input.remove(); previous?.focus() }
}
