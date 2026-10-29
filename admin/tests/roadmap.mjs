import {chromium} from '@playwright/test'
import {randomBytes,createHmac} from 'node:crypto'
import {execFileSync} from 'node:child_process'
import fs from 'node:fs'
import assert from 'node:assert/strict'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>[x.slice(0,x.indexOf('=')),x.slice(x.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV,'true')
const suffix=randomBytes(6).toString('hex'),uid='ops-'+suffix,app='ops-app-'+suffix,email=uid+'@example.invalid',key='ops-key-'+suffix
const sql=q=>{try{return execFileSync('docker',['exec','duoyu-postgres-dev','psql','-U','duoyu','-d','duoyu','-v','ON_ERROR_STOP=1','-q','-c',q],{stdio:['ignore','pipe','pipe']})}catch{throw Error('Fixture SQL failed')}}
const mac=v=>createHmac('sha256',Buffer.from(env.DUOYU_SECRET,'hex')).update(v).digest('hex')
const browser=await chromium.launch({executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
const errors=[]
const admin=await browser.newPage(),member=await browser.newPage()
for(const page of [admin,member]){page.on('pageerror',e=>errors.push(e.message));await page.route('**/api/auth/code',r=>r.abort());await page.route('**/api/service/mail',r=>r.abort());await page.route('https://api.iconify.design/**',r=>r.abort())}
const api=(page,path,method='GET',body)=>page.evaluate(async({path,method,body})=>{const r=await fetch(path,{method,headers:{'X-Duoyu-Request':'1','X-Duoyu-Legal-Version':'2026-10-29','X-Duoyu-Adult':'true','Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});return {status:r.status,...await r.json()}},{path,method,body})
try{
 sql(`INSERT INTO users(id,email,name) VALUES('${uid}','${email}','Operations test');INSERT INTO apps(id,name,secret_hash,redirects,logout_redirects,public) VALUES('${app}','Operations project','',ARRAY['http://localhost:8199/callback'],ARRAY[]::text[],true);INSERT INTO api_keys(id,name,digest,app_id,scopes) VALUES('${key}','Operations key','${mac('fixture-'+suffix)}','${app}',ARRAY['app:read']);INSERT INTO challenges(email,purpose,digest,expires_at) VALUES('${email}','login','${mac(email+'|login|123456')}',now()+interval '5 minutes')`)
 await admin.goto('http://localhost:5252/login')
 assert.equal((await api(admin,'/api/auth/login','POST',{mode:'password',email:env.DUOYU_ADMIN_EMAIL,password:env.DUOYU_ADMIN_PASSWORD})).status,200)
 await admin.goto('http://localhost:5252/#/manage/users/access?id='+uid)
 await admin.getByRole('heading',{name:'管理权限',exact:true}).waitFor()

 assert.equal(await admin.getByRole('button',{name:'保存权限',exact:true}).count(),0)
 for(const role of ['auditor','maintainer']) assert.equal((await api(admin,'/api/admin/users/'+uid+'/access','PUT',{role,app_ids:[app]})).status,403)
 await member.goto('http://localhost:5252/login');assert.equal((await api(member,'/api/auth/login','POST',{mode:'code',email,code:'123456'})).status,200)
 const info=await api(member,'/api/user/info');assert.deepEqual(info.data.roles,['R_USER'])
 for(const path of ['/api/admin/apps','/api/admin/users','/api/admin/diagnostics','/api/admin/audit'])assert.equal((await api(member,path)).status,403)
 await admin.goto('http://localhost:5252/#/account/profile');await admin.locator('.dy-super-badge').waitFor();
 await admin.goto('http://localhost:5252/#/manage/keys/edit?id='+key);await admin.getByRole('heading',{name:'轮换 API 凭证',exact:true}).waitFor();await admin.getByRole('button',{name:'生成新凭证',exact:true}).click();const rotated=admin.waitForResponse(r=>r.url().endsWith('/rotate')&&r.request().method()==='POST');await admin.getByRole('button',{name:'确定',exact:true}).click();assert.equal((await rotated).status(),200);await admin.getByRole('heading',{name:'保存你的密钥',exact:true}).waitFor();await admin.getByRole('button',{name:'已保存，返回列表'}).click()
 for(const width of [1440,390]){await admin.setViewportSize({width,height:1000});await admin.goto('http://localhost:5252/#/manage/users/access?id='+uid);await admin.getByRole('heading',{name:'管理权限',exact:true}).waitFor();await admin.waitForFunction(()=>!document.querySelector('.dy-page .el-loading-mask'));assert.equal(await admin.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);if(width===390){await admin.waitForFunction(()=>!document.querySelector('.menu-left-open'));assert.equal(await admin.getByRole('button',{name:'保存权限',exact:true}).count(),0)}await admin.screenshot({path:'../test-results/roadmap-access-'+width+'.png',fullPage:true})}
 assert.deepEqual(errors,[]);console.log('PASS: single super administrator badge; delegated roles denied; ordinary user management APIs denied; API key rotation UI; local icons; responsive access page. No SMTP.')
}finally{await browser.close();sql(`DELETE FROM sessions WHERE user_id='${uid}';DELETE FROM challenges WHERE email='${email}';DELETE FROM api_keys WHERE id='${key}';DELETE FROM users WHERE id='${uid}';DELETE FROM apps WHERE id='${app}'`)}
