import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import { createRouter, createMemoryHistory } from 'vue-router'

const root = fileURLToPath(new URL('../', import.meta.url))
const vite = await createServer({
  configFile: false,
  optimizeDeps: { noDiscovery: true, include: [] },
  root,
  plugins: [vue()],
  resolve: { alias: { '@': fileURLToPath(new URL('../src/', import.meta.url)) } },
  server: { middlewareMode: true, watch: null, ws: false },
  appType: 'custom'
})
try {
  const { RouteRegistry } = await vite.ssrLoadModule('/src/router/core/RouteRegistry.ts')
  const router = createRouter({ history: createMemoryHistory(), routes: [] })
  const registry = new RouteRegistry(router)
  const menu = ['one', 'two'].map((id) => ({
    name: id,
    path: '/' + id,
    component: '/index/index',
    meta: {},
    children: [{ name: id + 'Detail', path: 'detail', component: '/duoyu/profile', meta: {} }]
  }))
  const add = router.addRoute.bind(router)
  router.addRoute = (route) => {
    if (route.name === 'two') throw Error('simulated second route failure')
    return add(route)
  }
  assert.throws(() => registry.register(menu), /simulated/)
  assert.equal(registry.isRegistered(), false)
  assert.equal(router.hasRoute('one'), false)
  assert.equal(router.hasRoute('oneDetail'), false)
  router.addRoute = add
  registry.register(menu)
  assert.ok(
    menu.every((group) => router.hasRoute(group.name) && router.hasRoute(group.name + 'Detail'))
  )
  registry.unregister()
  assert.equal(router.getRoutes().length, 0)
  console.log('PASS partial dynamic route installation rolls back and retries in Vue Router')
} finally {
  await vite.close()
}
