import {chromium} from '@playwright/test'
import {createHmac,createHash,randomBytes} from 'node:crypto'
import {execFileSync} from 'node:child_process'
import fs from 'node:fs'
import http from 'node:http'
import assert from 'node:assert/strict'
import jsQR from 'jsqr'
import {PNG} from 'pngjs'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>[x.slice(0,x.indexOf('=')),x.slice(x.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV,'true');assert.equal(new URL(env.DUOYU_ORIGIN).protocol,'http:')
const origin=env.DUOYU_ORIGIN,suffix=randomBytes(6).toString('hex'),email='totp-'+suffix+'@example.invalid',ids=['totp_public_'+suffix,'totp_server_'+suffix],clientSecret=randomBytes(32).toString('hex')
const mac=v=>createHmac('sha256',Buffer.from(env.DUOYU_SECRET,'hex')).update(v).digest('hex')
const sql=q=>{try{return execFileSync('docker',['exec','duoyu-postgres-dev','psql','-U','duoyu','-d','duoyu','-v','ON_ERROR_STOP=1','-q','-c',q],{stdio:['ignore','pipe','pipe']})}catch{throw Error('Isolated fixture SQL failed')}}
function seed(purpose){sql(`INSERT INTO challenges(email,purpose,digest,expires_at) VALUES('${email}','${purpose}','${mac(email+'|'+purpose+'|123456')}',now()+interval '5 minutes') ON CONFLICT(email,purpose) DO UPDATE SET digest=EXCLUDED.digest,attempts=0,expires_at=EXCLUDED.expires_at`)}
function unbase32(v){let bits='';for(const c of v)bits+='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'.indexOf(c).toString(2).padStart(5,'0');return Buffer.from(bits.match(/.{8}/g).map(v=>parseInt(v,2)))}
function code(secret){const b=Buffer.alloc(8);b.writeBigUInt64BE(BigInt(Math.floor(Date.now()/30000)));const h=createHmac('sha1',unbase32(secret)).update(b).digest(),i=h[19]&15;return ((h.readUInt32BE(i)&0x7fffffff)%1000000).toString().padStart(6,'0')}
const callback=http.createServer((req,res)=>{res.end('OK')});await new Promise(r=>callback.listen(0,'127.0.0.1',r));
const browser=await chromium.launch({executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
try{
 const context=await browser.newContext({viewport:{width:1440,height:1000}}),p=await context.newPage();await p.route('**/api/auth/code',r=>r.abort());
 seed('login');await p.goto(origin+'/login');await p.locator('#email').fill(email);await p.locator('#code').fill('123456');await p.locator('#submit').click();await p.waitForURL('**/#/account/profile');await p.getByRole('heading',{name:'账号与安全',exact:true}).waitFor();
 await p.locator('.dy-security-item').filter({hasText:'身份验证器'}).getByRole('button',{name:'管理'}).click();await p.getByRole('heading',{name:'绑定身份验证器',exact:true}).waitFor();
 await p.waitForLoadState('networkidle');await p.waitForFunction(()=>{let e=document.querySelector('.dy-page');while(e){if(Number(getComputedStyle(e).opacity)<1)return false;e=e.parentElement}return true});for(const width of [1440,390]){await p.setViewportSize({width,height:1000});await p.screenshot({path:'../test-results/authenticator-settings-'+width+'.png',fullPage:true});assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)}
 seed('email');await p.getByPlaceholder('6 位邮箱验证码').fill('123456');await p.getByRole('button',{name:'验证邮箱并继续'}).click();await p.locator('.dy-authenticator-qr').waitFor();const secret=await p.locator('.dy-authenticator-secret').innerText();const png=PNG.sync.read(Buffer.from((await p.locator('.dy-authenticator-qr').getAttribute('src')).split(',')[1],'base64'));const uri=new URL(jsQR(new Uint8ClampedArray(png.data),png.width,png.height).data);assert.equal(uri.protocol,'otpauth:');assert.equal(uri.searchParams.get('secret'),secret);assert.equal(uri.searchParams.get('issuer'),'DuoYu ID');
 await p.getByPlaceholder('6 位动态验证码').fill(code(secret));await p.getByRole('button',{name:'确认绑定',exact:true}).click();await p.getByRole('heading',{name:'保存恢复码'}).waitFor();const recovery=await p.locator('.dy-recovery-codes code').allTextContents();assert.equal(recovery.length,8);await p.getByRole('button',{name:'已保存，完成'}).click();await p.waitForURL('**/#/account/profile');await p.waitForLoadState('networkidle');console.log('PASS enrollment UI, QR payload and recovery codes (no secrets logged)');
 await p.goto(origin+'/login');await p.locator('#switch').click();await p.locator('#form').waitFor({state:'visible'});
 // Only reset the consumed counter in this disposable fixture, to test without waiting for a clock boundary.
 sql(`UPDATE authenticators SET last_step=-1 WHERE user_id=(SELECT id FROM users WHERE email='${email}')`);
 await p.goto(origin+'/login');await p.locator('#authenticator-open').click();await p.locator('#authenticator-account').fill(email);await p.locator('#authenticator-code').fill(code(secret));await p.locator('#authenticator-submit').click();await p.waitForURL('**/#/account/profile');console.log('PASS dynamic-code login');
 const tokens=[];
 for(let i=0;i<2;i++){
 const redirect='http://localhost:'+callback.address().port+'/totp-callback';sql(`INSERT INTO apps(id,name,secret_hash,redirects,public,login_methods) VALUES('${ids[i]}','Authenticator fixture','${i?mac(clientSecret):''}',ARRAY['${redirect}'],${i===0},ARRAY['totp'])`)
 const verifier=randomBytes(32).toString('base64url'),challenge=createHash('sha256').update(verifier).digest('base64url');await p.route(redirect+'**',r=>r.fulfill({body:'OK',contentType:'text/plain'}));
 await p.goto(origin+'/authorize?'+new URLSearchParams({client_id:ids[i],redirect_uri:redirect,response_type:'code',scope:'openid profile email',code_challenge:challenge,code_challenge_method:'S256',state:suffix}));await p.locator('#continue').click();await p.waitForURL(redirect+'**');const authCode=new URL(p.url()).searchParams.get('code');assert.ok(authCode);
 const d=await(await fetch(origin+'/.well-known/openid-configuration')).json();const body=new URLSearchParams({grant_type:'authorization_code',code:authCode,redirect_uri:redirect,client_id:ids[i],code_verifier:verifier});const headers={'Content-Type':'application/x-www-form-urlencoded'};if(i)headers.Authorization='Basic '+Buffer.from(ids[i]+':'+clientSecret).toString('base64');const res=await fetch(d.token_endpoint,{method:'POST',headers,body});assert.equal(res.status,200);const token=await res.json();tokens.push(token);const claims=JSON.parse(Buffer.from(token.id_token.split('.')[1],'base64url'));assert.ok(claims.amr.includes('totp'));assert.deepEqual(claims.roles,['user']);assert.equal(claims.project_role.field,'user');assert.equal((await fetch(d.userinfo_endpoint,{headers:{Authorization:'Bearer '+token.access_token}})).status,200)
 }
 console.log('PASS public and confidential OIDC token exchange with authenticator AMR');
 sql(`UPDATE app_users SET role_id='admin' WHERE app_id='${ids[0]}' AND user_id=(SELECT id FROM users WHERE email='${email}')`);
 const roleDiscovery=await(await fetch(origin+'/.well-known/openid-configuration')).json();
 for(let i=0;i<2;i++){const info=await(await fetch(roleDiscovery.userinfo_endpoint,{headers:{Authorization:'Bearer '+tokens[i].access_token}})).json();assert.deepEqual(info.roles,[i===0?'admin':'user']);assert.equal(info.project_role.field,i===0?'admin':'user')}
 console.log('PASS ID Token project role claims and live UserInfo role isolation');

 sql(`UPDATE apps SET login_methods=ARRAY['code'] WHERE id='${ids[0]}'`);const d=await(await fetch(origin+'/.well-known/openid-configuration')).json();assert.equal((await fetch(d.userinfo_endpoint,{headers:{Authorization:'Bearer '+tokens[0].access_token}})).status,401);console.log('PASS disabled project method rejects existing access token');
 await p.goto(origin+'/login');await p.locator('#switch').click();await p.locator('#authenticator-open').click();await p.locator('#authenticator-recovery').click();await p.locator('#authenticator-account').fill(email);await p.locator('#authenticator-code').fill(recovery[0]);await p.locator('#authenticator-submit').click();await p.waitForURL('**/#/account/profile');console.log('PASS recovery-code login');
 await p.goto(origin+'/#/account/security?action=authenticator');await p.getByRole('heading',{name:'已绑定身份验证器'}).waitFor();seed('email');await p.getByPlaceholder('6 位邮箱验证码').fill('123456');await p.locator('label.el-checkbox').click();await p.getByPlaceholder('输入一个尚未使用的恢复码').fill(recovery[1]);await p.getByRole('button',{name:'解绑身份验证器',exact:true}).click();await p.getByRole('dialog').getByRole('button',{name:'确定',exact:true}).click();await p.waitForURL(origin+'/login');console.log('PASS email + recovery unbinding and session revocation');
} finally {
 await browser.close();await new Promise(r=>callback.close(r));const apps=ids.map(v=>"'"+v+"'").join(',');sql(`DELETE FROM access_tokens WHERE app_id IN (${apps});DELETE FROM app_users WHERE app_id IN (${apps});DELETE FROM auth_requests WHERE body->'Request'->>'client_id' IN (${apps});DELETE FROM apps WHERE id IN (${apps});DELETE FROM sessions WHERE user_id IN(SELECT id FROM users WHERE email='${email}');DELETE FROM audit WHERE user_id IN(SELECT id FROM users WHERE email='${email}');DELETE FROM challenges WHERE email='${email}';DELETE FROM users WHERE email='${email}';`)
}
