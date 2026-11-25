import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { subscriptionDnsMessages as messages } from '../src/locales/subscriptionDns.ts'

test('all six panel languages contain the same nonempty DNS labels', () => {
  assert.deepEqual(Object.keys(messages).sort(), ['en','fa','ru','vi','zhHans','zhHant'].sort())
  const keys = Object.keys(messages.en).sort()
  for (const [locale, labels] of Object.entries(messages)) {
    assert.deepEqual(Object.keys(labels).sort(), keys, locale)
    assert.ok(Object.values(labels).every(v => v.trim()), locale)
  }
  const component = readFileSync(new URL('../src/components/settings/SubscriptionDns.vue', import.meta.url), 'utf8')
  for (const match of component.matchAll(/subscriptionDns\.([a-zA-Z]+)/g)) assert.ok(keys.includes(match[1]), match[1])
  assert.ok(!component.includes('example.org'), 'new rules must not contain a sample domain')
})
