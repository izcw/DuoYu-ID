import {acceptLegal} from './consent.mjs'
import {chromium} from '@playwright/test'
import fs from 'node:fs'
import assert from 'node:assert/strict'
import {randomBytes} from 'node:crypto'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(l=>l.includes('=')).map(l=>[l.slice(0,l.indexOf('=')),l.slice(l.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV,'true');const origin=env.DUOYU_ORIGIN
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
const page=await browser.newPage();let keyID,token
const errors=[];page.on('pageerror',e=>errors.push(e.message))
try{
 await page.goto(origin+'/login');await page.locator('[data-mode="password"]').click();await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL);await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD);await acceptLegal(page);await page.locator('#submit').click();await page.waitForURL('**/#/account/profile');await page.getByRole('button',{name:'保存资料',exact:true}).waitFor()
 await page.goto(origin+'/#/manage/project-api');await page.getByRole('heading',{name:'MCP 连接',exact:true}).waitFor()
 await page.evaluate(()=>{Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>window.__mcpCopy=text}})})
 await page.getByRole('button',{name:'复制 MCP 配置',exact:true}).click();await page.waitForFunction(()=>window.__mcpCopy);const template=JSON.parse(await page.evaluate(()=>window.__mcpCopy));assert.equal(template.mcpServers.duoyu.url,origin+'/mcp');assert.match(template.mcpServers.duoyu.headers.Authorization,/YOUR_DEVELOPER_KEY/)
 await page.getByRole('button',{name:'创建开发者凭证',exact:true}).click();await page.getByPlaceholder('例如：本地项目接入 AI').fill('developer-'+randomBytes(6).toString('hex'));await page.locator('label.el-checkbox').filter({hasText:'发送项目邮件'}).click();const created=page.waitForResponse(r=>r.url().endsWith('/api/admin/developer-keys')&&r.request().method()==='POST');await page.getByRole('button',{name:'创建凭证',exact:true}).click();const data=(await(await created).json()).data;keyID=data.id;token=data.secret
 const rpc=async(method,params={})=>{const res=await fetch(origin+'/mcp',{method:'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json',Accept:'application/json, text/event-stream','MCP-Protocol-Version':'2025-06-18'},body:JSON.stringify({jsonrpc:'2.0',id:1,method,params})});assert.equal(res.status,200);return(await res.json()).result}
 const result=await rpc('initialize',{protocolVersion:'2025-06-18',capabilities:{},clientInfo:{name:'browser-test',version:'1'}});assert.equal(result.serverInfo.name,'duoyu-id')
 assert.ok((await rpc('tools/list')).tools.some(t=>t.name==='send_mail'))
 await page.getByRole('button',{name:'已保存，返回列表',exact:true}).click();await page.waitForURL('**/#/manage/project-api');await page.goto(origin+'/#/manage/project-api/edit?id='+keyID);await page.getByRole('heading',{name:'编辑开发者凭证',exact:true}).waitFor();await page.locator('label.el-checkbox').filter({hasText:'发送项目邮件'}).click();await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForURL('**/#/manage/project-api');assert.equal((await rpc('tools/list')).tools.some(t=>t.name==='send_mail'),false)
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});await page.getByRole('heading',{name:'MCP 连接',exact:true}).waitFor();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.waitForFunction(()=>{let e=document.querySelector('.dy-page');while(e){if(Number(getComputedStyle(e).opacity)<1)return false;e=e.parentElement}return !document.querySelector('.el-loading-mask')});await page.screenshot({path:'../test-results/developer-mcp-'+width+'.png',fullPage:true})}
 assert.deepEqual(errors,[]);console.log('PASS unified credential creation/edit, MCP configuration, initialize/tool discovery, immediate scope removal, desktop/mobile; no SMTP')
}finally{if(keyID)await page.evaluate(async id=>fetch('/api/admin/developer-keys/'+id+'/revoke',{method:'POST',headers:{'Content-Type':'application/json','X-Duoyu-Request':'1','X-Duoyu-Legal-Version':'2026-10-29','X-Duoyu-Adult':'true'},body:'{}'}),keyID).catch(()=>{});await browser.close()}
