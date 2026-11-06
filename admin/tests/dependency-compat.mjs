import assert from 'node:assert/strict'
import './lock-proof.mjs'
import './route-registry.mjs'
import './button-auth.mjs'
import './table-races.mjs'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import path from 'node:path'
import { createI18n } from 'vue-i18n'
import * as XLSX from 'xlsx'
import { ESLint } from 'eslint'
import parser from '@typescript-eslint/parser'
import stylelint from 'stylelint'

const require = createRequire(import.meta.url)
const root = path.resolve(import.meta.dirname, '..')
const json = async (file) => JSON.parse(await readFile(path.join(root, file), 'utf8'))
const messages = {
  en: await json('src/locales/langs/en.json'),
  zh: await json('src/locales/langs/zh.json')
}
const i18n = createI18n({ legacy: false, locale: 'zh', fallbackLocale: 'zh', messages })
assert.equal(i18n.global.t('httpMsg.unauthorized'), messages.zh.httpMsg.unauthorized)
i18n.global.locale.value = 'en'
assert.equal(i18n.global.t('httpMsg.unauthorized'), messages.en.httpMsg.unauthorized)
console.log('PASS existing Chinese/English translations with composition API')

const rows = [
  { 名称: '中文资料', 数量: 0, 备注: '保留空字符串' },
  { 名称: 'Other', 数量: 42, 备注: '' }
]
for (const bookType of ['xlsx', 'biff8']) {
  const book = XLSX.utils.book_new()
  const sheet = XLSX.utils.json_to_sheet(rows)
  sheet['!cols'] = [{ wch: 20 }, { wch: 10 }, { wch: 30 }]
  XLSX.utils.book_append_sheet(book, sheet, '数据')
  const bytes = XLSX.write(book, { bookType, type: 'array', compression: true })
  const restored = XLSX.read(bytes, { type: 'array' })
  assert.deepEqual(XLSX.utils.sheet_to_json(restored.Sheets[restored.SheetNames[0]]), rows)
}
console.log('PASS XLSX and legacy XLS round trip with Chinese/zero/empty fields')

// This reads configuration only; it must never invoke git commit or hooks.
const config = require('commitizen/dist/commitizen/configLoader').load(null, path.join(root, 'src'))
assert.equal(config.path, 'node_modules/cz-git')
console.log('PASS actual Commitizen parent-directory config discovery')

const eslint = new ESLint({
  cwd: root,
  overrideConfigFile: true,
  overrideConfig: [
    { files: ['**/*.ts'], languageOptions: { parser }, rules: { 'no-unused-vars': 'error' } }
  ]
})
const [valid] = await eslint.lintText('const value: number = 1; console.log(value)', {
  filePath: 'src/audit-compat.ts'
})
assert.equal(valid.errorCount, 0)
const [invalid] = await eslint.lintText('const unused: number = 1', {
  filePath: 'src/audit-compat.ts'
})
assert.equal(invalid.errorCount, 1)
console.log('PASS ESLint glob matching and TypeScript parser preserve diagnostics')
const css = await stylelint.lint({
  code: '<template><div /></template><style>.compat { color: #111; }</style>',
  codeFilename: path.join(root, 'src/audit-compat.vue'),
  config: { extends: ['stylelint-config-standard', 'stylelint-config-recommended-vue'] }
})
assert.equal(css.errored, false)
console.log('PASS Stylelint Vue recommended configuration compatibility')
