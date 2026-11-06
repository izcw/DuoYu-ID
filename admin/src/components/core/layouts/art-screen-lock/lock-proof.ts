import CryptoJS from 'crypto-js'

const iterations = 210000
const prefix = 'pbkdf2-sha256-v1'

async function derive(password: string, salt: string) {
  if (globalThis.crypto?.subtle) {
    const bytes = new Uint8Array(salt.match(/../g)!.map((pair) => parseInt(pair, 16)))
    const key = await crypto.subtle.importKey(
      'raw',
      new TextEncoder().encode(password),
      'PBKDF2',
      false,
      ['deriveBits']
    )
    const digest = await crypto.subtle.deriveBits(
      { name: 'PBKDF2', hash: 'SHA-256', salt: bytes, iterations },
      key,
      256
    )
    return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
  }
  // Preserve local HTTP development support where SubtleCrypto is unavailable.
  return CryptoJS.PBKDF2(password, CryptoJS.enc.Hex.parse(salt), {
    hasher: CryptoJS.algo.SHA256,
    keySize: 8,
    iterations
  }).toString()
}

export async function createLockProof(password: string) {
  const salt = CryptoJS.lib.WordArray.random(16).toString()
  return [prefix, salt, await derive(password, salt)].join(':')
}

export async function matchesLockProof(password: string, proof: string, legacyKey: string) {
  if (!password || !proof) return false
  if (proof.startsWith(prefix + ':')) {
    const [, salt, digest, ...rest] = proof.split(':')
    if (rest.length || !/^[0-9a-f]{32}$/.test(salt) || !/^[0-9a-f]{64}$/.test(digest)) return false
    return (await derive(password, salt)) === digest
  }
  // Old installations stored CryptoJS's salted AES format. A successful unlock
  // discards it; new locks always use a randomly salted, one-way verifier.
  if (!proof.startsWith('U2FsdGVkX1') || !legacyKey) return false
  try {
    return CryptoJS.AES.decrypt(proof, legacyKey).toString(CryptoJS.enc.Utf8) === password
  } catch {
    return false
  }
}
