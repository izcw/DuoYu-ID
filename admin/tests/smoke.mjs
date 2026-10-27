import { chromium } from '@playwright/test'
import fs from 'node:fs'
import assert from 'node:assert/strict'

// Only local Mailpit is used. Never send a real email in this test.
const env = Object.fromEntries(fs.readFileSync('../.env.local', 'utf8').split(/\r?\n/).filter(x => x.includes('=')).map(x => [x.slice(0, x.indexOf('=')), x.slice(x.indexOf('=') + 1)]))
if (process.env.SKIP_MAIL !== 'true' || process.env.TEST_PASSKEY === 'true') {
  assert.equal(env.SMTP_PLAIN_LOCAL, 'true')
  assert.equal(env.SMTP_PORT, '11025')
}
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true })
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
const page = await context.newPage()
const errors = []
page.on('pageerror', e => errors.push(e.message))
fs.mkdirSync('../test-results', { recursive: true })
try {
  await page.goto('http://localhost:8080/login')
  await page.screenshot({ path: '../test-results/login-desktop.png', fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.screenshot({ path: '../test-results/login-mobile.png', fullPage: true })
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false)
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL)
  if (process.env.SKIP_MAIL !== 'true') {
  const sent = page.waitForResponse(r => r.url().endsWith('/api/auth/code'))
  await page.locator('#send').click()
  assert.equal((await sent).status(), 200)
  await page.locator('.el-message--success').waitFor()
  assert.match(await page.locator('.el-message--success').textContent(), /本机测试收件箱/)
  const messages = await (await fetch('http://localhost:18025/api/v1/messages')).json()
  const latest = messages.messages.find(m => m.To.some(to => to.Address === env.DUOYU_ADMIN_EMAIL))
  assert.ok(latest)
  const mail = await (await fetch('http://localhost:18025/api/v1/message/' + latest.ID)).json()
  const code = mail.Text.match(/验证码：(\d{6})/)[1]
  const mailPage = await context.newPage()
  await mailPage.setContent(mail.HTML)
  await mailPage.screenshot({ path: '../test-results/email.png', fullPage: true })
  await mailPage.close()
  }
  await page.locator('[data-mode="password"]').click()
  assert.equal(await page.locator('#code-wrap').isVisible(), false)
  await page.locator('#reset').click()
  assert.equal(await page.locator('#code-wrap').isVisible(), true)
  await page.locator('[data-mode="password"]').click()
  await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD)
  await page.getByRole('button', {name:'显示密码',exact:true}).click()
  assert.equal(await page.locator('#password').getAttribute('type'), 'text')
  await page.getByRole('button', {name:'隐藏密码',exact:true}).click()
  assert.equal(await page.locator('#password').getAttribute('type'), 'password')
  await page.screenshot({path:'../test-results/login-password.png',fullPage:true})
  await page.locator('#submit').click()
  await page.waitForURL('**/#/account/profile')
  await page.getByRole('heading', { name: '账号与安全' }).waitFor()
  await page.screenshot({ path: '../test-results/account.png', fullPage: true })
  for (const route of ['overview', 'users', 'apps', 'keys', 'audit', 'settings', 'mail', 'docs']) {
    await page.goto('http://localhost:8080/#/manage/' + route)
    await page.waitForLoadState('networkidle')
    const heading = {overview:'欢迎回来', users:'账号管理', apps:'项目管理', keys:'API 凭证', audit:'安全日志', settings:'邮件与系统设置', docs:'接入文档', mail:'项目邮件'}[route]
    await page.locator('.dy-page h1').filter({hasText:heading}).waitFor({state:'visible'})
    await page.locator('.dy-page .el-loading-mask').waitFor({state:'hidden'})
    assert.equal(await page.locator('.dy-error').count(), 0, route)
    await page.screenshot({ path: '../test-results/' + route + '.png', fullPage: true, animations: 'disabled' })
  }
  assert.deepEqual(errors, [])
  if (process.env.TEST_PASSKEY === 'true') {
    const cdp = await context.newCDPSession(page)
    await cdp.send('WebAuthn.enable')
    await cdp.send('WebAuthn.addVirtualAuthenticator', { options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true } })
    const getCode = async () => {
      // Honor the real cooldown instead of weakening server rate limits for tests.
      await new Promise(resolve => setTimeout(resolve, 61000))
      const status = await page.evaluate(async email => (await fetch('/api/auth/code', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Duoyu-Request': '1' }, body: JSON.stringify({email, purpose:'login'}) })).status, env.DUOYU_ADMIN_EMAIL)
      assert.equal(status, 200)
      const list = await (await fetch('http://localhost:18025/api/v1/messages')).json()
      const msg = list.messages.find(m => m.To.some(t => t.Address === env.DUOYU_ADMIN_EMAIL))
      const body = await (await fetch('http://localhost:18025/api/v1/message/' + msg.ID)).json()
      return body.Text.match(/验证码：(\d{6})/)[1]
    }
    await page.goto('http://localhost:8080/#/account/profile')
    await page.getByRole('button', {name: '添加', exact:true}).click()
    const code = await getCode()
    await page.getByPlaceholder('6 位验证码', {exact:true}).fill(code)
    const registered = page.waitForResponse(r => r.url().endsWith('/api/me/passkeys/finish'))
    await page.getByRole('button', {name:'继续',exact:true}).click()
    assert.equal((await registered).status(),200)
    await page.getByRole('dialog').waitFor({state:'hidden'})
    await page.evaluate(() => fetch('/api/auth/logout', {method:'POST',headers:{'X-Duoyu-Request':'1'}}))
    await page.goto('http://localhost:8080/login')
    await page.locator('#passkey').click()
    await page.waitForURL('**/#/account/profile')
    await page.getByRole('heading', {name:'账号与安全'}).waitFor()
    await page.getByRole('button', {name:'移除',exact:true}).last().click()
    const deleteCode = await getCode()
    await page.getByPlaceholder('6 位验证码', {exact:true}).fill(deleteCode)
    const removed = page.waitForResponse(r => r.request().method()==='DELETE' && r.url().includes('/api/me/passkeys/'))
    await page.getByRole('button', {name:'继续',exact:true}).click()
    assert.equal((await removed).status(),200)
    console.log('PASS: virtual authenticator passkey registration, discoverable login and authenticated deletion.')
  }
  console.log('PASS: responsive login, admin password-only login, recovery OTP visibility, profile and all admin pages; no browser runtime errors. Mail test: '+(process.env.SKIP_MAIL === 'true' ? 'skipped' : 'local Mailpit passed'))
} finally { await browser.close() }

