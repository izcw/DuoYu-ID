import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { createSSRApp, h } from 'vue'
import { renderToString } from 'vue/server-renderer'

const root = fileURLToPath(new URL('../', import.meta.url))
const vite = await createServer({
  root,
  configFile: false,
  plugins: [
    {
      name: 'unused-columns',
      enforce: 'pre',
      resolveId(id) {
        if (id === './useTableColumns') return '/__columns.ts'
      },
      load(id) {
        if (id === '/__columns.ts') return 'export const useTableColumns=()=>{throw Error()}'
      }
    }
  ],
  resolve: { alias: { '@': root + 'src' } },
  optimizeDeps: { noDiscovery: true },
  server: { middlewareMode: true, watch: null, ws: false }
})
try {
  const { useTable } = await vite.ssrLoadModule('/src/hooks/core/useTable.ts')
  const requests = []
  let state
  await renderToString(
    createSSRApp({
      setup() {
        state = useTable({
          core: {
            apiFn: () => new Promise((resolve, reject) => requests.push({ resolve, reject })),
            immediate: false
          },
          performance: { enableCache: false }
        })
        return () => h('div')
      }
    })
  )
  const first = state.fetchData(),
    second = state.fetchData()
  requests[1].resolve({ records: [{ id: 2 }], current: 1, size: 10, total: 1 })
  await second
  assert.equal(state.data.value[0].id, 2)
  requests[0].reject(new Error('late network failure'))
  await first
  assert.deepEqual(
    state.data.value.map((r) => r.id),
    [2]
  )
  assert.equal(state.error.value, null)
  const third = state.fetchData(),
    fourth = state.fetchData()
  requests[2].resolve({ records: [], total: 0 })
  await third
  assert.equal(state.loading.value, true)
  requests[3].resolve({ records: [], total: 0 })
  await fourth
  console.log('PASS late success and failure leave the active list, loading state and error intact')
  const { TableCache } = await vite.ssrLoadModule('/src/utils/table/tableCache.ts')
  const { defaultResponseAdapter, createSmartDebounce } = await vite.ssrLoadModule(
    '/src/utils/table/tableUtils.ts'
  )
  const failures = []
  async function check(name, run) {
    try {
      await run()
      console.log('PASS ' + name)
    } catch (error) {
      failures.push(error)
      console.error('FAIL ' + name, error.message)
    }
  }
  await check('cache enforces its bound at insertion and keeps replacements', () => {
    const cache = new TableCache(60000, 1)
    cache.set({ current: 1 }, [1], {})
    cache.set({ current: 2 }, [2], {})
    assert.equal(cache.getStats().total, 1)
    cache.set({ current: 2 }, [3], {})
    assert.deepEqual(cache.get({ current: 2 }).data, [3])
    const disabled = new TableCache(60000, 0)
    disabled.set({}, [1], {})
    assert.equal(disabled.getStats().total, 0)
  })
  await check('null nested data is an empty page', () => {
    assert.deepEqual(defaultResponseAdapter({ data: null, total: 9 }), { records: [], total: 9 })
  })
  await check('every coalesced, flushed or cancelled caller settles', async () => {
    const called = []
    const f = createSmartDebounce(async (x) => {
      called.push(x)
      return x
    }, 10000)
    const a = f(1),
      b = f(2)
    let timeout
    try {
      const bounded = (p) =>
        Promise.race([
          p,
          new Promise((_, reject) => {
            timeout = setTimeout(() => reject(Error('caller did not settle')), 150)
          })
        ]).finally(() => clearTimeout(timeout))
      assert.deepEqual(await bounded(Promise.all([a, b, f.flush()])), [2, 2, 2])
      const c = f(3)
      f.cancel()
      assert.equal(await bounded(c), undefined)
      assert.deepEqual(called, [2])
    } finally {
      f.cancel()
    }
  })
  if (failures.length) throw new AggregateError(failures, 'table utility regressions')
} finally {
  await vite.close()
}
