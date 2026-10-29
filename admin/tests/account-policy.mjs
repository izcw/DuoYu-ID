import {acceptLegal} from './consent.mjs'
import { chromium } from '@playwright/test'
import fs from 'node:fs'
import { execFileSync } from 'node:child_process'
import { randomBytes,createHash } from 'node:crypto'
import assert from 'node:assert/strict'

const env = Object.fromEntries(fs.readFileSync('../.env.local', 'utf8').split(/\r?\n/).filter(x => x.includes('=')).map(x => [x.slice(0,x.indexOf('=')),x.slice(x.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV, 'true')
const id = 'ui-policy-' + randomBytes(6).toString('hex')
function sql(statement) {
  try { return execFileSync('docker', ['exec','duoyu-postgres-dev','psql','-U','duoyu','-d','duoyu','-v','ON_ERROR_STOP=1','-Atc',statement], {encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim() }
  catch { throw Error('Fixture database operation failed') }
}
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless:true })
const page = await browser.newPage({viewport:{width:1440,height:1000}})
const errors=[];page.on('pageerror',e=>errors.push(e.message))
await page.route('**/api/auth/code',r=>r.abort())
await page.route('**/api/service/mail',r=>r.abort())
let uploadedAvatar='';
try {
  const accountID = sql(`INSERT INTO users(id,email,name) VALUES('${id}','${id}@example.com','昵称测试') RETURNING account_id` ).split('\n')[0]
  assert.match(accountID,/^[1-9][0-9]{9}$/)
  await page.goto('http://localhost:5252/login')
  await page.locator('[data-mode="password"]').click()
  await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL)
  await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD)
  await acceptLegal(page);await page.locator('#submit').click()
  await page.waitForURL('**/#/account/profile')
  await page.goto('http://localhost:5252/#/manage/users/detail?id='+id)
  await page.getByRole('heading',{name:'昵称测试',exact:true}).waitFor()
  assert.match(await page.locator('.dy-page').innerText(),new RegExp(accountID))
  assert.equal(await page.getByRole('button',{name:'保存昵称',exact:true}).count(),0)
  assert.equal(await page.locator('.dy-panel input').count(),0)
  const denied=await page.evaluate(async id=>(await fetch('/api/admin/users/'+id,{method:'PATCH',headers:{'Content-Type':'application/json','X-Duoyu-Request':'1','X-Duoyu-Legal-Version':'2026-10-29','X-Duoyu-Adult':'true'},body:JSON.stringify({name:'不应成功'})})).status,id);assert.equal(denied,403)
  const appA=id+'-a',appB=id+'-b';sql(`INSERT INTO apps(id,name,redirects,public) VALUES('${appA}','项目归属甲',ARRAY['http://localhost:8199/callback'],true),('${appB}','项目归属乙',ARRAY['http://localhost:8199/callback'],true);INSERT INTO app_users(app_id,user_id) VALUES('${appA}','${id}'),('${appB}','${id}');`)
  // Disposable authenticated user session: no SMTP and no administrator impersonation API.
  const token=randomBytes(32).toString('base64url'),hash=createHash('sha256').update(token).digest('hex');sql(`INSERT INTO sessions(hash,user_id,expires_at,amr) VALUES('${hash}','${id}',now()+interval '1 hour','pwd')`)
  const own=await browser.newContext();await own.addCookies([{name:'duoyu_session',value:token,url:'http://localhost:5252',httpOnly:true}]);const userPage=await own.newPage();await userPage.goto('http://localhost:5252/#/account/profile');await userPage.getByRole('button',{name:'保存资料',exact:true}).waitFor();await userPage.locator('.el-form input').first().fill('新的昵称 😀');const saved=userPage.waitForResponse(r=>r.url().endsWith('/api/me')&&r.request().method()==='PATCH');await userPage.getByRole('button',{name:'保存资料',exact:true}).click();assert.equal((await saved).status(),200)
  const avatarResponse=userPage.waitForResponse(r=>r.url().endsWith('/api/me/avatar'));await userPage.locator('input[type=file]').setInputFiles('public/logo.png');const avatarData=await(await avatarResponse).json();assert.equal(avatarData.code,200);uploadedAvatar=avatarData.data.avatar;assert.match(uploadedAvatar,/^\/avatars\/[A-Za-z0-9_-]{43}\.png$/);await own.close()
  await page.reload();await page.getByRole('heading',{name:'新的昵称 😀',exact:true}).waitFor();await page.getByText('项目归属甲 · 用户',{exact:true}).waitFor();await page.getByText('项目归属乙 · 用户',{exact:true}).waitFor();await page.waitForFunction(()=>document.querySelector('.dy-account-detail-title img')?.naturalWidth===256);assert.equal(await page.locator('.dy-panel input').count(),0)
  await page.screenshot({path:'../test-results/account-readonly.png',fullPage:true})
  for (const width of [1440,390]) {
    await page.setViewportSize({width,height:1000})
    await page.goto('http://localhost:5252/#/manage/users')
    await page.getByPlaceholder('搜索名称、邮箱或标识').fill(accountID)
    await page.getByRole('button',{name:'刷新',exact:true}).click()
    await page.waitForFunction(id=>document.querySelectorAll('.el-table__body tbody tr').length===1 && document.querySelector('.el-table__body tbody tr')?.textContent.includes(id),accountID)
    await page.waitForFunction(()=>![...document.querySelectorAll('.el-loading-mask')].some(e=>e.getClientRects().length))
    const row=page.locator('.el-table__body tbody tr').first();assert.equal(await row.getByRole('link',{name:'管理项目：项目归属甲',exact:true}).count(),1);assert.equal(await row.getByRole('link',{name:'管理项目：项目归属乙',exact:true}).count(),1);assert.ok((await row.getByRole('link',{name:'管理项目：项目归属甲',exact:true}).getAttribute('href')).includes(appA));await page.waitForFunction(()=>document.querySelector('.el-table__body tbody tr img')?.naturalWidth===256)
    const rects=await row.locator('.dy-actions button').evaluateAll(nodes=>nodes.map(n=>{const r=n.getBoundingClientRect();return {top:r.top,bottom:r.bottom,left:r.left,right:r.right}}))
    assert.equal(rects.length,3)
    assert.ok(rects.every(r=>Math.abs(r.top-rects[0].top)<1),'action buttons wrap')
    assert.ok(rects.slice(1).every((r,i)=>r.left>=rects[i].right),'action buttons overlap')
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
    await row.locator('.dy-actions button').last().scrollIntoViewIfNeeded()
    await page.screenshot({path:`../test-results/account-actions-${width}.png`,fullPage:true,animations:'disabled'})
  }
  await page.getByRole('link',{name:'管理项目：项目归属甲',exact:true}).click();await page.getByRole('heading',{name:'编辑项目',exact:true}).waitFor();assert.ok(page.url().includes(appA));
  assert.deepEqual(errors,[])
  console.log('PASS: account projects, admin nickname blocked, self nickname edit, uploaded avatar and desktop/mobile actions; no SMTP.')
} finally {
  await browser.close()
  sql(`DELETE FROM app_users WHERE user_id='${id}';DELETE FROM apps WHERE id IN ('${id}-a','${id}-b');DELETE FROM sessions WHERE user_id='${id}';DELETE FROM audit WHERE target='${id}' OR user_id='${id}'; DELETE FROM users WHERE id='${id}'`);if(/^\/avatars\/[A-Za-z0-9_-]{43}\.png$/.test(uploadedAvatar))fs.rmSync('../data'+uploadedAvatar,{force:true})
}
