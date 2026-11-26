// JSON subscription rules may reference a rule-set by a string or a list,
// including inside logical rules. Do not rebuild the user's catalog.
type CatalogEntry = { tag: string }

// sing-box 1.14 accepts multiple tags in a single local or remote definition.
export function ruleSetTags(value: unknown): string[] {
  const tags = typeof value === 'string' ? [value] : value
  return Array.isArray(tags) && tags.length > 0 && tags.every(t => typeof t === 'string' && t !== '') ? tags : []
}

export const isObject = (v: unknown): v is Record<string, any> => v !== null && typeof v === 'object' && !Array.isArray(v)
export const refs = (v: unknown): string[] => typeof v === 'string' ? [v] : Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []

function collectRuleSetTags(rules: unknown, tags: Set<string>): void {
  if (!Array.isArray(rules)) return
  for (const rule of rules) {
    if (!isObject(rule)) continue
    for (const tag of refs(rule.rule_set)) if (tag) tags.add(tag)
    collectRuleSetTags(rule.rules, tags)
  }
}

export function referencedRuleSets(template: Record<string, unknown>): Set<string> {
  const tags = new Set<string>()
  if (isObject(template.dns)) collectRuleSetTags(template.dns.rules, tags)
  collectRuleSetTags(template.rules, tags)
  return tags
}

// Only remove an unchanged stock entry when an interface edit removed its
// last reference. The stock catalog consists of flat scalar-valued objects;
// custom definitions, extra fields, grouped tags and existing unused entries
// are never removed. Key order is irrelevant.
export function syncRuleSets(template: Record<string, unknown>, catalog: readonly CatalogEntry[], before?: ReadonlySet<string>): void {
  const existing = template.rule_set
  if (existing !== undefined && (!Array.isArray(existing) || existing.some(r => !isObject(r) || !ruleSetTags(r.tag).length))) return
  const current = Array.isArray(existing) ? existing : []
  const tags = referencedRuleSets(template)
  const kept = current.filter(r => !(typeof r.tag === 'string' && before?.has(r.tag) && !tags.has(r.tag) &&
    catalog.some(c => c.tag === r.tag && Object.keys(c).length === Object.keys(r).length &&
      Object.entries(c).every(([key, value]) => Object.hasOwn(r, key) && r[key] === value))))
  const known = new Set(kept.flatMap(r => ruleSetTags(r.tag)))
  const additions: CatalogEntry[] = []
  for (const entry of catalog) {
    if (!tags.has(entry.tag) || known.has(entry.tag)) continue
    additions.push(JSON.parse(JSON.stringify(entry)))
    known.add(entry.tag)
  }
  if (kept.length !== current.length || additions.length) template.rule_set = [...kept, ...additions]
}
