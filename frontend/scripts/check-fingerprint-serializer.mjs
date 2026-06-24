import assert from 'node:assert/strict'
import { Buffer } from 'node:buffer'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { build } from 'esbuild'

const repoRoot = new URL('../../', import.meta.url)
const frontendRoot = new URL('../', import.meta.url)

const result = await build({
  entryPoints: [fileURLToPath(new URL('src/modules/browser/utils/fingerprintSerializer.ts', frontendRoot))],
  bundle: true,
  format: 'esm',
  platform: 'node',
  write: false,
  logLevel: 'silent',
})

const code = result.outputFiles[0]?.text
if (!code) {
  throw new Error('serializer bundle is empty')
}

const moduleUrl = `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const {
  FINGERPRINT_PRESETS,
  defaultAcceptLanguage,
  deserialize,
  serialize,
} = await import(moduleUrl)

const [webglData, fontData, localeData] = await Promise.all([
  readJson(new URL('backend/internal/fingerprint/data/webgl.json', repoRoot)),
  readJson(new URL('backend/internal/fingerprint/data/fonts.json', repoRoot)),
  readJson(new URL('backend/internal/fingerprint/data/locales.json', repoRoot)),
])

async function readJson(path) {
  return JSON.parse(await readFile(path, 'utf8'))
}

function countKey(args, key) {
  return args.filter(arg => arg === key || arg.startsWith(`${key}=`)).length
}

assert.equal(defaultAcceptLanguage('en_US'), 'en-US,en;q=0.9')
assert.equal(defaultAcceptLanguage('zh-CN'), 'zh-CN,zh;q=0.9')
assert.equal(defaultAcceptLanguage(''), '')

const parsed = deserialize([
  '--lang',
  'en-US',
  '--accept-language',
  'en-US,en;q=0.9',
  '--fingerprint-webgl-vendor=Intel',
  '--unknown-flag',
  '--unknown-pair',
  'kept',
])
assert.equal(parsed.lang, 'en-US')
assert.equal(parsed.acceptLanguage, 'en-US,en;q=0.9')
assert.deepEqual(parsed.unknownArgs, ['--unknown-flag', '--unknown-pair', 'kept'])

const serialized = serialize({
  lang: 'zh-CN',
  fingerprintLocale: 'zh-CN',
  acceptLanguage: 'zh-CN,zh;q=0.9',
  webglVendor: 'NVIDIA',
  unknownArgs: [
    '--lang',
    'en-US',
    '--fingerprint-accept-language=en-US,en;q=0.9',
    '--custom-flag',
    '--custom-pair',
    'value',
  ],
})
assert.equal(countKey(serialized, '--lang'), 1)
assert.equal(countKey(serialized, '--fingerprint-accept-language'), 1)
assert(serialized.includes('--custom-flag'))
assert(serialized.includes('--custom-pair'))
assert(serialized.includes('value'))
assert(!serialized.includes('--fingerprint-accept-language=en-US,en;q=0.9'))

for (const preset of FINGERPRINT_PRESETS) {
  const config = preset.config
  const args = serialize(preset.config)
  assert.equal(countKey(args, '--fingerprint-accept-language'), 1, `${preset.id} accept-language`)
  assert(args.some(arg => arg.startsWith('--fingerprint-fonts=')), `${preset.id} fonts`)
  assert(args.some(arg => arg.startsWith('--fingerprint-webgl-vendor=')), `${preset.id} webgl vendor`)
  assert(args.some(arg => arg.startsWith('--fingerprint-webgl-renderer=')), `${preset.id} webgl renderer`)

  const platform = config.platform
  assert(platform in webglData, `${preset.id} known platform`)
  assert(
    webglData[platform].some(item => item.vendor === config.webglVendor && item.renderer === config.webglRenderer),
    `${preset.id} WebGL tuple is not in backend data`,
  )

  const fontPool = fontData[platform]
  assert(fontPool, `${preset.id} font platform`)
  const fontSet = new Set((config.fonts || '').split(',').map(font => font.trim()).filter(Boolean))
  for (const markerFont of fontPool.markerFonts) {
    assert(fontSet.has(markerFont), `${preset.id} missing backend marker font ${markerFont}`)
  }

  assert(
    localeData.some(region => region.locales.includes(config.lang) && region.timezones.includes(config.timezone)),
    `${preset.id} locale/timezone pair is not in backend data`,
  )
}

console.log(`fingerprint serializer checks passed (${FINGERPRINT_PRESETS.length} presets)`)
