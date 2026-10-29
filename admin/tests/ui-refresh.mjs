import {acceptLegal} from './consent.mjs'
import {chromium} from '@playwright/test'
import fs from 'node:fs'
import assert from 'node:assert/strict'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>[x.slice(0,x.indexOf('=')),x.slice(x.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV,'true')
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
const page=await browser.newPage({viewport:{width:1440,height:1000}})
const pending=new Set();page.on('request',r=>pending.add(r));page.on('requestfinished',r=>pending.delete(r));page.on('requestfailed',r=>pending.delete(r));
const errors=[];page.on('pageerror',e=>errors.push(e.message))

await page.route('**/api/auth/code',r=>r.abort());await page.route('**/api/service/mail',r=>r.abort())
try {
 await page.goto('http://localhost:5252/login');await page.waitForFunction(()=>![...document.querySelectorAll('.el-loading-mask')].some(e=>e.getClientRects().length));
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});for(const selector of ['#email','#code','#submit','#passkey','#qr-open']){const box=await page.locator(selector).boundingBox();assert.equal(Math.round(box.height),['#passkey','#qr-open'].includes(selector)?80:42,selector)}assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.screenshot({path:'../test-results/refresh-login-'+width+'.png',fullPage:true})}
 await page.locator('[data-mode="password"]').click();await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL);await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD);await acceptLegal(page);await page.locator('#submit').click();await page.waitForURL('**/#/account/profile');await page.getByRole('heading',{name:'账号与安全',exact:true}).waitFor();
 const pages=[['manage/overview','欢迎回来'],['manage/users','账号管理'],['manage/apps','项目管理'],['manage/keys','API 凭证'],['manage/mail','项目邮件'],['manage/audit','安全日志'],['manage/docs','接入文档'],['manage/settings','邮件与系统设置'],['manage/health','运行状态'],['account/profile','账号与安全'],['manage/apps/edit','创建项目']]
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});for(const [path,title] of pages){await page.goto('http://localhost:5252/#/'+path);await page.locator('.dy-page h1').filter({hasText:title}).waitFor();await page.waitForFunction(()=>![...document.querySelectorAll('.dy-page .el-loading-mask')].some(e=>e.getClientRects().length));await page.waitForFunction(()=>![...document.querySelectorAll('.el-loading-mask')].some(e=>e.getClientRects().length));await page.waitForFunction(()=>{let e=document.querySelector('.dy-page');if(!e)return false;while(e){if(Number(getComputedStyle(e).opacity)<1)return false;e=e.parentElement}return true});assert.equal(await page.locator('.dy-error:visible').count(),0,path);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,path+' width '+width);if(width===390)await page.waitForFunction(()=>!document.querySelector('.menu-left-open'));const nameBox=await page.locator('.dy-header-username').boundingBox();assert.ok(width-nameBox.x-nameBox.width>=16,'username right padding');await page.getByRole('button',{name:'页签菜单',exact:true}).locator('svg').waitFor();await page.screenshot({path:'../test-results/refresh-'+path.replaceAll('/','-')+'-'+width+'.png',fullPage:true,animations:'disabled'})}}
 await page.goto('http://localhost:5252/#/manage/health');await page.getByRole('heading',{name:'运行状态',exact:true}).waitFor();await page.getByRole('button',{name:'查看',exact:true}).click();await page.waitForFunction(()=>![...document.querySelectorAll('.el-loading-mask')].some(e=>e.getClientRects().length));assert.equal(await page.locator('.dy-error:visible').count(),0)
 assert.deepEqual(errors,[]);console.log('PASS: login controls 42px, 11 administration/account views at 1440/390px, no horizontal overflow or runtime errors, logout status filter. No SMTP.')
} finally {await browser.close()}
