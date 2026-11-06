import assert from 'node:assert/strict'
import { mkdir, mkdtemp, realpath, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { build, preview } from 'vite'
import vue from '@vitejs/plugin-vue'
import { chromium } from '@playwright/test'

const admin = fileURLToPath(new URL('../', import.meta.url))
const cache = path.join(admin, 'node_modules/.cache/banner')
await mkdir(cache, { recursive: true })
const root = await mkdtemp(path.join(cache, 'case-'))
try {
  await writeFile(
    path.join(root, 'index.html'),
    '<meta charset="utf-8"><main id="app"></main><script type="module" src="/entry.js"></script>'
  )
  await writeFile(
    path.join(root, 'entry.js'),
    `
  import { createApp, h } from 'vue'
  import Banner from 'tested-banner'
  const events = []
  window.bannerEvents = events
  createApp(() => h('form', { onSubmit(e) { e.preventDefault(); events.push('submit') } }, [
    h(Banner, {
      title: '默认图片', description: '说明',
      cancelButton: { show: true, text: '取消' },
      onCancel: () => events.push('cancel'), onClick: () => events.push('confirm')
    }),
    h(Banner, {
      image: '/passkey.png', title: '自定义图片', description: '说明',
      button: { show: false }, cancelButton: { show: false }
    })
  ])).mount('#app')
`
  )
  const config = {
    root,
    envDir: root,
    configFile: false,
    publicDir: path.join(admin, 'public'),
    plugins: [vue()],
    resolve: {
      alias: {
        'tested-banner': path.join(admin, 'src/components/core/banners/art-card-banner/index.vue'),
        '@imgs': path.join(admin, 'src/assets/images')
      }
    }
  }
  await build(config)
  const server = await preview({ ...config, preview: { host: '127.0.0.1', port: 0 } })
  let browser
  try {
    browser = await chromium.launch({ channel: 'msedge', headless: true })
    const page = await browser.newPage()
    await page.goto('http://127.0.0.1:' + server.httpServer.address().port)
    for (const [title, source] of [
      ['默认图片', '/logo.png'],
      ['自定义图片', '/passkey.png']
    ]) {
      const img = page.getByRole('img', { name: title })
      await img.waitFor()
      assert.equal(await img.getAttribute('src'), source)
      assert.ok(
        await img.evaluate(async (el) => {
          await el.decode()
          return el.naturalWidth > 0
        })
      )
    }
    assert.equal(await page.getByRole('button').count(), 2)
    await page.keyboard.press('Tab')
    assert.equal(await page.evaluate(() => document.activeElement.textContent.trim()), '取消')
    await page.keyboard.press('Space')
    await page.keyboard.press('Tab')
    assert.equal(await page.evaluate(() => document.activeElement.textContent.trim()), '查看详情')
    await page.keyboard.press('Enter')
    assert.deepEqual(await page.evaluate(() => window.bannerEvents), ['cancel', 'confirm'])
    console.log('PASS default and custom images, hidden actions, keyboard events and form behavior')
  } finally {
    await browser?.close()
    await server.close()
  }
} finally {
  const directory = await realpath(root)
  assert.equal(path.dirname(directory), await realpath(cache))
  assert.match(path.basename(directory), /^case-[A-Za-z0-9]+$/)
  await rm(directory, { recursive: true, force: true })
}
