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

const id='role_ui_'+randomBytes(6).toString('hex');
const sql=q=>execFileSync('docker',['exec','duoyu-postgres-dev','psql','-U','duoyu','-d','duoyu','-v','ON_ERROR_STOP=1','-q','-c',q],{stdio:['ignore','pipe','pipe']});
try{
 sql(`INSERT INTO apps(id,name,redirects,public) VALUES('${id}','角色界面测试',ARRAY['http://localhost:8199/callback'],true)`);
 await page.goto('http://localhost:5252/login');await page.locator('[data-mode="password"]').click();await page.locator('#email').fill(env.DUOYU_ADMIN_EMAIL);await page.locator('#password').fill(env.DUOYU_ADMIN_PASSWORD);await page.locator('#submit').click();await page.waitForURL('**/#/account/profile');await page.waitForLoadState('networkidle');
 await page.goto('http://localhost:5252/#/manage/apps/roles?id='+id);await page.getByRole('heading',{name:'项目角色',exact:true}).waitFor();await page.locator('.el-table__body tr').first().waitFor();assert.equal(await page.locator('.el-table__body tr').count(),2);
 await page.getByRole('button',{name:'新增角色',exact:true}).click();await page.getByRole('heading',{name:'新增角色',exact:true}).waitFor();await page.getByPlaceholder('例如：管理员').fill('编辑员');await page.getByPlaceholder('例如：admin、user、editor').fill('editor');await page.locator('textarea').fill('内容维护');await page.locator('.el-switch').click();await page.getByRole('button',{name:'创建角色',exact:true}).click();await page.waitForURL('**/roles?id='+id);await page.locator('.el-table__body tr').filter({hasText:'编辑员'}).waitFor();
 sql(`INSERT INTO app_users(app_id,user_id) SELECT '${id}',id FROM users WHERE admin`);
 const detail=await page.evaluate(async id=>({roles:(await(await fetch('/api/admin/apps/'+id+'/roles')).json()).data,users:(await(await fetch('/api/admin/apps/'+id+'/users')).json()).data}),id);const editor=detail.roles.find(r=>r.role_key==='editor');assert.equal(editor.is_default,true);assert.equal(detail.users[0].role_id,editor.id);
 await page.goto('http://localhost:5252/#/manage/apps/members?id='+id);await page.getByRole('button',{name:'修改角色',exact:true}).click();await page.getByRole('heading',{name:'修改用户项目角色'}).waitFor();await page.locator('.el-select').click();await page.getByRole('option',{name:'管理员（admin）',exact:true}).click();await page.getByRole('button',{name:'保存角色',exact:true}).click();await page.waitForURL('**/members?id='+id);await page.locator('.el-table__body tr').filter({hasText:'管理员'}).waitFor();
 await page.goto('http://localhost:5252/#/manage/apps/roles/edit?id='+id+'&role=user');await page.getByRole('heading',{name:'编辑角色'}).waitFor();await page.getByPlaceholder('例如：管理员').fill('基础用户');await page.getByPlaceholder('例如：admin、user、editor').fill('basic_user');await page.getByRole('button',{name:'保存修改'}).click();await page.waitForURL('**/roles?id='+id);await page.locator('.el-table__body tr').filter({hasText:'基础用户'}).waitFor();
 await page.goto('http://localhost:5252/#/manage/apps/roles/edit?id='+id+'&role='+editor.id);await page.getByRole('heading',{name:'编辑角色'}).waitFor();await page.locator('.el-select').click();await page.getByRole('option',{name:'管理员',exact:true}).click();await page.getByRole('button',{name:'删除角色',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'确定',exact:true}).click();await page.waitForURL('**/roles?id='+id);await page.locator('.el-table__body tr').filter({hasText:'管理员'}).waitFor();
 const roles=await page.evaluate(async id=>(await(await fetch('/api/admin/apps/'+id+'/roles')).json()).data,id);assert.equal(roles.length,2);assert.equal(roles.find(r=>r.is_default).role_key,'admin');
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});await page.waitForLoadState('networkidle');assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.screenshot({path:'../test-results/project-roles-'+width+'.png',fullPage:true})}
 await page.goto('http://localhost:5252/#/manage/apps/edit?id='+id);await page.getByRole('heading',{name:'编辑项目'}).waitFor();for(const label of ['公共客户端'])assert.equal(await page.getByRole('radio',{name:label,exact:true}).getAttribute('aria-checked'),'true');
 for(const input of await page.locator('.dy-choice-options .el-checkbox__input,.dy-choice-options .el-radio__input').all())assert.equal(await input.evaluate(e=>getComputedStyle(e).clipPath),'inset(50%)');
 assert.equal(await page.locator('.dy-choice-options img[src="/authenticator.png"]').count(),1);await page.waitForLoadState('networkidle');await page.screenshot({path:'../test-results/project-choice-icons.png',fullPage:true});assert.deepEqual(errors,[]);console.log('PASS role create/default/rename/delete, user role assignment, desktop/mobile, hidden native indicators and shared icons. No SMTP.');
}finally{await browser.close();sql(`DELETE FROM app_users WHERE app_id='${id}';DELETE FROM audit WHERE target LIKE '${id}%';DELETE FROM apps WHERE id='${id}';`)}
