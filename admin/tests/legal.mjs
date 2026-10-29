import {chromium} from '@playwright/test'
import fs from 'node:fs'
import assert from 'node:assert/strict'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(l=>l.includes('=')).map(l=>[l.slice(0,l.indexOf('=')),l.slice(l.indexOf('=')+1)]));assert.equal(env.DUOYU_DEV,'true');const origin=env.DUOYU_ORIGIN
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true});const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message))
try{
 let sends=0;await page.route('**/api/auth/code',r=>{sends++;return r.abort()})
 await page.goto(origin+'/login');assert.equal(await page.locator('#legal-accept').isChecked(),false);await page.locator('#email').fill('adult@example.invalid');await page.locator('#send').click();await page.getByText('请阅读并同意服务条款和隐私政策',{exact:true}).waitFor();assert.equal(sends,0)
 await page.locator('#passkey').click();assert.equal(sends,0)
 const popupWait=page.waitForEvent('popup');await page.getByRole('link',{name:'《隐私政策》',exact:true}).click();const policy=await popupWait;await policy.waitForLoadState('domcontentloaded');await policy.getByRole('heading',{name:'隐私政策',exact:true}).waitFor();assert.ok((await policy.locator('body').innerText()).includes('2405824084@qq.com'));assert.equal(await page.locator('#legal-accept').isChecked(),false);await policy.close()
 for(const path of ['/legal/terms','/legal/privacy']){const doc=await browser.newPage();await doc.goto(origin+path);assert.ok((await doc.locator('main').innerText()).includes('18周岁'));for(const width of [1440,390]){await doc.setViewportSize({width,height:900});assert.equal(await doc.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)}await doc.close()}
 await page.locator('[data-mode="password"]').click();await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL);await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD);await page.locator('#submit').click();assert.ok(page.url().includes('/login'))
 await page.locator('#legal-accept').check();await page.locator('#submit').click();await page.waitForURL('**/#/account/profile');await page.getByRole('button',{name:'保存资料',exact:true}).waitFor()
 assert.deepEqual(errors,[]);console.log('PASS unchecked by default, blocked OTP/passkey/password, readable policies and links without implicit consent, desktop/mobile, accepted login; no SMTP')
}finally{await browser.close()}
