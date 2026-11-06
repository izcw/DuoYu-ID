import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createPinia, defineStore, setActivePinia } from 'pinia'
import { computed, createSSRApp, reactive, ref } from 'vue'
import { routeLocationKey } from 'vue-router'
import { createServer } from 'vite'

// Load the production composable with isolated role data in both supported modes.
const storeId = '/__test-user.ts'
const modeId = '/__test-mode.ts'
const vite = await createServer({
  root: fileURLToPath(new URL('../', import.meta.url)),
  configFile: false,
  optimizeDeps: { noDiscovery: true },
  server: { middlewareMode: true, watch: null, ws: false },
  plugins: [
    {
      name: 'test-auth-context',
      resolveId(id) {
        if (id === '@/store/modules/user') return storeId
        if (id === '@/hooks/core/useAppMode') return modeId
      },
      load(id) {
        if (id === storeId) return 'export const useUserStore = () => globalThis.fixtureUserStore()'
        if (id === modeId)
          return 'export const useAppMode = () => ({isFrontendMode: globalThis.fixtureFrontend})'
      }
    }
  ]
})
try {
  const user = defineStore('auth-fixture', { state: () => ({ info: { buttons: ['add'] } }) })
  const mode = ref(true)
  globalThis.fixtureUserStore = user
  globalThis.fixtureFrontend = mode
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = createSSRApp({})
  app.use(pinia)
  const route = reactive({ meta: { authList: [{ authMark: 'add' }] } })
  app.provide(routeLocationKey, route)
  const { useAuth } = await vite.ssrLoadModule('/src/hooks/core/useAuth.ts')
  const { hasAuth } = app.runWithContext(useAuth)
  const allowed = computed(() => hasAuth('add'))
  assert.equal(allowed.value, true)
  user().info = { buttons: [] }
  assert.equal(allowed.value, false, 'role change should revoke button immediately')
  mode.value = false
  assert.equal(allowed.value, true)
  route.meta = { authList: [] }
  assert.equal(allowed.value, false, 'route change should drop old permissions')
  route.meta = { authList: [{ authMark: 'add' }] }
  assert.equal(allowed.value, true)
  console.log('PASS reactive role revocation, route changes and regrant in both button modes')
} finally {
  delete globalThis.fixtureUserStore
  delete globalThis.fixtureFrontend
  await vite.close()
}
