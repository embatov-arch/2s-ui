const { test } = require('node:test')
const assert = require('node:assert/strict')
const path = require('node:path')
const { editDns, validateDns, parseTemplate, setResolver, toggleDns, copy, removeDnsServer, dnsServerTag } = require(path.join(process.env.DNS_TEST_BUILD, 'subscriptionDns.js'))
const seed = () => ({ dns: { servers: [{ tag: 'cloudflare', type: 'https', server: '1.1.1.1', tls: { server_name: 'cloudflare-dns.com' } }, { tag: 'local', type: 'local' }], final: 'cloudflare', rules: [{ action: 'evaluate', server: 'cloudflare', tag: 'cf' }, { match_response: 'cf', action: 'respond' }] },
  default_domain_resolver: { server: 'local', strategy: 'ipv4_only', timeout: '3s', future: true }, rules: [{ action: 'sniff' }, { domain: ['example.org'], outbound: 'direct', action: 'route' }] })
const cat = [{ tag: 'geosite-private', type: 'remote', url: 'https://example.org/r.srs' }]
const route = (server, rest = {}) => ({ action: 'route', server, ...rest })
for (const [name, edit, error] of [
  ['missing resolver', x => setResolver(x, 'missing'), /unknown DNS tag/],
  ['duplicate DNS tag', x => x.dns.servers.push({ tag: 'local' }), /duplicate tag/],
  ['referenced server', x => x.dns.servers.pop(), /unknown DNS tag/],
  ['evaluate ordering', x => x.dns.rules.reverse(), /preceding evaluate/],
  ['nested evaluate ordering', x => x.dns.rules.unshift(route('local', { type: 'logical', rules: [{ match_response: 'cf' }] })), /preceding evaluate/],
  ['duplicate evaluate', x => x.dns.rules.splice(1, 0, { action: 'evaluate', server: 'local', tag: 'cf' }), /duplicate evaluate/],
  ['missing anonymous evaluate', x => { delete x.dns.rules[1].match_response }, /untagged evaluate/],
  ['resolver cycle', x => { x.dns.servers[0].domain_resolver = 'local'; x.dns.servers[1].domain_resolver = { server: 'cloudflare' } }, /circular/],
  ['self cycle', x => { x.dns.servers[0].domain_resolver = 'cloudflare' }, /circular/],
  ['overlapping rule-set tags', x => { x.rule_set = [{ tag: ['a', 'b'] }, { tag: 'b' }] }, /duplicate tag/],
  ['malformed rule-set refs', x => { x.dns.rules[1].rule_set = [1] }, /expected string/],
  ['empty match tag', x => x.rules.push({ dns_server_address: { '': ['192.0.2.1/32'] } }), /unknown DNS tag/],
]) test(name, () => { const t = seed(), before = copy(t); assert.throws(() => editDns(t, edit), error); assert.deepEqual(t, before) })

test('transaction preserves unknown fields, TLS, order and resolver options', () => {
  const t = seed(); t.dns.servers[0].future = { a: 1 }; t.dns.rules[0].dns_server_address = ['1.1.1.1']
  const expected = copy(t); expected.dns.servers[0].server = '9.9.9.9'; expected.dns.final = 'local'
  assert.deepEqual(editDns(t, x => { x.dns.servers[0].server = '9.9.9.9'; x.dns.final = 'local' }), expected)
  assert.equal(t.dns.servers[0].server, '1.1.1.1'); assert.equal(t.dns.final, 'cloudflare')
  expected.default_domain_resolver = { ...t.default_domain_resolver, server: 'cloudflare' }; expected.dns.final = t.dns.final; expected.dns.servers = t.dns.servers
  assert.deepEqual(editDns(t, x => setResolver(x, 'cloudflare')), expected)
})
test('rule arrays and catalog definitions', () => {
  const t = seed(), rule = route('local', { rule_set: cat[0].tag })
  const next = editDns(t, x => x.dns.rules.push(rule), cat)
  assert.deepEqual(next.dns.rules, [...t.dns.rules, rule]); assert.deepEqual(next.rule_set, cat)
  assert.deepEqual(editDns(next, x => x.dns.rules.pop(), cat).rule_set, []); assert.deepEqual(next.rule_set, cat)
  delete t.dns.rules; assert.deepEqual(editDns(t, x => (x.dns.rules ??= []).push(rule), cat).dns.rules, [rule])
  const group = { tag: ['one', 'two'], type: 'remote', url: 'https://example.org/{tag}.srs' }
  t.rule_set = [group]; t.dns.rules = [route('local', { rule_set: ['one', 'two'] })]
  assert.deepEqual(editDns(t, x => { x.dns.strategy = 'ipv4_only' }).rule_set, [group])
})
test('implicit tags, anonymous evaluate and JSON shapes', () => {
  for (const raw of ['null', '[]', '{"dns":{"servers":{}}}', '{"dns":{"rules":[null]}}']) assert.throws(() => parseTemplate(raw))
  assert.deepEqual(parseTemplate(''), {})
  const t = { dns: { servers: [{ type: 'local' }, { type: 'local' }], final: '1' } }
  assert.deepEqual(editDns(t, x => { x.dns.strategy = 'ipv4_only' }).dns.servers, t.dns.servers)
  assert.throws(() => editDns(t, x => { x.dns.final = 'missing' })); assert.throws(() => editDns(t, x => { x.dns.servers[1].tag = '0' }))
  const a = seed(); delete a.dns.rules[0].tag; a.dns.rules[1].match_response = true; validateDns(a)
})
test('DNS switch preserves routing, refuses dependencies and copies defaults', () => {
  const t = seed(), sniff = { action: 'sniff' }, hijack = { protocol: 'dns', action: 'hijack-dns' }, reject = { protocol: 'dns', action: 'reject' }
  assert.throws(() => toggleDns(t, false, {}), /default_domain_resolver/); delete t.default_domain_resolver
  const defaults = { servers: [{ tag: 'local', type: 'local' }] }
  assert.deepEqual(toggleDns({ rules: [] }, true, defaults).rules, [sniff, hijack])
  const enabled = toggleDns({ rules: t.rules }, true, defaults)
  assert.deepEqual(enabled.rules, [sniff, hijack, ...t.rules.slice(1)]); assert.deepEqual(toggleDns(enabled, true, defaults), enabled)
  const custom = [{ port: 53, action: 'hijack-dns' }]; assert.deepEqual(toggleDns({ rules: custom }, true, defaults).rules, custom)
  assert.throws(() => toggleDns({ ...t, rules: custom }, false, {}), /custom rules/)
  const fresh = toggleDns({}, true, defaults); assert.deepEqual(fresh.rules.map(r => r.action), ['sniff', 'hijack-dns', 'route', 'route'])
  fresh.dns.servers[0].tag = 'other'; assert.equal(defaults.servers[0].tag, 'local')
  t.rules.unshift({ ...hijack, protocol: ['dns'] }, reject); const disabled = toggleDns(t, false, {})
  assert.equal(disabled.dns, undefined); assert.deepEqual(disabled.rules, t.rules.slice(1))
})
for (const [name, extra, internal = false] of [
  ['rule-set inline HTTP client', { rule_set: [{ tag: 'r', http_client: { domain_resolver: 'local' } }] }],
  ['named HTTP client', { http_clients: [{ tag: 'h', domain_resolver: { server: 'local', strategy: 'ipv4_only' } }] }],
  ['ACME HTTP client', { inbounds: [{ tls: { certificate_provider: { type: 'acme', http_client: { domain_resolver: 'local' } } } }] }],
  ['Reality handshake', { inbounds: [{ tls: { reality: { handshake: { domain_resolver: 'local' } } } }] }],
  ['route resolve', { rules: [{ type: 'logical', rules: [{ domain: ['a.example'] }], action: 'resolve', server: 'local' }] }],
  ...['dns_server_address', 'dns_search_domain'].flatMap(field => [false, true].map(dns => [field + (dns ? ' DNS' : ' route'), { logical: { type: 'logical', rules: [{ [field]: { local: [field === 'dns_server_address' ? '192.0.2.1/32' : 'example.org'] } }] } }, dns])),
  ['preferred_by', { logical: { preferred_by: 'local' } }, true],
]) test(name, () => {
  const t = seed(); delete t.default_domain_resolver
  if (extra.logical) (internal ? t.dns.rules : t.rules).push(internal ? route('cloudflare', extra.logical) : { action: 'route', outbound: 'direct', ...extra.logical }); else Object.assign(t, extra)
  validateDns(t); assert.throws(() => editDns(t, x => x.dns.servers.pop()), /unknown DNS tag local/)
  if (!internal) assert.throws(() => toggleDns(t, false, {}), /custom rules/)
})
test('opaque data and empty references are not DNS dependencies', () => {
  const t = seed(); delete t.default_domain_resolver
  const headers = { domain_resolver: ['opaque'], default_domain_resolver: ['opaque'], dns_server_address: ['opaque'], action: ['hijack-dns'] }
  t.http_clients = [{ tag: 'h', headers, domain_resolver: '' }]; t.rule_set = [{ tag: 'r', http_client: { headers } }]
  t.dns.servers[0].headers = headers; t.dns.servers[0].future = { domain_resolver: 'opaque' }
  t.dns.servers.push({ tag: 'hosts', type: 'hosts', predefined: { domain_resolver: ['192.0.2.1'] } })
  t.rules.push({ action: 'resolve', server: '' }, { preferred_by: ['wireguard'], action: 'route', outbound: 'direct' })
  const next = editDns(t, x => { x.dns.strategy = 'ipv4_only' }); assert.deepEqual(next.dns.servers, t.dns.servers)
  assert.deepEqual(next.http_clients, t.http_clients); assert.deepEqual(toggleDns(t, false, {}).http_clients, t.http_clients)
})

test('route resolve may use the native local fallback only without explicit DNS servers', () => {
  const rules = [{ domain_suffix: ['example.org'], action: 'resolve', server: 'local' }]
  validateDns({ rules }); validateDns({ dns: { servers: [] }, rules })
  assert.throws(() => validateDns({ dns: { servers: [{ type: 'https', tag: 'cf', server: '1.1.1.1' }] }, rules }), /unknown DNS tag/)
  assert.throws(() => validateDns({ rules: [{ ...rules[0], server: 'missing' }] }), /unknown DNS tag/)
})

test('removing implicit servers preserves all remaining tags and refuses dangling references', () => {
  const t = { dns: { servers: [{ tag: 'unused', type: 'local' }, ...Array.from({ length: 10 }, () => ({ type: 'local' }))], final: '10', rules: [route('3')] } }
  let next = t
  for (const i of [0, 3]) {
    const expected = next.dns.servers.map(dnsServerTag).filter((_, j) => j !== i)
    next = editDns(next, x => removeDnsServer(x.dns.servers, i))
    assert.deepEqual(next.dns.servers.map(dnsServerTag), expected)
  }
  for (const tag of ['3', '10']) {
    const index = next.dns.servers.map(dnsServerTag).indexOf(tag)
    assert.throws(() => editDns(next, x => removeDnsServer(x.dns.servers, index)), /unknown DNS tag/)
  }
})
