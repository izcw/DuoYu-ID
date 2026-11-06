import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import ts from 'typescript'
import CryptoJS from 'crypto-js'

const require = createRequire(import.meta.url)
const source = await readFile(
  new URL('../src/components/core/layouts/art-screen-lock/lock-proof.ts', import.meta.url),
  'utf8'
)
const js = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2022 }
}).outputText
const library = pathToFileURL(require.resolve('crypto-js')).href
const { createLockProof, matchesLockProof } = await import(
  'data:text/javascript;base64,' +
    Buffer.from(js.replace(/(['"])crypto-js\1/, JSON.stringify(library))).toString('base64')
)
const password = '测试锁屏-012',
  key = 'isolated-fixture-key'
const old = CryptoJS.AES.encrypt(password, key).toString()
assert.equal(CryptoJS.AES.decrypt(old, key).toString(CryptoJS.enc.Utf8), password)
assert.equal(await matchesLockProof(password, old, key), true)
assert.equal(await matchesLockProof('wrong', old, key), false)
const proof = await createLockProof(password)
assert.match(proof, /^pbkdf2-sha256-v1:[0-9a-f]{32}:[0-9a-f]{64}$/)
assert.notEqual(proof, await createLockProof(password))
assert.ok(await matchesLockProof(password, proof, ''))
assert.equal(await matchesLockProof('wrong', proof, ''), false)
assert.equal(await matchesLockProof(password, proof + ':extra', ''), false)
const nativeCrypto = Object.getOwnPropertyDescriptor(globalThis, 'crypto')
let fallbackProof
try {
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: undefined })
  assert.ok(await matchesLockProof(password, proof, ''))
  fallbackProof = await createLockProof(password)
} finally {
  if (nativeCrypto) Object.defineProperty(globalThis, 'crypto', nativeCrypto)
  else delete globalThis.crypto
}
assert.ok(await matchesLockProof(password, fallbackProof, ''))
console.log('PASS salted lock verifier, wrong password, malformed data and legacy AES unlock')
