import fs from 'node:fs'
import { randomBytes } from 'node:crypto'
import { spawn, execFileSync } from 'node:child_process'
import { chromium } from '@playwright/test'
import assert from 'node:assert/strict'

// Refuse to overwrite local configuration. Run only in an isolated CI checkout.
assert.equal(process.env.CI, 'true')
assert.equal(fs.existsSync('../.env.local'), false)
assert.ok(process.env.DUOYU_TEST_DATABASE_URL)
const config = {
  DATABASE_URL: process.env.DUOYU_TEST_DATABASE_URL,
  DUOYU_ORIGIN: 'http://localhost:5252', DUOYU_ADDR: '127.0.0.1:5252',
  DUOYU_DEV: 'true', DUOYU_ROLE: 'api', DUOYU_DATA_DIR: '.ci-data',
  DUOYU_SECRET: randomBytes(32).toString('hex'),
  DUOYU_ADMIN_EMAIL: 'admin@example.invalid', DUOYU_ADMIN_PASSWORD: randomBytes(32).toString('hex'),
  SMTP_PASSWORD: '', SMTP_PLAIN_LOCAL: 'false'
}
fs.mkdirSync('../test-results', { recursive: true })
fs.writeFileSync('../.env.local', Object.entries(config).map(([k,v])=>k+'='+v).join('\n'), { flag:'wx' })
const binary = process.platform === 'win32' ? 'test-results/ci-server.exe' : 'test-results/ci-server'
execFileSync('go', ['build','-o',binary,'./server'], {cwd:'..',stdio:'inherit'})
const log = fs.openSync('../test-results/ci-server.log','w')
const server = spawn('./'+binary,[],{cwd:'..',env:{...process.env,...config},stdio:['ignore',log,log]})
try {
  let ready=false
  for(let i=0;i<60;i++) {
    try { ready=(await fetch(config.DUOYU_ORIGIN+'/api/auth/session')).ok } catch {}
    if(ready)break
    await new Promise(resolve=>setTimeout(resolve,1000))
  }
  assert.ok(ready,'CI server startup timeout')
  for(const file of ['session','ui-refresh','smoke']) execFileSync(process.execPath,['tests/'+file+'.mjs'],{
    env:{...process.env,CHROME_PATH:chromium.executablePath(),SKIP_MAIL:'true',TEST_PASSKEY:'false'},stdio:'inherit'
  })
} finally {
  server.kill()
  fs.closeSync(log)
}
