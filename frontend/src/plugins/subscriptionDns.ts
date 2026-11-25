import { appendMissingRuleSets } from './subscriptionRuleSets'

type Obj = Record<string, any>
export type Catalog = readonly { tag: string }[]
export const isObject = (v: unknown): v is Obj => v !== null && typeof v === 'object' && !Array.isArray(v)
export const copy = <T>(v: T): T => JSON.parse(JSON.stringify(v))
const objects = (v: unknown): v is Obj[] => Array.isArray(v) && v.every(isObject)
const fail = (path: string): never => { throw new Error(path) }
export const refs = (v: unknown): string[] => typeof v === 'string' ? [v] : Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
export const resolverTag = (v: unknown): string => typeof v === 'string' ? v : isObject(v) && typeof v.server === 'string' ? v.server : ''

// Guard the shapes read by Settings.vue. Do not discard unknown native keys.
export function parseTemplate(text: string): Obj {
  const value: unknown = text.trim() === '' ? {} : JSON.parse(text)
  if (!isObject(value)) fail('JSON: object expected')
  const t = value as Obj
  for (const key of ['dns', 'log', 'experimental']) if (key in t && !isObject(t[key])) fail(key + ': object expected')
  for (const key of ['rules', 'rule_set', 'inbounds']) if (key in t && !objects(t[key])) fail(key + ': array of objects expected')
  if (isObject(t.dns)) for (const key of ['servers', 'rules']) if (key in t.dns && !objects(t.dns[key])) fail('dns.' + key + ': array of objects expected')
  return t
}

export function validateDns(t: Obj): void {
  const d = t.dns
  if (d === undefined) {
    if (resolverTag(t.default_domain_resolver)) fail('default_domain_resolver: DNS section is absent')
    return
  }
  if (!isObject(d)) fail('dns: object expected')
  const servers: unknown = d.servers ?? []
  if (!objects(servers)) fail('dns.servers: array of objects expected')
  const tags = new Set<string>()
  const serverList = servers as Obj[]
  for (const [i, s] of serverList.entries()) {
    if (s.tag !== undefined && typeof s.tag !== 'string') fail(`dns.servers[${i}].tag`)
    if (typeof s.tag === 'string' && s.tag) {
      if (tags.has(s.tag)) fail('dns.servers: duplicate tag ' + s.tag)
      tags.add(s.tag)
    }
  }
  // If an unnamed server exists, leave implicit tag semantics to the native core.
  const allNamed = serverList.every((s: Obj) => typeof s.tag === 'string' && s.tag !== '')
  const checkTag = (v: unknown, path: string) => {
    if (v === undefined || v === '') return
    if (typeof v !== 'string' || (allNamed && !tags.has(v))) fail(path + ': unknown DNS tag ' + String(v))
  }
  const checkResolver = (v: unknown, path: string) => {
    if (v === undefined || v === '') return
    if (isObject(v)) {
      if (!resolverTag(v)) fail(path + '.server: required')
      checkTag(v.server, path + '.server')
    } else checkTag(v, path)
  }
  checkTag(d.final, 'dns.final')
  checkResolver(t.default_domain_resolver, 'default_domain_resolver')
  for (const [i, s] of serverList.entries()) checkResolver(s.domain_resolver, `dns.servers[${i}].domain_resolver`)
  const catalog = new Set<string>((t.rule_set ?? []).map((r: Obj) => r.tag))
  const evaluated = new Set<string>()
  const checkRules = (rules: unknown, path: string, top = true) => {
    if (rules === undefined) return
    if (!objects(rules)) fail(path + ': array of objects expected')
    for (const [i, r] of (rules as Obj[]).entries()) {
      const p = `${path}[${i}]`
      checkTag(r.server, p + '.server')
      if (top && (!r.action || r.action === 'route' || r.action === 'evaluate') && !r.server) fail(p + '.server: required')
      if (r.match_response === true && !evaluated.has('')) fail(p + '.match_response: preceding untagged evaluate required')
      if (typeof r.match_response === 'string' && !evaluated.has(r.match_response)) fail(p + '.match_response: preceding evaluate required')
      if (top && r.action === 'respond' && evaluated.size === 0) fail(p + ': preceding evaluate required')
      for (const tag of refs(r.preferred_by)) checkTag(tag, p + '.preferred_by')
      for (const tag of refs(r.rule_set)) if (!catalog.has(tag)) fail(p + '.rule_set: unknown tag ' + tag)
      checkRules(r.rules, p + '.rules', false)
      if (top && r.action === 'evaluate') evaluated.add(typeof r.tag === 'string' ? r.tag : '')
    }
  }
  checkRules(d.rules, 'dns.rules')
  // Explicit route resolve and dial references in the template must also survive.
  const walk = (v: unknown, path: string) => {
    if (Array.isArray(v)) { v.forEach((x, i) => walk(x, `${path}[${i}]`)); return }
    if (!isObject(v)) return
    if (v.action === 'resolve') checkTag(v.server, path + '.server')
    if ('domain_resolver' in v) checkResolver(v.domain_resolver, path + '.domain_resolver')
    if (Array.isArray(v.rules)) walk(v.rules, path + '.rules')
  }
  walk(t.rules, 'rules')
  for (const key of ['inbounds', 'http_clients']) if (Array.isArray(t[key])) t[key].forEach((v: Obj, i: number) => { if (isObject(v)) checkResolver(v.domain_resolver, `${key}[${i}].domain_resolver`) })
}

export function editDns(t: Obj, edit: (next: Obj) => void, catalog: Catalog = []): Obj {
  const next = copy(t)
  edit(next)
  parseTemplate(JSON.stringify(next))
  appendMissingRuleSets(next, catalog)
  validateDns(next)
  return next
}

export function setResolver(t: Obj, tag: string): void {
  if (!tag) { delete t.default_domain_resolver; return }
  const previous = t.default_domain_resolver
  t.default_domain_resolver = isObject(previous) ? { ...previous, server: tag } : tag
}

const plainHijack = (r: Obj) => r.action === 'hijack-dns' && Object.keys(r).length === 2 && refs(r.protocol).length === 1 && refs(r.protocol)[0] === 'dns'
const hasDependency = (v: unknown): boolean => {
  if (Array.isArray(v)) return v.some(hasDependency)
  if (!isObject(v)) return false
  return v.action === 'hijack-dns' || (v.action === 'resolve' && !!v.server) || !!v.domain_resolver || Object.values(v).some(hasDependency)
}
export function toggleDns(t: Obj, enabled: boolean, defaults: Obj): Obj {
  const next = copy(t)
  if (enabled) {
    if (next.dns !== undefined) return next
    next.dns = copy(defaults)
    // Materialize the actual backend defaults only if rules were absent.
    // Existing explicit rules, including [], belong to the operator.
    if (next.rules === undefined) next.rules = [
      { action: 'sniff' }, { protocol: 'dns', action: 'hijack-dns' },
      { clash_mode: 'Direct', action: 'route', outbound: 'direct' },
      { clash_mode: 'Global', action: 'route', outbound: 'proxy' },
    ]
  } else {
    if (next.dns === undefined) return next
    if (resolverTag(next.default_domain_resolver)) fail('Clear default_domain_resolver before removing DNS')
    const remaining = (next.rules ?? []).filter((r: Obj) => !plainHijack(r))
    if (hasDependency(remaining) || hasDependency(next.inbounds) || hasDependency(next.http_clients)) fail('DNS is referenced by custom rules or dial options; edit these first')
    // Only the exact standalone DNS interception rule belongs to this action.
    // Keep reject, resolve without explicit server, sniff, and all custom rules.
    if (next.rules !== undefined) next.rules = remaining
    delete next.dns
  }
  return next
}
