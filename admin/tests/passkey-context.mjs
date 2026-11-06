import { acceptLegal } from './consent.mjs'
import { chromium } from '@playwright/test'
import assert from 'node:assert/strict'
const origin =
  process.env.DUOYU_CI_FIXTURE === '1' ? process.env.DUOYU_ORIGIN : 'http://localhost:5252'
assert.match(origin, /^http:\/\/(?:localhost|127\.0\.0\.1):\d+$/)
const browser = await chromium.launch({
  executablePath:
    process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe',
  headless: true
})
try {
  const page = await browser.newPage()
  // Serve the real local HTML and assets to a non-secure origin without LAN traffic.
  await page.route('http://passkey-fixture.invalid/**', async (route) => {
    const url = new URL(route.request().url())
    assert.equal(route.request().method(), 'GET')
    const response = await page.request.get(origin + url.pathname + url.search, { maxRedirects: 0 })
    await route.fulfill({ response })
  })
  await page.goto('http://passkey-fixture.invalid/login')
  assert.equal(await page.evaluate(() => window.isSecureContext), false)
  await acceptLegal(page)
  await page.locator('#passkey').click()
  await page.getByText(/当前页面使用不安全的 HTTP 地址/).waitFor()
  console.log('PASS: HTTP LAN explains secure-context restriction')
  const secure = await browser.newPage()
  await secure.addInitScript(() => {
    navigator.credentials.get = async () => {
      window.passkeyCalled = true
      throw new DOMException('test cancellation', 'NotAllowedError')
    }
  })
  await secure.goto(origin + '/login')
  assert.equal(await secure.evaluate(() => window.isSecureContext), true)
  await acceptLegal(secure)
  const begin = secure.waitForResponse((r) => r.url() === origin + '/api/auth/passkey/begin')
  await secure.locator('#passkey').click()
  assert.equal((await begin).status(), 200)
  await secure.waitForFunction(() => window.passkeyCalled === true)
  await secure.getByText('通行密钥操作已取消或超时。', { exact: true }).waitFor()
  console.log(
    'PASS: local secure context reaches navigator.credentials.get and handles cancellation'
  )
  await page.close()
  await secure.close()
} finally {
  await browser.close()
}
