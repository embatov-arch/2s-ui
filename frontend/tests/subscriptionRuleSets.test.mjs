import test from 'node:test'
import assert from 'node:assert/strict'
import { appendMissingRuleSets } from '../src/plugins/subscriptionRuleSets.ts'

const catalog = [
  { tag: 'geosite-private', type: 'remote', url: 'https://example.org/default.srs' },
  { tag: 'geosite-cn', type: 'remote', url: 'https://example.org/cn.srs' },
]
const mirror = { ...catalog[0], url: 'https://example.org/custom.srs' }
const custom = { tag: 'custom', type: 'inline', rules: [{ domain: ['example.org'] }] }
const run = (template, entries = catalog) => {
  appendMissingRuleSets(template, entries)
  return template
}
test('scalar reference preserves mirror', () => {
  const t = { dns: { rules: [{ rule_set: mirror.tag }] }, rule_set: [mirror] }
  assert.deepEqual(run(t).rule_set, [mirror])
})
test('nested DNS and route logical rules add missing sets', () => {
  const t = { dns: { rules: [{ type: 'logical', mode: 'and', rules: [{ rule_set: mirror.tag }] }] }, rules: [{ type: 'logical', mode: 'or', rules: [{ rule_set: ['geosite-cn'] }] }] }
  const before = structuredClone(t)
  assert.deepEqual(run(t).rule_set, catalog)
  assert.deepEqual(t.rules, before.rules)
  assert.deepEqual(t.dns, before.dns)
})
test('explicit empty route rules survive DNS edits', () => {
  const t = { rules: [], dns: { rules: [{ rule_set: mirror.tag }] } }
  assert.deepEqual(run(t).rules, [])
})
test('unreferenced custom and catalog entries are preserved in order', () => {
  const t = { rule_set: [custom, mirror], dns: { rules: [] } }
  assert.deepEqual(run(t).rule_set, [custom, mirror])
})
test('existing entries win; missing entries appended in catalog order', () => {
  const t = { rule_set: [custom, mirror], rules: [{ rule_set: ['geosite-cn', mirror.tag] }] }
  assert.deepEqual(run(t).rule_set, [custom, mirror, catalog[1]])
})
test('no duplicate insertion, including duplicate catalog entries', () => {
  const t = { rules: [{ rule_set: ['geosite-cn', 'geosite-cn'] }] }
  run(t, [...catalog, catalog[1]])
  run(t)
  assert.deepEqual(t.rule_set, [catalog[1]])
})
test('catalog defaults are not mutated by a later edit', () => {
  const t = run({ rules: [{ rule_set: mirror.tag }] })
  t.rule_set[0].url = 'changed'
  assert.equal(catalog[0].url, 'https://example.org/default.srs')
})
test('missing and explicit empty catalog are not materialized unnecessarily', () => {
  assert.equal(Object.hasOwn(run({}), 'rule_set'), false)
  assert.deepEqual(run({ rule_set: [] }), { rule_set: [] })
})
test('malformed catalog preserved rather than overwritten', () => {
  for (const value of ['bad', null, [null], [{ url: 'missing-tag' }]]) {
    const t = { rule_set: value, rules: [{ rule_set: mirror.tag }] }
    assert.deepEqual(run(t).rule_set, value)
  }
})
test('unknown references not fabricated or removed', () => {
  const t = { dns: { rules: [{ rule_set: 'unknown' }] } }
  assert.deepEqual(run(t), { dns: { rules: [{ rule_set: 'unknown' }] } })
})

test('grouped rule-set definitions remain intact and prevent duplicate catalog insertion', () => {
  const group = { type: 'remote', tag: ['geosite-cn', 'custom'], url: 'https://example.org/{tag}.srs' }
  const t = { rule_set: [group], dns: { rules: [{ rule_set: ['geosite-cn', 'custom', 'geosite-private'] }] } }
  assert.deepEqual(run(t).rule_set, [group, catalog[0]])
})
