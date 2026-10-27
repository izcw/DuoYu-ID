import {chromium} from '@playwright/test'
import fs from 'node:fs'
import {execFileSync} from 'node:child_process'
import {randomBytes} from 'node:crypto'
import assert from 'node:assert/strict'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>[x.slice(0,x.indexOf('=')),x.slice(x.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV,'true')
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
const page=await browser.newPage({viewport:{width:1440,height:1000}})
const errors=[];page.on('pageerror',e=>errors.push(e.message))
let appID='',keyID='';const name='页面联调-'+randomBytes(5).toString('hex')
try{
 await page.goto('http://localhost:8080/login');await page.locator('[data-mode="password"]').click();await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL);await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD);await page.locator('#submit').click();await page.waitForURL('**/#/account/profile');await page.getByRole('heading',{name:'账号与安全',exact:true}).waitFor()
 await page.goto('http://localhost:8080/#/manage/apps');await page.getByRole('button',{name:'＋ 创建项目',exact:true}).click();await page.waitForURL('**/#/manage/apps/edit');await page.getByRole('heading',{name:'创建项目',exact:true}).waitFor();assert.equal(await page.getByRole('dialog').count(),0)
 await page.getByPlaceholder('输入便于识别的名称').fill(name);await page.locator('label.el-radio').filter({hasText:'公共客户端'}).click();await page.getByPlaceholder('https://app.example.com/auth/callback').fill('http://localhost:8091/callback')
 await page.screenshot({path:'../test-results/app-edit-page.png',fullPage:true,animations:'disabled'})
 let response=page.waitForResponse(r=>r.url().endsWith('/api/admin/apps')&&r.request().method()==='POST');await page.getByRole('button',{name:'创建',exact:true}).click();let result=await(await response).json();assert.equal(result.code,200);appID=result.data.id;await page.waitForURL('**/#/manage/apps')
 await page.goto('http://localhost:8080/#/manage/apps/grants?id='+appID);await page.getByRole('heading',{name:'项目准入名单',exact:true}).waitFor();await page.getByRole('button',{name:'保存名单',exact:true}).click();await page.waitForURL('**/#/manage/apps')
 await page.goto('http://localhost:8080/#/manage/keys/edit');await page.getByRole('heading',{name:'创建 API 凭证',exact:true}).waitFor();await page.getByPlaceholder('输入便于识别的名称').fill(name+' key');await page.locator('.el-select').click();await page.getByRole('option',{name,exact:true}).click();assert.equal(await page.getByRole('radio',{name:'永久有效',exact:true}).isChecked(),true);await page.locator('label.el-checkbox').filter({hasText:'发送项目邮件'}).click();await page.locator('label.el-checkbox').filter({hasText:'查询邮件状态'}).click()
 response=page.waitForResponse(r=>r.url().endsWith('/api/admin/keys')&&r.request().method()==='POST');await page.getByRole('button',{name:'创建',exact:true}).click();result=await(await response).json();assert.equal(result.code,200);keyID=result.data.id;await page.getByRole('button',{name:'已保存，返回列表',exact:true}).click();await page.waitForURL('**/#/manage/keys')
 await page.goto('http://localhost:8080/#/manage/keys/edit?id='+keyID);await page.getByRole('heading',{name:'编辑 API 凭证',exact:true}).waitFor();await page.getByPlaceholder('输入便于识别的名称').fill(name+' renamed');await page.getByRole('button',{name:'取消',exact:true}).click();await page.getByRole('dialog').waitFor();await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click();await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForURL('**/#/manage/keys')
 assert.match(await page.locator('tr').filter({hasText:name+' renamed'}).innerText(),/永久有效/);assert.doesNotMatch(await page.locator('tr').filter({hasText:name+' renamed'}).innerText(),/已过期/);
 const detail=await page.evaluate(id=>fetch('/api/admin/keys/'+id).then(r=>r.json()),keyID);assert.equal(detail.data[0].expires_at,null);assert.deepEqual(detail.data[0].scopes,['app:read','mail:send','mail:read']);assert.equal(detail.data[0].name,name+' renamed')
 await page.goto('http://localhost:8080/#/manage/overview');await page.getByRole('heading',{name:'登录趋势',exact:true}).waitFor();await page.locator('.el-loading-mask').waitFor({state:'hidden'});await page.locator('.el-message').first().waitFor({state:'hidden'});await page.screenshot({path:'../test-results/overview-updated.png',fullPage:true,animations:'disabled'})
 await page.goto('http://localhost:8080/#/account/profile');await page.getByRole('button',{name:'修改',exact:true}).click();await page.waitForURL('**/#/account/security?action=password');await page.getByRole('heading',{name:'修改密码',exact:true}).waitFor();assert.equal(await page.getByRole('dialog').count(),0);await page.reload();await page.getByRole('heading',{name:'修改密码',exact:true}).waitFor()
 await page.setViewportSize({width:390,height:844});
 for(const path of ['manage/overview','manage/apps/edit','manage/mail']){await page.goto('http://localhost:8080/#/'+path);await page.getByRole('heading',{name:path==='manage/overview'?'登录趋势':path==='manage/mail'?'项目邮件':'创建项目',exact:true}).waitFor();await page.locator('.dy-page .el-loading-mask').waitFor({state:'hidden'});await page.waitForLoadState('networkidle');await page.waitForFunction(()=>{let el=document.querySelector('.dy-page');if(!el)return false;while(el){if(Number(getComputedStyle(el).opacity)<1)return false;el=el.parentElement}return true});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.screenshot({path:'../test-results/'+path.replaceAll('/','-')+'-mobile.png',fullPage:true})}
 const diagnostics=await page.evaluate(()=>fetch('/api/admin/diagnostics').then(r=>r.json()));assert.equal(diagnostics.code,200);assert.equal(diagnostics.data.issuer,'http://localhost:8080');assert.equal(typeof diagnostics.data.logout_queued,'number');
 assert.deepEqual(errors,[]);console.log('PASS: standalone app/grants/key pages, permanent key creation and edit, unsaved leave guard, real charts, refreshable account security page. No SMTP sent.')
}finally{
 await browser.close()
 if(appID){assert.match(appID,/^app_[A-Za-z0-9_-]+$/);execFileSync('docker',['exec','duoyu-postgres-dev','psql','-U','duoyu','-d','duoyu','-v','ON_ERROR_STOP=1','-q','-c',`DELETE FROM api_keys WHERE app_id='${appID}'; DELETE FROM grants WHERE app_id='${appID}'; DELETE FROM app_users WHERE app_id='${appID}'; DELETE FROM apps WHERE id='${appID}';`],{stdio:['ignore','pipe','pipe']})}
}
