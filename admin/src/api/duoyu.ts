export async function api<T = any>(
  path: string,
  method = 'GET',
  data?: unknown,
  signal?: AbortSignal
): Promise<T> {
  const form = data instanceof FormData
  const response = await fetch(path, {
    method,
    signal,
    credentials: 'same-origin',
    headers: { 'X-Duoyu-Request': '1', ...(form ? {} : { 'Content-Type': 'application/json' }) },
    body: data === undefined ? undefined : form ? data : JSON.stringify(data)
  })
  const result = await response.json()
  if (!response.ok) {
    if (response.status === 401) window.location.assign('/login')
    throw new Error(
      (result.msg || '请求失败') +
        (response.headers.get('X-Request-ID')
          ? '（请求 ' + response.headers.get('X-Request-ID') + '）'
          : '')
    )
  }
  return result.data as T
}
export const dateText = (value: string) => (value ? new Date(value).toLocaleString('zh-CN') : '—')
export const toBuffer = (v: string) =>
  Uint8Array.from(atob(v.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0))
export const toBase64 = (v: ArrayBuffer) =>
  btoa(String.fromCharCode(...new Uint8Array(v)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '')
