import test from 'node:test'
import assert from 'node:assert/strict'
import { syncRuleSets, referencedRuleSets } from '../src/plugins/subscriptionRuleSets.ts'
const catalog = [{ tag: 'a', type: 'remote', url: 'https://example.org/a.srs' }, { tag: 'b', type: 'remote', url: 'https://example.org/b.srs' }]
const [a, b] = catalog, mirror = { ...a, url: 'https://example.org/mirror.srs' }
const custom = { tag: 'custom', type: 'inline', rules: [{ domain: ['example.org'] }] }
const group = { ...b, tag: ['b', 'custom'], url: 'https://example.org/{tag}.srs' }
const rule = rule_set => ({ rule_set }), logical = tag => ({ type: 'logical', rules: [rule(tag)] })
for (const [name, template, expected] of [
  ['nested references', { dns: { rules: [logical('a')] }, rules: [logical(['b'])] }, catalog],
  ['mirror and order preserved', { rule_set: [custom, mirror], rules: [rule(['b', 'a', 'b'])] }, [custom, mirror, b]],
  ['unused definitions', { rule_set: [custom, mirror] }, [custom, mirror]],
  ['grouped tags', { rule_set: [group], dns: { rules: [rule(['b', 'custom', 'a'])] } }, [group, a]],
  ['empty route rules', { rules: [], dns: { rules: [rule('a')] } }, [a]],
  ['empty catalog', { rule_set: [] }, []], ['absent catalog', {}, undefined],
  ['unknown reference', { rules: [rule('unknown')] }, undefined],
  ...['bad', null, [null], [{ url: 'missing-tag' }]].map(value => ['malformed catalog', { rule_set: value, rules: [rule('a')] }, value]),
]) test(name, () => {
  const before = structuredClone(template)
  syncRuleSets(template, [...catalog, b]); syncRuleSets(template, catalog)
  assert.deepEqual(template.rule_set, expected)
  assert.deepEqual(template.dns, before.dns); assert.deepEqual(template.rules, before.rules)
  assert.equal(Object.hasOwn(template, 'rule_set'), expected !== undefined)
})
test('stock entries are copied, not shared', () => {
  const t = { rules: [rule('a')] }; syncRuleSets(t, catalog); t.rule_set[0].url = 'changed'
  assert.equal(a.url, 'https://example.org/a.srs')
})
test('only stock entry losing last reference is removed', () => {
  for (const definition of [{ url: a.url, type: a.type, tag: a.tag }, mirror, { ...a, update_interval: '1d' }, { ...a, tag: ['a'] }]) {
    const t = { rule_set: [definition, b, custom], dns: { rules: [rule('a')] }, rules: [logical('a')] }
    let before = referencedRuleSets(t); t.dns.rules = []; syncRuleSets(t, catalog, before)
    assert.deepEqual(t.rule_set, [definition, b, custom]); before = referencedRuleSets(t); t.rules = []; syncRuleSets(t, catalog, before)
    const stock = definition.tag === 'a' && definition.url === a.url && !definition.update_interval
    assert.deepEqual(t.rule_set, stock ? [b, custom] : [definition, b, custom]); assert.deepEqual(t.rules, [])
  }
})
