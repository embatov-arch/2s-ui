<template>
  <div class="sub-dns">
    <div class="dns-basics">
      <Field :label="$t('dns.final')" :mb="0">
        <Select :disabled="!!draft || !!pending" :model-value="dns.final ?? ''" @change="changeFinal">
          <option value="">{{ $t('ui.firstServer') }}</option>
          <option v-if="dns.final && !tags.includes(dns.final)" :value="dns.final">{{ dns.final }} (!)</option>
          <option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</option>
        </Select>
      </Field>
      <Field :label="$t('basic.routing.defaultDns')" :mb="0">
        <Select :disabled="!!draft || !!pending" :model-value="resolver" @change="changeResolver">
          <option value="">{{ $t('subscriptionDns.automatic') }}</option>
          <option v-if="resolver && !tags.includes(resolver)" :value="resolver">{{ resolver }} (!)</option>
          <option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</option>
        </Select>
      </Field>
      <Field :label="$t('ui.domainStrategy')" :mb="0">
        <Select :disabled="!!draft || !!pending" :model-value="dns.strategy ?? ''" @change="changeStrategy">
          <option value="">{{ $t('ui.none') }}</option>
          <option v-if="dns.strategy && !strategies.includes(dns.strategy)" :value="dns.strategy">{{ dns.strategy }}</option>
          <option v-for="strategy in strategies" :key="strategy" :value="strategy">{{ strategy }}</option>
        </Select>
      </Field>
    </div>
    <div class="dns-list-wrap">
    <details class="dns-list" open>
      <summary><Ico name="chevronDown" :size="16" class="dns-chevron" /><strong>{{ $t('ui.dnsServers') }}</strong><span class="dns-count">{{ servers.length }}</span></summary>
      <div class="dns-list-body">
        <div v-for="(server, i) in servers" :key="i" class="sub-dns-row">
          <button type="button" class="sub-dns-desc" :aria-label="$t('actions.edit') + ' ' + (dnsServerTag(server, i))" :disabled="!!draft || !!pending" :title="`${dnsServerTag(server, i)} · ${server.type || $t('subscriptionDns.legacy')} · ${server.server || '—'}`" @click="openItem('server', i)">
            <Ico name="edit" :size="16" class="dns-row-edit" />
            <strong>{{ dnsServerTag(server, i) }}</strong><span class="mono">{{ server.type || $t('subscriptionDns.legacy') }}<template v-if="server.server"> · {{ server.server }}<template v-if="server.server_port">:{{ server.server_port }}</template></template></span>
          </button>
          <Btn sm icon variant="subtle" class="dns-icon dns-delete" :aria-label="$t('actions.del')" :title="$t('actions.del')" :disabled="!!draft || !!pending" @click="askRemove('server', i)"><Ico name="trash" :size="16" /></Btn>
        </div>
      </div>
    </details>
      <Btn sm icon variant="subtle" class="dns-icon dns-add" :aria-label="$t('ui.addDnsServer')" :title="$t('ui.addDnsServer')" :disabled="!!draft || !!pending" @click="openNewServer"><Ico name="plus" :size="16" /></Btn>
    </div>
    <div class="dns-list-wrap">
    <details class="dns-list" open>
      <summary><Ico name="chevronDown" :size="16" class="dns-chevron" /><strong>{{ $t('ui.dnsRules') }}</strong><span class="dns-count">{{ rules.length }}</span></summary>
      <div class="dns-list-body">
        <div v-for="(rule, i) in rules" :key="i" class="sub-dns-row">
          <button type="button" class="sub-dns-desc" :aria-label="$t('actions.edit') + ' ' + $t('objects.dnsrule') + ' #' + (i + 1)" :title="ruleSummary(rule)" :disabled="!!draft || !!pending" @click="openItem('rule', i)">
            <Ico name="edit" :size="16" class="dns-row-edit" />
            <span class="dns-num">{{ i + 1 }}.</span><strong>{{ rule.action || 'route' }}<template v-if="rule.server"> → {{ rule.server }}</template></strong><span class="mono">{{ ruleSummary(rule) }}</span>
          </button>
          <Btn sm icon variant="subtle" class="dns-icon" :aria-label="$t('subscriptionDns.up') + ' ' + (i + 1)" :title="$t('subscriptionDns.up')" :disabled="!!draft || !!pending || i === 0" @click="move(i, -1)"><Ico name="chevronDown" :size="16" style="transform:rotate(180deg)" /></Btn>
          <Btn sm icon variant="subtle" class="dns-icon" :aria-label="$t('subscriptionDns.down') + ' ' + (i + 1)" :title="$t('subscriptionDns.down')" :disabled="!!draft || !!pending || i === rules.length - 1" @click="move(i, 1)"><Ico name="chevronDown" :size="16" /></Btn>
          <Btn sm icon variant="subtle" class="dns-icon dns-delete" :aria-label="$t('actions.del')" :title="$t('actions.del')" :disabled="!!draft || !!pending" @click="askRemove('rule', i)"><Ico name="trash" :size="16" /></Btn>
        </div>
      </div>
    </details>
      <Btn sm icon variant="subtle" class="dns-icon dns-add" :aria-label="$t('ui.addDnsRule')" :title="$t('ui.addDnsRule')" :disabled="!!draft || !!pending" @click="openItem('rule', -1)"><Ico name="plus" :size="16" /></Btn>
    </div>
    <EditorModal
      :open="!!draft"
      :title="draftTitle"
      :content="draft?.text ?? ''"
      controlled keep-open-on-save
      @update:content="setDraftText"
      @save="saveDraft"
      @close="draft = null; error = ''"
    >
      <template #before>
        <div v-if="draft" class="sub-dns-draft">
          <template v-if="draft.kind === 'server' && draft.index === -1">
            <Field :label="$t('type')"><Select :disabled="!parsed" :model-value="parsed?.type ?? ''" @change="newServerType"><option v-for="t in dnsServerTypeOptions" :key="t.value" :value="t.value">{{ t.title }}</option></Select></Field>
          </template>
          <template v-if="draft.kind === 'server' && parsed && dnsServerTypesWithAddress.includes(parsed.type)">
            <div class="field-grid">
              <Field :label="$t('dns.server')"><input class="input mono" :value="parsed.server ?? ''" @change="patchText('server', ($event.target as HTMLInputElement).value)" /></Field>
              <Field :label="$t('in.port')"><input class="input mono" type="number" min="1" max="65535" :value="parsed.server_port ?? ''" @change="patchPort(($event.target as HTMLInputElement).value)" /></Field>
            </div>
          </template>
          <template v-if="draft.kind === 'rule' && parsed">
            <Field v-if="!parsed.action || parsed.action === 'route' || parsed.action === 'evaluate'" :label="$t('dns.server')"><Select :model-value="parsed.server ?? ''" @change="patchText('server', $event)"><option value="">—</option><option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</option></Select></Field>
            <ChipSelect v-if="parsed.type !== 'logical'" :model-value="refs(parsed.rule_set)" :options="ruleOptions" label="rule_set" :placeholder="$t('ui.selectHint')" @update:model-value="patchSets" />
          </template>
          <p v-if="draft.kind === 'rule' && draft.index === -1" class="dns-hint">{{ $t('subscriptionDns.ruleHelp') }}</p>
        </div>
      </template>
      <template #after>
        <div v-if="error" role="alert" class="dns-error dns-draft-error">{{ $t('subscriptionDns.invalid') }}: {{ error }}</div>
      </template>
    </EditorModal>
    <DeleteConfirm :open="!!pending" @close="pending = null" @confirm="remove" />
    <div v-if="error && !draft" role="alert" class="dns-error">{{ $t('subscriptionDns.invalid') }}: {{ error }}</div>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Field from '@/components/ui/Field.vue'
import Select from '@/components/ui/Select.vue'
import Btn from '@/components/ui/Btn.vue'
import Ico from '@/components/ui/Ico.vue'
import EditorModal from '@/layouts/drawers/EditorModal.vue'
import DeleteConfirm from '@/components/ui/DeleteConfirm.vue'
import { moveItem } from '@/plugins/useReorder'
import ChipSelect from '@/components/ui/ChipSelect.vue'
import { createDnsServer, dnsServerTypeOptions, dnsServerTypesWithAddress } from '@/types/dns'
import { editDns, isObject, resolverTag, setResolver, refs, dnsServerTag, type Catalog } from '@/plugins/subscriptionDns'
import { ruleSetTags } from '@/plugins/subscriptionRuleSets'
const props = defineProps<{ config: Record<string, any>; catalog: Catalog; catalogOptions: readonly { title: string; value: string }[] }>()
const emit = defineEmits<{ 'update:config': [value: Record<string, any>] }>()
const { t: translate } = useI18n()
const dns = computed(() => props.config.dns ?? {})
const servers = computed<any[]>(() => Array.isArray(dns.value.servers) ? dns.value.servers : [])
const rules = computed<any[]>(() => Array.isArray(dns.value.rules) ? dns.value.rules : [])
const tags = computed<string[]>(() => servers.value.map(dnsServerTag))
const resolver = computed(() => resolverTag(props.config.default_domain_resolver))
const strategies = ['prefer_ipv4', 'prefer_ipv6', 'ipv4_only', 'ipv6_only']
const error = ref('')
const draft = ref<{ kind: 'server'|'rule'; index: number; text: string } | null>(null)
const pending = ref<{ kind: 'server'|'rule'; index: number } | null>(null)
const parsed = computed(() => { try { const v = JSON.parse(draft.value?.text ?? ''); return isObject(v) ? v : null } catch { return null } })
const draftTitle = computed(() => {
  if (!draft.value) return ''
  const { kind, index } = draft.value
  if (index === -1) return translate(kind === 'server' ? 'ui.dnssNew' : 'ui.dnsrNew') + ' · JSON'
  const name = kind === 'server' ? dnsServerTag(servers.value[index] ?? {}, index) : translate('objects.dnsrule') + ' #' + (index + 1)
  return translate('actions.edit') + ' · ' + name + ' · JSON'
})
const ruleOptions = computed(() => {
  const available = new Set([...props.catalog.map(r => r.tag), ...(props.config.rule_set ?? []).flatMap((r: any) => ruleSetTags(r.tag))].filter(t => typeof t === 'string' && t))
  const result = props.catalogOptions.filter(o => available.delete(o.value))
  return [...result, ...Array.from(available, value => ({ title: value, value }))]
})
const commit = (fn: (next: Record<string, any>) => void) => {
  try { emit('update:config', editDns(props.config, fn, props.catalog)); error.value = ''; return true }
  catch (e) { error.value = String(e); return false }
}
const changeFinal = (tag: string) => { commit(t => { if (tag) t.dns.final = tag; else delete t.dns.final }) }
const changeResolver = (tag: string) => { commit(t => setResolver(t, tag)) }
const changeStrategy = (strategy: string) => { commit(t => { if (strategy) t.dns.strategy = strategy; else delete t.dns.strategy }) }
const setDraftText = (text: string) => { if (draft.value) draft.value.text = text }
const writeDraft = (v: any) => setDraftText(JSON.stringify(v, null, 2))
const newTag = () => { let i = 1; while (tags.value.includes('dns-' + i)) i++; return 'dns-' + i }
const openItem = (kind: 'server'|'rule', index: number) => {
  error.value = ''; pending.value = null
  const obj = kind === 'server' ? (index < 0 ? createDnsServer('local', { tag: newTag() }) : servers.value[index])
    : (index < 0 ? { action: 'route', server: '' } : rules.value[index])
  draft.value = { kind, index, text: JSON.stringify(obj, null, 2) }
}
const openNewServer = () => openItem('server', -1)
const newServerType = (type: string) => {
  if (!parsed.value || parsed.value.type === type || !dnsServerTypeOptions.some(o => o.value === type)) return
  writeDraft(createDnsServer(type, { tag: parsed.value.tag || newTag() }))
}
const patchText = (key: string, value: string) => { if (parsed.value) writeDraft({ ...parsed.value, [key]: value }) }
const patchPort = (value: string) => { if (!parsed.value) return; const next = { ...parsed.value }; if (value === '') delete next.server_port; else next.server_port = Number(value); writeDraft(next) }
const patchSets = (value: string[]) => { if (!parsed.value) return; const next = { ...parsed.value }; if (value.length) next.rule_set = value; else delete next.rule_set; writeDraft(next) }
const ruleSummary = (r: any) => Object.entries(r).filter(([k]) => !['action', 'server'].includes(k)).map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(' · ')
const move = (i: number, delta: number) => {
  if (draft.value || pending.value || i < 0 || i + delta < 0 || i + delta >= rules.value.length || i >= rules.value.length) return
  commit(t => moveItem(t.dns.rules, i, delta))
}
const askRemove = (kind: 'server'|'rule', index: number) => { draft.value = null; error.value = ''; pending.value = { kind, index } }
const remove = () => {
  const p = pending.value
  if (!p) return
  commit(t => {
    if (p.kind === 'server') {
      const servers = t.dns.servers
      // Preserve effective tags before their indices change.
      for (let i = p.index + 1; i < servers.length; i++) {
        if (servers[i].tag === undefined || servers[i].tag === '') {
          servers[i].tag = dnsServerTag(servers[i], i)
        }
      }
      servers.splice(p.index, 1)
    } else {
      t.dns.rules.splice(p.index, 1)
    }
  })
  pending.value = null
}
const saveDraft = () => {
  const d = draft.value; if (!d) return
  try {
    const value: unknown = JSON.parse(d.text)
    if (!isObject(value)) throw new Error(translate('subscriptionDns.invalidJson'))
    if (commit(t => {
      const key = d.kind === 'server' ? 'servers' : 'rules'
      const items = t.dns[key] ??= []
      if (d.index === -1) items.push(value); else items[d.index] = value
    })) draft.value = null
  } catch (e) { error.value = String(e) }
}
// Loading a different template invalidates an item draft's index.
watch(() => props.config, () => { draft.value = null; pending.value = null })
</script>
<style scoped>
.sub-dns { display:flex; flex-direction:column; gap:5px; }
.dns-basics { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:6px; align-items:end; }
.dns-list-wrap { position:relative; min-width:0; }
.dns-list { border-top:1px solid var(--line); min-width:0; }
.dns-add { position:absolute; inset-inline-end:0; top:1px; }
.dns-list summary { display:flex; align-items:center; gap:6px; cursor:pointer; min-height:34px; list-style:none; font-size:13px; padding-inline-end:36px; }
.dns-list summary::-webkit-details-marker { display:none; }
.dns-chevron { transition:transform .15s; transform:rotate(-90deg); }
.dns-list[open] > summary .dns-chevron { transform:rotate(0); }
.dns-count { color:var(--text-3); font-size:11px; }
.dns-list-body { display:flex; flex-direction:column; gap:0; }

.sub-dns-row { display:flex; align-items:center; gap:2px; min-height:34px; padding:0; border-bottom:1px solid var(--line); }
.sub-dns-desc { flex:1 1 0; min-width:0; display:flex; align-items:center; min-height:40px; margin-inline-end:12px; gap:6px; background:transparent; border:0; text-align:start; color:inherit; cursor:pointer; padding:4px 0; font:inherit; }
.sub-dns-desc strong { font-size:13px; font-weight:600; flex:0 0 auto; min-width:0; max-width:65%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.sub-dns-desc span { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:12px; color:var(--text-2); }
.sub-dns-desc .dns-num { flex:0 0 auto; }
.dns-row-edit { flex:0 0 auto; margin-inline-end:4px; color:var(--text-2); }
.dns-icon { flex:none; }
.sub-dns-desc:hover:not(:disabled) { background:var(--surface-3); }
.sub-dns-desc .mono { flex:1 1 0; }
.dns-delete { color:var(--rose); }
.dns-icon:focus-visible, .sub-dns-desc:focus-visible, summary:focus-visible { outline:2px solid var(--brand); outline-offset:1px; }
.sub-dns-draft { padding:12px; display:flex; flex-direction:column; gap:6px; }
.dns-hint { font-size:12px; color:var(--text-3); margin:0; }
.dns-error { color:var(--rose); overflow-wrap:anywhere; font-size:12px; }
.dns-draft-error { padding:0 12px 12px; }
@media(max-width:640px) {
 .dns-basics { grid-template-columns:repeat(2,minmax(0,1fr)); }
 .sub-dns-desc { flex-wrap:wrap; gap:0 5px; }
 .sub-dns-desc strong { max-width:100%; }
 .sub-dns-desc .mono { flex:1 1 75%; }
 .dns-row-edit { align-self:center; }
}
</style>
