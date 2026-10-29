import {acceptLegal} from './consent.mjs'
// Local-only QR integration test. Seeds disposable OTP records; never sends SMTP.
import { chromium } from '@playwright/test'
import { createHmac, randomBytes } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import assert from 'node:assert/strict'
import jsQR from 'jsqr'
import { PNG } from 'pngjs'
const env=Object.fromEntries(fs.readFileSync('../.env.local','utf8').split(/\r?\n/).filter(x=>x.includes('=')).map(x=>[x.slice(0,x.indexOf('=')),x.slice(x.indexOf('=')+1)]))
assert.equal(env.DUOYU_DEV,'true')
assert.equal(env.DUOYU_ORIGIN,'http://localhost:5252')
const email='qr-test-'+randomBytes(8).toString('hex')+'@example.invalid'
const sql=q=>execFileSync('docker',['exec','duoyu-postgres-dev','psql','-U','duoyu','-d','duoyu','-v','ON_ERROR_STOP=1','-q','-c',q],{stdio:['ignore','pipe','pipe']})
function seed(){const digest=createHmac('sha256',Buffer.from(env.DUOYU_SECRET,'hex')).update(email+'|login|123456').digest('hex');sql(`INSERT INTO challenges(email,purpose,digest,expires_at) VALUES('${email}','login','${digest}',now()+interval '5 minutes') ON CONFLICT(email,purpose) DO UPDATE SET digest=EXCLUDED.digest,attempts=0,expires_at=EXCLUDED.expires_at;`)}
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||'C:/Program Files/Google/Chrome/Application/chrome.exe',headless:true})
const desktop=await browser.newContext({viewport:{width:1440,height:1000}})
const phone=await browser.newContext({viewport:{width:390,height:844}})
const page=await desktop.newPage(),mobile=await phone.newPage()
const errors=[];for(const p of [page,mobile])p.on('pageerror',e=>errors.push(e.message))
let qrID=''
try{
 seed();await mobile.goto(env.DUOYU_ORIGIN+'/login');await mobile.locator('#email').fill(email);await mobile.locator('#code').fill('123456');await acceptLegal(mobile);await mobile.locator('#submit').click();await mobile.waitForURL('**/#/account/profile');await mobile.getByRole('button',{name:'添加手机',exact:true}).waitFor()
 const cdp=await phone.newCDPSession(mobile);await cdp.send('WebAuthn.enable');await cdp.send('WebAuthn.addVirtualAuthenticator',{options:{protocol:'ctap2',transport:'internal',hasResidentKey:true,hasUserVerification:true,isUserVerified:true,automaticPresenceSimulation:true}})
 seed();await mobile.getByRole('button',{name:'添加手机',exact:true}).click();await mobile.getByPlaceholder('6 位验证码',{exact:true}).click();await mobile.getByPlaceholder('6 位验证码',{exact:true}).fill('123456');const registered=mobile.waitForResponse(r=>r.url().endsWith('/api/me/passkeys/finish'));await mobile.getByRole('button',{name:'继续',exact:true}).click();assert.equal((await registered).status(),200);await mobile.getByRole('dialog').waitFor({state:'hidden'})
 await mobile.waitForURL('**/#/account/profile');
 const credentials=await mobile.evaluate(()=>fetch('/api/me/passkeys').then(r=>r.json()));
 assert.equal(credentials.data.length,1);const credential=credentials.data[0];
 assert.equal(credential.qr_enabled,true);assert.equal(credential.device_type,'设备内置验证器');assert.ok(credential.methods.includes('设备内验证'));
 assert.notEqual(credential.registration_device,'历史记录未保存');assert.equal('credential' in credential,false);
 await mobile.getByText('创建环境：',{exact:false}).waitFor();
 await mobile.waitForFunction(()=>![...document.querySelectorAll('.el-loading-mask')].some(e=>e.getClientRects().length));
 await mobile.waitForFunction(()=>{let e=document.querySelector('.dy-page');while(e){if(Number(getComputedStyle(e).opacity)<1)return false;e=e.parentElement}return true});
 await mobile.locator('.dy-panel').filter({has:mobile.getByRole('heading',{name:'我的通行密钥',exact:true})}).screenshot({path:'../test-results/passkey-device-details.png',animations:'disabled'});
 await page.goto(env.DUOYU_ORIGIN+'/login');const created=page.waitForResponse(r=>r.url().endsWith('/api/auth/qr'));await acceptLegal(page);await page.locator('#qr-open').click();const out=(await (await created).json()).data;qrID=out.id;const png=PNG.sync.read(Buffer.from(out.image.split(',')[1],'base64'));const decoded=jsQR(new Uint8ClampedArray(png.data),png.width,png.height);assert.ok(decoded);assert.ok(decoded.data.startsWith(env.DUOYU_ORIGIN+'/scan#'))
 await page.locator('#qr-image').waitFor({state:'visible'});await page.screenshot({path:'../test-results/qr-desktop.png',fullPage:true})
 await mobile.goto(decoded.data);await mobile.locator('#verify').waitFor();await mobile.waitForFunction(()=>!document.getElementById('verify').disabled);assert.equal(await mobile.locator('#label').textContent(),out.label);assert.equal(await mobile.locator('#approve').isVisible(),false)
 const verified=mobile.waitForResponse(r=>r.url().endsWith('/api/auth/passkey/finish'));await acceptLegal(mobile);await mobile.locator('#verify').click();assert.equal((await verified).status(),200);await mobile.locator('#approve').waitFor({state:'visible'});assert.equal(await mobile.locator('#account').textContent(),email);await mobile.screenshot({path:'../test-results/qr-phone.png',fullPage:true})
 await mobile.locator('#approve').click();await page.waitForURL('**/#/account/profile');const me=await page.evaluate(()=>fetch('/api/me').then(r=>r.json()));assert.equal(me.data.email,email)
 const replay=await page.evaluate(async id=>fetch('/api/auth/qr/'+id+'/poll',{method:'POST',headers:{'X-Duoyu-Request':'1','X-Duoyu-Legal-Version':'2026-10-29','X-Duoyu-Adult':'true'}}).then(r=>r.json()),qrID);assert.equal(replay.data.status,'expired');assert.deepEqual(errors,[])
 console.log('PASS: QR image decoded, separate browser contexts, phone passkey verification, explicit account confirmation, desktop session, one-time grant. No SMTP sent.')
}catch(e){
 await mobile.screenshot({path:'../test-results/qr-failure.png',fullPage:true})
 console.log('Mobile URL:',mobile.url(),'Page errors:',errors)
 throw e
}finally{
 await browser.close()
 sql(`DELETE FROM qr_logins WHERE user_id IN(SELECT id FROM users WHERE email='${email}')${qrID?` OR id='${qrID}'`:''}; DELETE FROM passkeys WHERE user_id IN(SELECT id FROM users WHERE email='${email}'); DELETE FROM sessions WHERE user_id IN(SELECT id FROM users WHERE email='${email}'); DELETE FROM webauthn_challenges WHERE user_id IN(SELECT id FROM users WHERE email='${email}'); DELETE FROM audit WHERE user_id IN(SELECT id FROM users WHERE email='${email}'); DELETE FROM challenges WHERE email='${email}'; DELETE FROM users WHERE email='${email}';`)
}
