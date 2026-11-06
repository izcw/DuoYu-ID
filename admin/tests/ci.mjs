import fs from 'node:fs'
import { randomBytes } from 'node:crypto'
import { spawn, execFileSync } from 'node:child_process'
import { once } from 'node:events'
import { createServer } from 'node:net'
import path from 'node:path'
import { tmpdir } from 'node:os'
import { chromium } from '@playwright/test'
import assert from 'node:assert/strict'

assert.equal(process.env.CI, 'true')
const database = new URL(process.env.DUOYU_TEST_DATABASE_URL)
assert.ok(['postgres:', 'postgresql:'].includes(database.protocol))
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(database.hostname))
assert.equal(database.pathname, '/duoyu_ci', 'CI requires its own disposable duoyu_ci database')
assert.ok(
  [...database.searchParams].every(([key, value]) => key === 'sslmode' && value === 'disable')
)
const portProbe = createServer()
portProbe.listen(0, '127.0.0.1')
await once(portProbe, 'listening')
const port = portProbe.address().port
await new Promise((resolve, reject) =>
  portProbe.close((error) => (error ? reject(error) : resolve()))
)
const config = {
  DATABASE_URL: database.href,
  DUOYU_ORIGIN: 'http://localhost:' + port,
  DUOYU_ADDR: '127.0.0.1:' + port,
  DUOYU_DEV: 'true',
  DUOYU_ROLE: 'api',
  DUOYU_SECRET: randomBytes(32).toString('hex'),
  DUOYU_ADMIN_EMAIL: 'admin@example.invalid',
  DUOYU_ADMIN_PASSWORD: randomBytes(32).toString('hex')
}
fs.mkdirSync('../test-results', { recursive: true })
// Build in a private per-run directory outside uploaded reports. No developer
// configuration file is created or loaded by CI.
const tmp = fs.mkdtempSync(path.join(process.env.RUNNER_TEMP || tmpdir(), 'duoyu-ci-'))
const binary = path.join(tmp, process.platform === 'win32' ? 'server.exe' : 'server')
const baseEnv = Object.fromEntries(
  Object.entries(process.env).filter(([key]) => !/^(DUOYU_|SMTP_|PG|DATABASE_URL$)/i.test(key))
)
let server, stopped, log
try {
  execFileSync(process.env.GO_BINARY || 'go', ['build', '-o', binary, './server'], {
    cwd: '..',
    stdio: 'inherit'
  })
  log = fs.openSync('../test-results/ci-server.log', 'w')
  server = spawn(binary, [], {
    cwd: '..',
    windowsHide: true,
    env: { ...baseEnv, ...config, DUOYU_DATA_DIR: path.join(tmp, 'data') },
    stdio: ['ignore', log, log]
  })
  stopped = once(server, 'close')
  let ready = false
  for (let i = 0; i < 60 && server.exitCode === null; i++) {
    try {
      ready = (
        await fetch(config.DUOYU_ORIGIN + '/api/auth/session', {
          signal: AbortSignal.timeout(2000)
        })
      ).ok
    } catch {
      /* startup may still be in progress */
    }
    if (ready) break
    await new Promise((resolve) => setTimeout(resolve, 500))
  }
  assert.ok(ready, 'CI server did not start')
  for (const file of ['session', 'ui-refresh', 'smoke', 'passkey-context']) {
    execFileSync(process.execPath, ['tests/' + file + '.mjs'], {
      env: {
        ...baseEnv,
        DUOYU_CI_FIXTURE: '1',
        DUOYU_DEV: 'true',
        DUOYU_ORIGIN: config.DUOYU_ORIGIN,
        DUOYU_ADMIN_EMAIL: config.DUOYU_ADMIN_EMAIL,
        DUOYU_ADMIN_PASSWORD: config.DUOYU_ADMIN_PASSWORD,
        CHROME_PATH: process.env.CHROME_PATH || chromium.executablePath(),
        SKIP_MAIL: 'true',
        TEST_PASSKEY: 'false'
      },
      stdio: 'inherit',
      timeout: 240000
    })
  }
} finally {
  if (server) {
    server.kill()
    await stopped
  }
  if (log !== undefined) fs.closeSync(log)
  fs.rmSync(tmp, { recursive: true, force: true })
}
