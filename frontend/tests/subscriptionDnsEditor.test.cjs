const test = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const vue = require('vue')
const { parse, compileScript } = require('@vue/compiler-sfc')

const root = path.resolve(__dirname, '../src')
const file = path.join(root, 'components/settings/SubscriptionDns.vue')
const source = fs.readFileSync(file, 'utf8')
const descriptor = parse(source).descriptor
const script = compileScript(descriptor, { id: 'dns-test' }).content

function load(code, filename) {
  const module = { exports: {} }

  function req(id) {
    if (id === 'vue-i18n')
      return { useI18n: () => ({ t: k => k }) }

    if (id.endsWith('.vue')) return {}

    if (id.startsWith('@/') || id.startsWith('.')) {
      const target = id.startsWith('@/')
        ? path.join(root, id.slice(2))
        : path.resolve(path.dirname(filename), id)

      return load(fs.readFileSync(target + '.ts', 'utf8'), target + '.ts')
    }

    return require(id)
  }

  const js = ts.transpileModule(code, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      esModuleInterop: true
    }
  }).outputText

  new Function('require', 'module', 'exports', js)(
    req, module, module.exports
  )

  return module.exports
}

const component = load(script, file).default
const plain = v => JSON.parse(JSON.stringify(v))

function harness(t) {
  const props = vue.reactive({
    catalog: [],
    config: {
      dns: {
        final: 'cloudflare',
        servers: [
          { tag: 'local', type: 'local' },
          {
            tag: 'cloudflare',
            type: 'https',
            server: '1.1.1.1',
            tls: { server_name: 'cloudflare-dns.com' }
          }
        ],
        rules: [
          { action: 'evaluate', tag: 'cf', server: 'cloudflare' },
          { action: 'respond', match_response: 'cf' },
          { action: 'route', server: 'local' }
        ]
      },
      default_domain_resolver: 'local',
      rules: [{ action: 'sniff' }],
      log: { level: 'warn' }
    }
  })

  const scope = vue.effectScope()
  t.after(() => scope.stop())

  const state = scope.run(() => component.setup(props, {
    expose() {},
    emit(event, value) {
      assert.equal(event, 'update:config')
      props.config = value
    }
  }))

  return { props, state }
}

test('Server pencil edits only selected DNS server', t => {
  const { props, state: s } = harness(t)
  const expected = plain(props.config)

  s.openItem('server', 1)
  s.patchText('server', '1.0.0.1')
  s.saveDraft()

  expected.dns.servers[1].server = '1.0.0.1'
  assert.deepEqual(plain(props.config), expected)
})

test('Rule pencil edits only selected DNS rule', t => {
  const { props, state: s } = harness(t)
  const expected = plain(props.config)

  s.openItem('rule', 2)
  s.patchText('server', 'cloudflare')
  s.saveDraft()

  expected.dns.rules[2].server = 'cloudflare'
  assert.deepEqual(plain(props.config), expected)
})

test('Invalid JSON cannot modify template', t => {
  const { props, state: s } = harness(t)
  const expected = plain(props.config)

  s.openItem('rule', 1)
  s.draft.value.text = '{invalid'
  s.saveDraft()

  assert.deepEqual(plain(props.config), expected)
  assert.ok(s.error.value)
})

test('Cancelling edit leaves template unchanged', t => {
  const { props, state: s } = harness(t)
  const expected = plain(props.config)

  s.openItem('server', 1)
  s.patchText('server', '8.8.8.8')
  s.draft.value = null

  assert.deepEqual(plain(props.config), expected)
})

test('Referenced server cannot be deleted', t => {
  const { props, state: s } = harness(t)
  const expected = plain(props.config)

  s.askRemove('server', 1)
  s.remove()

  assert.deepEqual(plain(props.config), expected)
  assert.ok(s.error.value)
})

test('Evaluate and respond order is protected', t => {
  const { props, state: s } = harness(t)
  const expected = plain(props.config)

  s.move(1, -1)

  assert.deepEqual(plain(props.config), expected)
  assert.ok(s.error.value)
})

test('DNS controls use the same main JSON template', () => {
  const settings = fs.readFileSync(
    path.join(root, 'views/Settings.vue'), 'utf8'
  )

  assert.ok(source.includes("openItem('server', i)"))
  assert.ok(source.includes("openItem('rule', i)"))
  assert.ok(settings.includes('v-model:config="subJsonExt"'))
  assert.ok(settings.includes(':content="settings.subJsonExt"'))
  assert.ok(!source.includes("openItem('advanced'"))
})
