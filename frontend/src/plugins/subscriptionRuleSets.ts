// JSON subscription rules may reference a rule-set by a string or a list,
// including inside logical rules. Do not rebuild the user's catalog.
type CatalogEntry = { tag: string }

// sing-box 1.14 accepts multiple tags in a single local or remote definition.
export function ruleSetTags(value: unknown): string[] {
  const tags = typeof value === 'string' ? [value] : value
  return Array.isArray(tags) && tags.length > 0 && tags.every(t => typeof t === 'string' && t !== '') ? tags : []
}

function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function collectRuleSetTags(rules: unknown, tags: Set<string>): void {
  if (!Array.isArray(rules)) return
  for (const rule of rules) {
    if (!isObject(rule)) continue
    const refs = typeof rule.rule_set === 'string' ? [rule.rule_set] : rule.rule_set
    if (Array.isArray(refs)) {
      for (const tag of refs) {
        if (typeof tag === 'string' && tag !== '') tags.add(tag)
      }
    }
    collectRuleSetTags(rule.rules, tags)
  }
}

export function appendMissingRuleSets(template: Record<string, unknown>, catalog: readonly CatalogEntry[]): void {
  const existing = template.rule_set
  // Leave malformed user data intact. Validation belongs to the save path.
  if (existing !== undefined && (!Array.isArray(existing) || existing.some(r => !isObject(r) || !ruleSetTags(r.tag).length))) return
  const current = Array.isArray(existing) ? existing : []
  const known = new Set(current.flatMap(r => ruleSetTags(r.tag)))
  const tags = new Set<string>()
  if (isObject(template.dns)) collectRuleSetTags(template.dns.rules, tags)
  collectRuleSetTags(template.rules, tags)
  const additions: CatalogEntry[] = []
  for (const entry of catalog) {
    if (!tags.has(entry.tag) || known.has(entry.tag)) continue
    additions.push(JSON.parse(JSON.stringify(entry)))
    known.add(entry.tag)
  }
  if (additions.length) template.rule_set = [...current, ...additions]
}
