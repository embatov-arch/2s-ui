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
    catalog: [
      { tag: 'geosite-cn', type: 'remote', url: 'https://example.org/cn.srs' },
      { tag: 'geosite-private', type: 'remote', url: 'https://example.org/private.srs' }
    ],
    catalogOptions: [
      { title: 'Site-Private', value: 'geosite-private' },
      { title: '🇨🇳 Site-China', value: 'geosite-cn' }
    ],
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

test('stock option titles and order, custom tags and definitions preserved', t => {
  const { props, state: s } = harness(t)
  props.config.rule_set = [
    { tag: 'geosite-cn', type: 'remote', url: 'https://example.org/custom-cn.srs' },
    { tag: 'custom', type: 'inline', rules: [{ domain: ['example.org'] }] }
  ]
  assert.deepEqual(plain(s.ruleOptions.value), [
    { title: 'Site-Private', value: 'geosite-private' },
    { title: '🇨🇳 Site-China', value: 'geosite-cn' },
    { title: 'custom', value: 'custom' }
  ])
  s.openItem('rule', 2)
  s.patchSets(['geosite-cn', 'custom'])
  s.saveDraft()
  assert.deepEqual(plain(props.config.dns.rules[2].rule_set), ['geosite-cn', 'custom'])
  assert.equal(props.config.rule_set[0].url, 'https://example.org/custom-cn.srs')
  assert.equal(props.config.rule_set.length, 2)
})
test('server type follows the JSON draft, not a second state', t => {
  const { state: s } = harness(t)
  s.openNewServer()
  s.setDraftText(JSON.stringify({ type: 'https', tag: 'new', server: '1.0.0.1', path: '/dns-query' }))
  s.newServerType('https')
  assert.equal(s.parsed.value.path, '/dns-query')
  s.newServerType('udp')
  assert.equal(s.parsed.value.type, 'udp')
  assert.equal(s.parsed.value.tag, 'new')
})
test('successful move uses shared implementation and preserves unrelated JSON', t => {
  const { props, state: s } = harness(t)
  const expected = plain(props.config)
  s.move(2, -1)
  const moved = expected.dns.rules.pop()
  expected.dns.rules.splice(1, 0, moved)
  assert.deepEqual(plain(props.config), expected)
})
test('scope uses existing editor and confirmation, no parallel catalog', () => {
  const settings = fs.readFileSync(path.join(root, 'views/Settings.vue'), 'utf8')
  assert.ok(settings.includes(':catalog-options="geoList"'))
  assert.ok(source.includes('<EditorModal'))
  assert.ok(source.includes('<DeleteConfirm'))
  assert.ok(!source.includes('<textarea'))
  assert.ok(source.includes('keep-open-on-save'))
})
test('shared editor preserves old close behavior and supports validated saving', t => {
  const file = path.join(root, 'layouts/drawers/EditorModal.vue')
  const code = compileScript(parse(fs.readFileSync(file, 'utf8')).descriptor, { id: 'editor-test' }).content
  const editor = load(code, file).default
  const scope = vue.effectScope()
  t.after(() => scope.stop())
  const events = []
  const props = vue.reactive({ open: true, content: '{}' })
  const state = scope.run(() => editor.setup(props, { expose() {}, emit: (...args) => events.push(args) }))
  state.onInput('{"a":1}')
  state.save()
  assert.deepEqual(events, [['save', '{"a":1}'], ['close']])
  events.length = 0
  props.keepOpenOnSave = true
  state.save()
  assert.deepEqual(events, [['save', '{"a":1}']])
  events.length = 0
  props.controlled = true
  props.content = '{"b":2}'
  state.onInput('{"b":3}')
  assert.deepEqual(events, [['update:content', '{"b":3}']])
  props.content = '{"b":3}'
  events.length = 0
  state.save()
  assert.deepEqual(events, [['save', '{"b":3}']])
})


test('Repeated DNS server deletion preserves effective tags', t => {
  const { props, state: s } = harness(t)

  props.config = {
    dns: {
      servers: Array.from({ length: 11 }, () => ({ type: 'local' })),
      final: '10',
      rules: [{ action: 'route', server: '3' }]
    },
    default_domain_resolver: { server: '10' },
    log: { level: 'warn' }
  }

  const expected = plain(props.config)

  s.askRemove('server', 2)
  s.remove()

  expected.dns.servers.forEach((server, i) => {
    if (i > 2) server.tag = String(i)
  })
  expected.dns.servers.splice(2, 1)

  assert.equal(s.error.value, '')
  assert.deepEqual(plain(props.config), expected)
  assert.deepEqual(plain(s.tags.value),
    ['0','1','3','4','5','6','7','8','9','10'])

  s.askRemove('server', 4)
  s.remove()
  expected.dns.servers.splice(4, 1)

  assert.equal(s.error.value, '')
  assert.deepEqual(plain(props.config), expected)
  assert.deepEqual(plain(s.tags.value),
    ['0','1','3','4','6','7','8','9','10'])

  s.openNewServer()
  s.saveDraft()

  assert.equal(s.error.value, '')
  assert.deepEqual(plain(s.tags.value),
    ['0','1','3','4','6','7','8','9','10','dns-1'])

  assert.deepEqual(
    plain(props.config.dns.servers.slice(0, -1)),
    expected.dns.servers
  )
})

test('Referenced implicit DNS server cannot be replaced by index shift', t => {
  const { props, state: s } = harness(t)

  props.config = {
    dns: {
      servers: [
        { type: 'local' },
        { type: 'https', server: '1.1.1.1' },
        { type: 'https', server: '8.8.8.8' }
      ],
      final: '1',
      rules: [{ action: 'route', server: '1' }]
    }
  }

  const expected = plain(props.config)

  s.askRemove('server', 1)
  s.remove()

  assert.ok(s.error.value)
  assert.deepEqual(plain(props.config), expected)
})

test('Explicit and implicit DNS tags remain stable together', t => {
  const { props, state: s } = harness(t)

  props.config = {
    dns: {
      servers: [
        { tag: 'unused', type: 'local' },
        { type: 'local' },
        { tag: 'cloudflare', type: 'https', server: '1.1.1.1' },
        { tag: '9', type: 'local' }
      ],
      final: 'cloudflare',
      rules: [{ action: 'route', server: '1' }]
    }
  }

  const expected = plain(props.config)
  expected.dns.servers[1].tag = '1'
  expected.dns.servers.shift()

  s.askRemove('server', 0)
  s.remove()

  assert.equal(s.error.value, '')
  assert.deepEqual(plain(props.config), expected)
  assert.deepEqual(plain(s.tags.value),
    ['1','cloudflare','9'])
})
