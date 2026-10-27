import {chromium} from '@playwright/test'
import fs from 'node:fs'
import assert from 'node:assert/strict'

// Visual fixture only. Go tests separately verify actual rendering and HTML escaping.
const source=fs.readFileSync('../server/templates/notification.html','utf8')
const values={Title:'任务处理完成',Text:'你的导出任务已完成。\n请登录项目查看结果。',Project:'示例项目',Logo:'http://localhost:8080/logo.png'}
const html=source.replace(/{{\.(\w+)}}/g,(_,key)=>values[key])
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
try{
 const page=await browser.newPage()
 for(const width of [800,390]){
  await page.setViewportSize({width,height:850});await page.setContent(html,{waitUntil:'networkidle'})
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:'../test-results/project-mail-template-'+width+'.png',fullPage:true})
 }
 console.log('PASS: default mail template desktop/mobile visual fixtures; no SMTP sent.')
}finally{await browser.close()}
