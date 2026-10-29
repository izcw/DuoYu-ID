import { chromium } from '@playwright/test'
import assert from 'node:assert/strict'

// Anonymous login-page regression; no credentials or email are required.
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
try {
  const context = await browser.newContext()
  const page = await context.newPage()
  const errors = []
  const protectedRequests = []
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  page.on('pageerror', error => errors.push(error.message))
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/me') protectedRequests.push(request.url()) })
  for (const staleCookie of [false, true]) {
    if (staleCookie) await context.addCookies([{ name: 'duoyu_session', value: 'invalid-session-regression', url: 'http://localhost:5252', httpOnly: true }])
    const probe = page.waitForResponse(r => r.url().endsWith('/api/auth/session'))
    await page.goto('http://localhost:5252/login')
    const response = await probe
    assert.equal(response.status(), 200)
    assert.equal(response.headers()['cache-control'], 'no-store')
    assert.deepEqual((await response.json()).data, { authenticated: false })
    await page.locator('#form').waitFor({ state: 'visible' })
    await page.waitForLoadState('networkidle')
  }
  assert.deepEqual(protectedRequests, [])
  assert.deepEqual(errors, [])
  console.log('PASS: anonymous and invalid-cookie login pages return normal session state, no /api/me probe or console errors. No SMTP sent.')
} finally {
  await browser.close()
}
