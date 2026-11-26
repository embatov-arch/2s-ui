import { syncRuleSets, referencedRuleSets, ruleSetTags, isObject, refs } from './subscriptionRuleSets'
export { isObject, refs } from './subscriptionRuleSets'

type Obj = Record<string, any>
export type Catalog = readonly { tag: string }[]
export const copy = <T>(v: T): T => JSON.parse(JSON.stringify(v))
const objects = (v: unknown): v is Obj[] => Array.isArray(v) && v.every(isObject)
const fail = (path: string): never => { throw new Error(path) }
export const resolverTag = (v: unknown): string => typeof v === 'string' ? v : isObject(v) && typeof v.server === 'string' ? v.server : ''

export const dnsServerTag = (server: Obj, index: number): string =>
  typeof server.tag === 'string' && server.tag !== '' ? server.tag : String(index)

export function removeDnsServer(servers: Obj[], index: number): void {
  // Pin implicit tags before deleting an earlier server changes their indices.
  for (let i = index + 1; i < servers.length; i++) {
    if (servers[i].tag === undefined || servers[i].tag === '') servers[i].tag = dnsServerTag(servers[i], i)
  }
  servers.splice(index, 1)
}

// Guard the shapes read by Settings.vue. Do not discard unknown native keys.
function checkTemplate(value: unknown): Obj {
  if (!isObject(value)) fail('JSON: object expected')
  const t = value as Obj
  for (const key of ['dns', 'log', 'experimental']) if (key in t && !isObject(t[key])) fail(key + ': object expected')
  for (const key of ['rules', 'rule_set', 'inbounds']) if (key in t && !objects(t[key])) fail(key + ': array of objects expected')
  if (isObject(t.dns)) for (const key of ['servers', 'rules']) if (key in t.dns && !objects(t.dns[key])) fail('dns.' + key + ': array of objects expected')
  return t
}
export const parseTemplate = (text: string): Obj => checkTemplate(text.trim() ? JSON.parse(text) : {})

// Follow configuration objects, not arbitrary JSON values such as HTTP
// headers, hosts records or unknown extension data. Route and DNS rules have
// different meanings for preferred_by, and only DNS rules name DNS servers
// with their route/evaluate actions.
type DnsRef = { tag: unknown; path: string }
const checkedRefs = (value: unknown, path: string): string[] => {
  if (value == null) return []
  if (typeof value === 'string') return [value]
  if (!Array.isArray(value) || !value.every(x => typeof x === 'string')) return fail(path + ': expected string or list of strings')
  return value
}
function dnsRefs(t: Obj): DnsRef[] {
  const out: DnsRef[] = []
  const add = (tag: unknown, path: string, resolver = false) => {
    if (tag == null || tag === '') return
    if (resolver && isObject(tag) && !resolverTag(tag)) fail(path + '.server: required')
    out.push({ tag: resolver ? resolverTag(tag) || tag : tag, path })
  }
  const walk = (v: unknown, path: string, kind?: 'dns' | 'route') => {
    if (Array.isArray(v)) { v.forEach((x, i) => walk(x, `${path}[${i}]`, kind)); return }
    if (!isObject(v)) return
    add(v.domain_resolver, path + '.domain_resolver', true)
    if (kind) {
      if (kind === 'route' && v.action === 'hijack-dns') out.push({ tag: null, path })
      if (kind === 'dns' || v.action === 'resolve') add(v.server, path + '.server')
      if (kind === 'dns') for (const tag of checkedRefs(v.preferred_by, path + '.preferred_by')) out.push({ tag, path: path + '.preferred_by' })
      for (const key of ['dns_server_address', 'dns_search_domain']) {
        if (isObject(v[key])) for (const tag of Object.keys(v[key])) out.push({ tag, path: path + '.' + key })
      }
      walk(v.rules, path + '.rules', kind)
      return
    }
    // Native nested dial options: inline rule-set HTTP clients, inbound TLS
    // Reality handshakes and inline certificate providers/ACME HTTP clients.
    for (const key of ['tls', 'reality', 'handshake', 'certificate_provider', 'acme', 'http_client']) {
      if (isObject(v[key])) walk(v[key], path + '.' + key)
    }
  }
  add(t.default_domain_resolver, 'default_domain_resolver', true)
  add(t.dns?.final, 'dns.final')
  walk(t.dns?.servers, 'dns.servers')
  walk(t.dns?.rules, 'dns.rules', 'dns')
  walk(t.rules, 'rules', 'route')
  for (const key of ['inbounds', 'http_clients', 'rule_set']) walk(t[key], key)
  return out
}

// Keeps the template consistent while it is edited: every DNS server, rule-set
// and evaluate response a rule names exists, and exists before it is used.
// What each option means is left to the subscriber's sing-box.
export function validateDns(t: Obj): void {
  checkTemplate(t)
  const d: Obj = t.dns ?? {}
  const serverList: Obj[] = d.servers ?? []
  const tags = new Set<string>()
  for (const [i, s] of serverList.entries()) {
    if (s.tag !== undefined && typeof s.tag !== 'string') fail(`dns.servers[${i}].tag`)
    const tag = dnsServerTag(s, i)
    if (tags.has(tag)) fail('dns.servers: duplicate tag ' + tag)
    tags.add(tag)
  }
  for (const ref of dnsRefs(t)) {
    if (ref.tag === null) continue // A DNS interception rule has no server tag.
    // With no explicit servers, sing-box creates a local transport at startup.
    if (ref.tag === 'local' && !serverList.length && !d.final && ref.path.startsWith('rules[') && ref.path.endsWith('.server')) continue
    if (typeof ref.tag !== 'string' || !tags.has(ref.tag)) fail(ref.path + ': unknown DNS tag ' + String(ref.tag))
  }
  // Only explicit resolver links are checked here; implicit behavior is native.
  const resolverLinks = new Map(serverList.map((s, i) => [dnsServerTag(s, i), resolverTag(s.domain_resolver)]))
  for (const start of resolverLinks.keys()) {
    const seen = new Set<string>()
    let tag = start
    while (resolverLinks.has(tag)) {
      if (seen.has(tag)) fail('dns.servers: circular domain_resolver dependency ' + tag)
      seen.add(tag)
      tag = resolverLinks.get(tag) ?? ''
    }
  }
  const catalog = new Set<string>()
  for (const r of t.rule_set ?? []) {
    const defined = ruleSetTags(r.tag)
    if (!defined.length) fail('rule_set: tag required')
    for (const tag of defined) {
      if (catalog.has(tag)) fail('rule_set: duplicate tag ' + tag)
      catalog.add(tag)
    }
  }
  // evaluate/respond: a response is only there to match once an earlier
  // evaluate fetched it, so their order is part of the template's integrity.
  const evaluated = new Set<string>()
  const checkRules = (rules: unknown, path: string, top = true) => {
    if (rules === undefined) return
    if (!objects(rules)) fail(path + ': array of objects expected')
    for (const [i, r] of (rules as Obj[]).entries()) {
      const p = `${path}[${i}]`
      // The editor opens a new rule with an empty server.
      if (top && (!r.action || r.action === 'route' || r.action === 'evaluate') && !r.server) fail(p + '.server: required')
      const match = r.match_response
      if (match != null && typeof match !== 'boolean' && typeof match !== 'string') fail(p + '.match_response: expected boolean or tag')
      if (match === true && !evaluated.has('')) fail(p + '.match_response: preceding untagged evaluate required')
      if (typeof match === 'string' && (!match || !evaluated.has(match))) fail(p + '.match_response: preceding evaluate required')
      if (top && r.action === 'respond' && typeof match !== 'string' && !evaluated.has('')) fail(p + ': preceding untagged evaluate required')
      for (const tag of checkedRefs(r.rule_set, p + '.rule_set')) if (!catalog.has(tag)) fail(p + '.rule_set: unknown tag ' + tag)
      checkRules(r.rules, p + '.rules', false)
      if (top && r.action === 'evaluate') {
        if (r.tag != null && typeof r.tag !== 'string') fail(p + '.tag: expected string')
        const tag = r.tag ?? ''
        if (tag && evaluated.has(tag)) fail(p + ': duplicate evaluate tag ' + tag)
        evaluated.add(tag)
      }
    }
  }
  checkRules(d.rules, 'dns.rules')
}

export function editDns(t: Obj, edit: (next: Obj) => void, catalog: Catalog = []): Obj {
  const next = copy(t)
  edit(next)
  syncRuleSets(next, catalog, referencedRuleSets(t))
  validateDns(next)
  return next
}

export function setResolver(t: Obj, tag: string): void {
  if (!tag) { delete t.default_domain_resolver; return }
  const previous = t.default_domain_resolver
  t.default_domain_resolver = isObject(previous) ? { ...previous, server: tag } : tag
}

const plainHijack = (r: Obj) => r.action === 'hijack-dns' && Object.keys(r).length === 2 && refs(r.protocol).length === 1 && refs(r.protocol)[0] === 'dns'
export function toggleDns(t: Obj, enabled: boolean, defaults: Obj): Obj {
  const next = copy(t)
  if (enabled) {
    next.dns ??= copy(defaults)
    // Preserve existing routing and restore standard DNS interception.
    if (next.rules === undefined) next.rules = [
      { action: 'sniff' }, { protocol: 'dns', action: 'hijack-dns' },
      { clash_mode: 'Direct', action: 'route', outbound: 'direct' },
      { clash_mode: 'Global', action: 'route', outbound: 'proxy' },
    ]
    if (!next.rules.some((r: Obj) => r.action === 'hijack-dns')) {
      const hijack = { protocol: 'dns', action: 'hijack-dns' }
      const sniff = next.rules.findIndex((r: Obj) =>
        r.action === 'sniff' && Object.keys(r).length === 1)
      if (sniff < 0) next.rules.unshift({ action: 'sniff' }, hijack)
      else next.rules.splice(sniff + 1, 0, hijack)
    }
  } else {
    if (next.dns === undefined) return next
    // Only the exact standalone DNS interception rule belongs to this action.
    // Keep reject, resolve without explicit server, sniff, and all custom rules.
    delete next.dns
    if (next.rules !== undefined) next.rules = next.rules.filter((r: Obj) => !plainHijack(r))
    const used = dnsRefs(next)
    if (used.length) fail(`DNS is referenced by custom rules or dial options (${used.map(r => r.path).join(', ')}); edit these first`)
  }
  return next
}
