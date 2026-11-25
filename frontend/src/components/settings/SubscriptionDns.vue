<template>
  <div class="sub-dns">
    <div class="dns-basics">
      <Field :label="$t('subscriptionDns.final')" :mb="0">
        <Select :disabled="!!draft || !!pending" :model-value="dns.final ?? ''" @change="changeFinal">
          <option value="">{{ $t('subscriptionDns.first') }}</option>
          <option v-if="dns.final && !tags.includes(dns.final)" :value="dns.final">{{ dns.final }} (!)</option>
          <option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</option>
        </Select>
      </Field>
      <Field :label="$t('subscriptionDns.resolver')" :mb="0">
        <Select :disabled="!!draft || !!pending" :model-value="resolver" @change="changeResolver">
          <option value="">{{ $t('subscriptionDns.automatic') }}</option>
          <option v-if="resolver && !tags.includes(resolver)" :value="resolver">{{ resolver }} (!)</option>
          <option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</option>
        </Select>
      </Field>
      <Field :label="$t('subscriptionDns.strategy')" :mb="0">
        <Select :disabled="!!draft || !!pending" :model-value="dns.strategy ?? ''" @change="changeStrategy">
          <option value="">{{ $t('ui.none') }}</option>
          <option v-if="dns.strategy && !strategies.includes(dns.strategy)" :value="dns.strategy">{{ dns.strategy }}</option>
          <option v-for="strategy in strategies" :key="strategy" :value="strategy">{{ strategy }}</option>
        </Select>
      </Field>
      <div class="dns-toolbar"><Btn sm :disabled="!!draft || !!pending" @click="openItem('advanced', -1)"><Ico name="edit" :size="14" /> {{ $t('subscriptionDns.advanced') }}</Btn></div>
    </div>
    <div class="dns-list-wrap">
    <details class="dns-list" open>
      <summary><Ico name="chevronDown" :size="13" class="dns-chevron" /><strong>{{ $t('ui.dnsServers') }}</strong><span class="dns-count">{{ servers.length }}</span></summary>
      <div class="dns-list-body">
        <div v-for="(server, i) in servers" :key="i" class="sub-dns-row">
          <button type="button" class="sub-dns-desc" :disabled="!!draft || !!pending" :title="`${server.tag || '#' + (i + 1)} · ${server.type || $t('subscriptionDns.legacy')} · ${server.server || '—'}`" @click="openItem('server', i)">
            <strong>{{ server.tag || '#' + (i + 1) }}</strong><span class="mono">{{ server.type || $t('subscriptionDns.legacy') }}<template v-if="server.server"> · {{ server.server }}<template v-if="server.server_port">:{{ server.server_port }}</template></template></span>
            <Ico name="edit" :size="13" class="dns-row-edit" />
          </button>
          <button type="button" class="dns-icon dns-delete" :aria-label="$t('actions.del')" :title="$t('actions.del')" :disabled="!!draft || !!pending" @click="askRemove('server', i)"><Ico name="trash" :size="14" /></button>
        </div>
      </div>
    </details>
      <button type="button" class="dns-icon dns-add" :aria-label="$t('ui.addDnsServer')" :title="$t('ui.addDnsServer')" :disabled="!!draft || !!pending" @click="openNewServer"><Ico name="plus" :size="16" /></button>
    </div>
    <div class="dns-list-wrap">
    <details class="dns-list" open>
      <summary><Ico name="chevronDown" :size="13" class="dns-chevron" /><strong>{{ $t('ui.dnsRules') }}</strong><span class="dns-count">{{ rules.length }}</span></summary>
      <div class="dns-list-body">
        <div v-for="(rule, i) in rules" :key="i" class="sub-dns-row">
          <button type="button" class="sub-dns-desc" :title="ruleSummary(rule)" :disabled="!!draft || !!pending" @click="openItem('rule', i)">
            <span class="dns-num">{{ i + 1 }}.</span><strong>{{ rule.action || 'route' }}<template v-if="rule.server"> → {{ rule.server }}</template></strong><span class="mono">{{ ruleSummary(rule) }}</span>
            <Ico name="edit" :size="13" class="dns-row-edit" />
          </button>
          <button type="button" class="dns-icon" :aria-label="$t('subscriptionDns.up') + ' ' + (i + 1)" :title="$t('subscriptionDns.up')" :disabled="!!draft || !!pending || i === 0" @click="move(i, -1)"><Ico name="chevronDown" :size="14" style="transform:rotate(180deg)" /></button>
          <button type="button" class="dns-icon" :aria-label="$t('subscriptionDns.down') + ' ' + (i + 1)" :title="$t('subscriptionDns.down')" :disabled="!!draft || !!pending || i === rules.length - 1" @click="move(i, 1)"><Ico name="chevronDown" :size="14" /></button>
          <button type="button" class="dns-icon dns-delete" :aria-label="$t('actions.del')" :title="$t('actions.del')" :disabled="!!draft || !!pending" @click="askRemove('rule', i)"><Ico name="trash" :size="14" /></button>
        </div>
      </div>
    </details>
      <button type="button" class="dns-icon dns-add" :aria-label="$t('ui.addDnsRule')" :title="$t('ui.addDnsRule')" :disabled="!!draft || !!pending" @click="openItem('rule', -1)"><Ico name="plus" :size="16" /></button>
    </div>
    <Modal :open="!!draft" :title="(draft?.kind === 'server' ? $t('subscriptionDns.editServer') : draft?.kind === 'rule' ? $t('subscriptionDns.editRule') : $t('ui.dnsOpt')) + ' · JSON'" :width="680" @close="draft = null; error = ''">
      <div v-if="draft" class="sub-dns-draft">
      <template v-if="draft.kind === 'server' && draft.index === -1">
        <Field :label="$t('type')"><Select :model-value="draftType" @change="newServerType"><option v-for="t in types" :key="t" :value="t">{{ t }}</option></Select></Field>
      </template>
      <template v-if="draft.kind === 'server' && parsed && networkTypes.includes(parsed.type)">
        <div class="field-grid">
          <Field :label="$t('dns.server')"><input class="input mono" :value="parsed.server ?? ''" @change="patchText('server', ($event.target as HTMLInputElement).value)" /></Field>
          <Field :label="$t('in.port')"><input class="input mono" type="number" min="1" max="65535" :value="parsed.server_port ?? ''" @change="patchPort(($event.target as HTMLInputElement).value)" /></Field>
        </div>
      </template>
      <template v-if="draft.kind === 'rule' && parsed && (parsed.action === 'route' || parsed.action === 'evaluate')">
        <Field :label="$t('dns.server')"><Select :model-value="parsed.server ?? ''" @change="patchText('server', $event)"><option value="">—</option><option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</option></Select></Field>
        <ChipSelect v-if="parsed.type !== 'logical'" :model-value="refs(parsed.rule_set)" :options="ruleOptions" label="rule_set" :placeholder="$t('ui.selectHint')" @update:model-value="patchSets" />
      </template>
      <p v-if="draft.kind === 'rule' && draft.index === -1" class="dns-hint">{{ $t('subscriptionDns.ruleHelp') }}</p>
      <textarea v-model="draft.text" class="input mono" rows="9" spellcheck="false" style="width:100%; resize:vertical;" />
        <div v-if="error" role="alert" class="dns-error">{{ $t('subscriptionDns.invalid') }}: {{ error }}</div>
      </div>
      <template #footer><Btn sm @click="draft = null; error = ''">{{ $t('ui.cancel') }}</Btn><Btn sm variant="primary" @click="saveDraft">{{ $t('actions.save') }}</Btn></template>
    </Modal>
    <div v-if="pending" class="sub-dns-head"><span>{{ $t(pending.kind === 'server' ? 'subscriptionDns.removeServer' : 'subscriptionDns.removeRule') }}</span><Btn sm @click="pending = null">{{ $t('no') }}</Btn><Btn sm @click="remove">{{ $t('yes') }}</Btn></div>
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
import Modal from '@/components/ui/Modal.vue'
import ChipSelect from '@/components/ui/ChipSelect.vue'
import { DnsTypes, createDnsServer } from '@/types/dns'
import { editDns, isObject, resolverTag, setResolver, refs, type Catalog } from '@/plugins/subscriptionDns'
const props = defineProps<{ config: Record<string, any>; catalog: Catalog }>()
const emit = defineEmits<{ 'update:config': [value: Record<string, any>] }>()
const { t: translate } = useI18n()
const dns = computed(() => props.config.dns ?? {})
const servers = computed<any[]>(() => Array.isArray(dns.value.servers) ? dns.value.servers : [])
const rules = computed<any[]>(() => Array.isArray(dns.value.rules) ? dns.value.rules : [])
const tags = computed<string[]>(() => servers.value.map(s => s.tag).filter(t => typeof t === 'string' && t))
const resolver = computed(() => resolverTag(props.config.default_domain_resolver))
const strategies = ['prefer_ipv4', 'prefer_ipv6', 'ipv4_only', 'ipv6_only']
const types = Object.values(DnsTypes)
const networkTypes = ['tcp', 'udp', 'tls', 'quic', 'https', 'h3']
const error = ref('')
const draft = ref<{ kind: 'server'|'rule'|'advanced'; index: number; text: string } | null>(null)
const pending = ref<{ kind: 'server'|'rule'; index: number } | null>(null)
const draftType = ref('local')
const parsed = computed(() => { try { const v = JSON.parse(draft.value?.text ?? ''); return isObject(v) ? v : null } catch { return null } })
const ruleOptions = computed(() => [...new Set([...props.catalog.map(r => r.tag), ...(props.config.rule_set ?? []).map((r: any) => r.tag)])].filter(t => typeof t === 'string').map(t => ({ title: t, value: t })))
const commit = (fn: (next: Record<string, any>) => void) => {
  try { emit('update:config', editDns(props.config, fn, props.catalog)); error.value = ''; return true }
  catch (e) { error.value = String(e); return false }
}
const changeFinal = (tag: string) => { commit(t => { if (tag) t.dns.final = tag; else delete t.dns.final }) }
const changeResolver = (tag: string) => { commit(t => setResolver(t, tag)) }
const changeStrategy = (strategy: string) => { commit(t => { if (strategy) t.dns.strategy = strategy; else delete t.dns.strategy }) }
const writeDraft = (v: any) => { if (draft.value) draft.value.text = JSON.stringify(v, null, 2) }
const newTag = () => { let i = 1; while (tags.value.includes('dns-' + i)) i++; return 'dns-' + i }
const openItem = (kind: 'server'|'rule'|'advanced', index: number) => {
  error.value = ''; pending.value = null
  const obj = kind === 'advanced' ? { dns: dns.value, ...(props.config.default_domain_resolver !== undefined ? { default_domain_resolver: props.config.default_domain_resolver } : {}) }
    : kind === 'server' ? (index < 0 ? createDnsServer('local', { tag: newTag() }) : servers.value[index])
    : (index < 0 ? { action: 'route', server: '' } : rules.value[index])
  draft.value = { kind, index, text: JSON.stringify(obj, null, 2) }
}
const openNewServer = () => { draftType.value = 'local'; openItem('server', -1) }
const newServerType = (type: string) => { draftType.value = type; writeDraft(createDnsServer(type, { tag: parsed.value?.tag || newTag() })) }
const patchText = (key: string, value: string) => { if (parsed.value) writeDraft({ ...parsed.value, [key]: value }) }
const patchPort = (value: string) => { if (!parsed.value) return; const next = { ...parsed.value }; if (value === '') delete next.server_port; else next.server_port = Number(value); writeDraft(next) }
const patchSets = (value: string[]) => { if (!parsed.value) return; const next = { ...parsed.value }; if (value.length) next.rule_set = value; else delete next.rule_set; writeDraft(next) }
const ruleSummary = (r: any) => Object.entries(r).filter(([k]) => !['action', 'server'].includes(k)).map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(' · ')
const move = (i: number, delta: number) => { if (draft.value) return; commit(t => { const a = t.dns.rules; const j = i + delta; if (j >= 0 && j < a.length) [a[i], a[j]] = [a[j], a[i]] }) }
const askRemove = (kind: 'server'|'rule', index: number) => { draft.value = null; error.value = ''; pending.value = { kind, index } }
const remove = () => { const p = pending.value; if (!p) return; if (commit(t => { (p.kind === 'server' ? t.dns.servers : t.dns.rules).splice(p.index, 1) })) pending.value = null }
const saveDraft = () => {
  const d = draft.value; if (!d) return
  try {
    const value: unknown = JSON.parse(d.text)
    if (!isObject(value)) throw new Error(translate('subscriptionDns.invalidJson'))
    if (commit(t => {
      if (d.kind === 'advanced') {
        if (!isObject(value.dns) || Object.keys(value).some(k => !['dns', 'default_domain_resolver'].includes(k))) throw new Error(translate('subscriptionDns.advancedShape'))
        t.dns = value.dns
        if ('default_domain_resolver' in value) t.default_domain_resolver = value.default_domain_resolver; else delete t.default_domain_resolver
      } else {
        const key = d.kind === 'server' ? 'servers' : 'rules'
        const items = t.dns[key] ??= []
        if (d.index === -1) items.push(value); else items[d.index] = value
      }
    })) draft.value = null
  } catch (e) { error.value = String(e) }
}
// Loading a different template invalidates an item draft's index.
watch(() => props.config, () => { draft.value = null; pending.value = null })
</script>
<style scoped>
.sub-dns { display:flex; flex-direction:column; gap:5px; }
.dns-basics { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)) auto; gap:6px; align-items:end; }
.dns-toolbar { display:flex; justify-content:flex-end; }
.dns-list-wrap { position:relative; min-width:0; }
.dns-list { border-top:1px solid var(--line); min-width:0; }
.dns-add { position:absolute; right:0; top:2px; }
.dns-list summary { display:flex; align-items:center; gap:6px; cursor:pointer; min-height:30px; list-style:none; font-size:12px; padding-right:34px; }
.dns-list summary::-webkit-details-marker { display:none; }
.dns-chevron { transition:transform .15s; transform:rotate(-90deg); }
.dns-list[open] > summary .dns-chevron { transform:rotate(0); }
.dns-count { color:var(--text-3); font-size:11px; }
.dns-list-body { display:flex; flex-direction:column; gap:0; }

.sub-dns-row { display:flex; align-items:center; gap:3px; min-height:30px; padding:1px 0; border-bottom:1px solid var(--line); }
.sub-dns-desc { flex:1 1 0; min-width:0; display:flex; align-items:baseline; gap:6px; background:transparent; border:0; text-align:left; color:inherit; cursor:pointer; padding:4px 0; font:inherit; }
.sub-dns-desc strong { font-size:12px; flex:0 1 auto; min-width:0; max-width:65%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.sub-dns-desc span { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:11px; color:var(--text-3); }
.sub-dns-desc .dns-num { flex:0 0 auto; }
.dns-row-edit { flex:0 0 auto; margin-left:auto; color:var(--text-2); }
.dns-icon { display:inline-flex; justify-content:center; align-items:center; flex:0 0 28px; height:28px; padding:0; border:0; border-radius:4px; background:transparent; color:var(--text-2); cursor:pointer; }
.dns-icon:hover:not(:disabled), .sub-dns-desc:hover:not(:disabled) { background:var(--surface-3); }
.dns-icon:disabled { opacity:.35; cursor:default; }
.dns-delete { color:var(--rose); }
.dns-icon:focus-visible, .sub-dns-desc:focus-visible, summary:focus-visible { outline:2px solid var(--emerald); outline-offset:1px; }
.sub-dns-head { display:flex; align-items:center; justify-content:flex-end; gap:6px; flex-wrap:wrap; }
.sub-dns-head span { flex:1; }
.sub-dns-draft { padding:12px; display:flex; flex-direction:column; gap:6px; }
.dns-hint { font-size:12px; color:var(--text-3); margin:0; }
.dns-error { color:var(--rose); overflow-wrap:anywhere; font-size:12px; }
@media(max-width:640px) {
 .dns-basics { grid-template-columns:repeat(2,minmax(0,1fr)); }
 .dns-toolbar { justify-content:flex-end; }
 .sub-dns-desc { flex-wrap:wrap; gap:0 5px; }
 .sub-dns-desc strong { max-width:100%; }
 .sub-dns-desc .mono { flex:1 1 75%; }
 .dns-row-edit { align-self:center; }
 .dns-icon { flex-basis:32px; height:32px; }
}
</style>
