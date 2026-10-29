import {acceptLegal} from './consent.mjs'
import {chromium} from '@playwright/test'
import fs from 'node:fs'
import assert from 'node:assert/strict'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>[x.slice(0,x.indexOf('=')),x.slice(x.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV,'true')
const browser=await chromium.launch({executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
const page=await browser.newPage({viewport:{width:1440,height:1000}})
let total=21
const rows=()=>Array.from({length:total},(_,i)=>({id:String(i+1),account_id:String(1000000000+i),name:'分页用户 '+i,email:`page${i}@example.com`,avatar:'',enabled:true,public:true,scopes:[],status:'queued',category:'notification',created_at:'2026-10-28T00:00:00Z',expires_at:'2026-10-29T00:00:00Z',last_login_at:'2026-10-28T00:00:00Z',first_login_at:'2026-10-28T00:00:00Z',login_count:1,agent:'分页设备',action:'test',actor:'test',target:'test',success:true,app_id:'pagination-fixture',app_name:'分页项目'}))
try{
 await page.goto('http://localhost:5252/login');await page.locator('[data-mode=password]').click();await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL);await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD);await acceptLegal(page);await page.locator('#submit').click();await page.waitForURL('**/#/account/profile')
 await page.route('**/api/**',async route=>{
 const u=new URL(route.request().url()),p=u.pathname
 if(p==='/api/auth/code'||p==='/api/service/mail')return route.abort()
 if(p==='/api/admin/apps/pagination-fixture')return route.fulfill({json:{code:200,data:[{id:'pagination-fixture',name:'分页项目',redirects:[],logout_redirects:[],allowed_origins:[]}]}})
 if(p==='/api/me/sessions'||/\/logout-events\/[^/]+\/attempts$/.test(p))return route.fulfill({json:{code:200,data:rows()}})
 if(/^\/api\/admin\/(users|apps|keys|audit|mail|logout-events)$/.test(p)||p==='/api/admin/apps/pagination-fixture/users'){
 const n=Number(u.searchParams.get('page')||1);return route.fulfill({json:{code:200,data:{items:rows().slice((n-1)*20,n*20),total,page:n,page_size:20}}})
 }
 return route.continue()
 })
 for(const path of ['manage/users','manage/apps','manage/keys','manage/audit','manage/mail','manage/health','manage/apps/members?id=pagination-fixture','account/profile']){
 for(const count of [0,20,21]){
 total=count;await page.goto('http://localhost:5252/#/'+path);await page.reload();await page.locator('.dy-page .el-table').first().waitFor();await page.waitForLoadState('networkidle');
 const table=page.locator('.dy-page .el-table').first(),pager=page.locator('.dy-page .el-pagination');
 assert.equal(await table.locator('.el-table__header th').first().innerText(),'序号',path)
 assert.equal(await table.locator('.el-table__body > tbody > tr').count(),Math.min(count,20),path)
 assert.equal(await pager.count(),count>20?1:0,path)
 if(count>20){assert.equal(await pager.evaluate(e=>!!e.closest('.dy-panel')),true);await pager.locator('.btn-next').click();await page.waitForLoadState('networkidle');await page.waitForFunction(()=>document.querySelector('.dy-page .el-table__body tbody tr td')?.textContent?.trim()==='21' && document.querySelectorAll('.dy-page .el-table__body > tbody > tr').length===1);assert.equal(await table.locator('.el-table__body > tbody > tr').count(),1,path)}
 }
 console.log('PASS 0/20/21 records, panel pagination and continuous index: '+path)
 }
 total=21;await page.goto('http://localhost:5252/#/manage/health');await page.reload();await page.waitForLoadState('networkidle');await page.locator('.el-table__expand-icon').first().click();await page.waitForLoadState('networkidle');const detail=page.locator('.el-table__expanded-cell');assert.equal(await detail.locator('.el-table__body > tbody > tr').count(),20);await detail.locator('.el-pagination .btn-next').click();assert.equal(await detail.locator('.el-table__body > tbody > tr').count(),1);assert.equal((await detail.locator('.el-table__body td').first().innerText()).trim(),'21');console.log('PASS nested attempts pagination')
 await page.setViewportSize({width:390,height:844});await page.goto('http://localhost:5252/#/manage/users');await page.reload();await page.locator('.dy-page h1').filter({hasText:'账号管理'}).waitFor();await page.waitForLoadState('networkidle');await page.waitForFunction(()=>{let e=document.querySelector('.dy-page');if(!e)return false;while(e){if(Number(getComputedStyle(e).opacity)<1)return false;e=e.parentElement}return !document.querySelector('.dy-page .el-loading-mask')});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.screenshot({path:'../test-results/pagination-mobile.png',fullPage:true});
}finally{await browser.close()}
