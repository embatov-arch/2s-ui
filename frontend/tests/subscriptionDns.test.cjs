const { test } = require('node:test')
const assert = require('node:assert/strict')
const path = require('node:path')
const { editDns, validateDns, parseTemplate, setResolver, toggleDns, copy } = require(path.join(process.env.DNS_TEST_BUILD, 'subscriptionDns.js'))
const seed = () => ({dns:{servers:[{tag:'cloudflare',type:'https',server:'1.1.1.1',path:'/dns-query',tls:{server_name:'cloudflare-dns.com'}},{tag:'local',type:'local'}],final:'cloudflare',rules:[{action:'evaluate',server:'cloudflare',tag:'cf'},{match_response:'cf',action:'respond'}]},default_domain_resolver:{server:'local',strategy:'ipv4_only',timeout:'3s',future:true},rules:[{action:'sniff'},{domain:['example.org'],outbound:'direct',action:'route'}]})
const cat=[{tag:'geosite-private',type:'remote',url:'https://example.org/r.srs'}]
test('actual DNS tags and DoH attributes',()=>{let t=seed();let out=editDns(t,x=>{x.dns.servers[0].server='9.9.9.9'});assert.equal(out.dns.servers[0].server,'9.9.9.9');assert.equal(t.dns.servers[0].server,'1.1.1.1');assert.deepEqual(out.dns.servers[0].tls,t.dns.servers[0].tls);assert.deepEqual(out.rules,t.rules)})
test('object resolver options and DNS final independent',()=>{let t=seed();let out=editDns(t,x=>setResolver(x,'cloudflare'));assert.deepEqual(out.default_domain_resolver,{...t.default_domain_resolver,server:'cloudflare'});assert.equal(out.dns.final,t.dns.final)})
test('changing final does not change resolver',()=>{const t=seed();assert.deepEqual(editDns(t,x=>{x.dns.final='local'}).default_domain_resolver,t.default_domain_resolver)})
test('missing tag refused atomically',()=>{let t=seed(), before=copy(t);assert.throws(()=>editDns(t,x=>setResolver(x,'missing')));assert.deepEqual(t,before)})
test('duplicate tag rejected',()=>assert.throws(()=>editDns(seed(),x=>x.dns.servers.push({tag:'local',type:'local'}))))
test('cannot remove a referenced server',()=>assert.throws(()=>editDns(seed(),x=>x.dns.servers.pop())))
test('append missing rule-set, preserve ordered rules',()=>{let t=seed();let out=editDns(t,x=>x.dns.rules.push({action:'route',server:'local',rule_set:'geosite-private'}),cat);assert.deepEqual(out.rule_set,cat);assert.deepEqual(out.dns.rules.slice(0,2),t.dns.rules)})
test('cannot reverse evaluate and respond',()=>assert.throws(()=>editDns(seed(),x=>x.dns.rules.reverse()),/preceding evaluate/))
test('cannot reference later named evaluate within logical rule',()=>{let t=seed();t.dns.rules.unshift({type:'logical',mode:'and',rules:[{match_response:'cf'}],action:'route',server:'local'});assert.throws(()=>validateDns(t),/preceding evaluate/)})
test('missing first DNS rule array created explicitly',()=>{let t=seed();delete t.dns.rules;let out=editDns(t,x=>(x.dns.rules??=[]).push({action:'route',server:'local',domain:['example.org']}));assert.equal(out.dns.rules.length,1)})
test('template shape refused without losing original',()=>{for(let raw of ['null','[]','{"dns":{"servers":{}}}','{"dns":{"rules":[null]}}'])assert.throws(()=>parseTemplate(raw));assert.deepEqual(parseTemplate(''),{})})
test('disable DNS blocks explicit resolver dependencies',()=>assert.throws(()=>toggleDns(seed(),false,{}),/default_domain_resolver/))
test('disable removes only exact hijack, not reject or unrelated rule',()=>{let t=seed();delete t.default_domain_resolver;t.rules.unshift({protocol:['dns'],action:'hijack-dns'},{protocol:'dns',action:'reject'});let out=toggleDns(t,false,{});assert.equal(out.dns,undefined);assert.deepEqual(out.rules,t.rules.slice(1))})
test('custom hijack dependency refused',()=>{let t=seed();delete t.default_domain_resolver;t.rules.unshift({protocol:'dns',inbound:['tun'],action:'hijack-dns'});assert.throws(()=>toggleDns(t,false,{}),/custom rules/)})
test('enable restores DNS interception and preserves existing routing', () => {
  const d = { servers: [{ tag: 'local', type: 'local' }] }
  const sniff = { action: 'sniff' }
  const hijack = { protocol: 'dns', action: 'hijack-dns' }

  assert.deepEqual(toggleDns({rules: []}, true, d).rules, [sniff, hijack])

  const rules = seed().rules
  const expected = [rules[0], hijack, ...rules.slice(1)]
  assert.deepEqual(toggleDns({rules}, true, d).rules, expected)

  const enabled = toggleDns({rules}, true, d)
  assert.deepEqual(toggleDns(enabled, true, d), enabled)

  const custom = [{port: 53, action: 'hijack-dns'}]
  assert.deepEqual(toggleDns({rules: custom}, true, d).rules, custom)

  const x = toggleDns({}, true, d)
  assert.deepEqual(x.rules.map(r => r.action), ['sniff', 'hijack-dns', 'route', 'route'])
  x.dns.servers[0].tag = 'other'
  assert.equal(d.servers[0].tag, 'local')
})
test('unknown future DNS options roundtrip unchanged',()=>{let t=seed();t.dns.rules[0].dns_server_address=['1.1.1.1'];t.dns.servers[0].future={a:1};const next=editDns(t,x=>{x.dns.strategy='ipv4_only'});assert.deepEqual(next.dns.rules,t.dns.rules);assert.deepEqual(next.dns.servers,t.dns.servers)})

test('named evaluate cannot be duplicated',()=>assert.throws(()=>editDns(seed(),x=>x.dns.rules.splice(1,0,{action:'evaluate',server:'local',tag:'cf'})),/duplicate evaluate/))
test('respond without a tag needs an anonymous evaluate',()=>assert.throws(()=>editDns(seed(),x=>{delete x.dns.rules[1].match_response}),/untagged evaluate/))
test('anonymous evaluate and respond remain valid',()=>{const t=seed();delete t.dns.rules[0].tag;t.dns.rules[1].match_response=true;assert.doesNotThrow(()=>validateDns(t))})
test('logical respond cannot select a named response',()=>assert.throws(()=>editDns(seed(),x=>{x.dns.rules[1]={type:'logical',mode:'and',rules:[{match_response:'cf'}],action:'respond'}}),/logical respond/))
test('explicit resolver cycle refused',()=>assert.throws(()=>editDns(seed(),x=>{x.dns.servers[0].domain_resolver='local';x.dns.servers[1].domain_resolver={server:'cloudflare'}}),/circular/))
test('duplicate rule-set tags refused',()=>assert.throws(()=>editDns(seed(),x=>{x.rule_set=[{tag:'a'},{tag:'a'}]}),/duplicate tag/))
test('malformed refs refused without silently dropping members',()=>assert.throws(()=>editDns(seed(),x=>{x.dns.rules[1].rule_set=[1]}),/expected string/))
test('evaluate fakeip refused',()=>assert.throws(()=>editDns(seed(),x=>{x.dns.servers[0].type='fakeip'}),/fakeip/))


test('implicit DNS tags follow native array indices', () => {
  const t = {dns:{servers:[{type:'local'}, {type:'local'}], final:'1'}}
  assert.doesNotThrow(() => validateDns(t))
  assert.throws(() => editDns(t, x => { x.dns.final = 'missing' }))
  assert.throws(() => editDns(t, x => { x.dns.servers[1].tag = '0' }))
  assert.deepEqual(editDns(t, x => { x.dns.strategy = 'ipv4_only' }).dns.servers, t.dns.servers)
})

test('grouped rule-set tags are accepted, and overlapping tags rejected', () => {
  const t = seed()
  const group = { type: 'remote', tag: ['one', 'two'], url: 'https://example.org/{tag}.srs' }
  t.rule_set = [group]
  t.dns.rules.push({ action: 'route', server: 'local', rule_set: ['one', 'two'] })
  assert.doesNotThrow(() => validateDns(t))
  assert.deepEqual(editDns(t, x => { x.dns.strategy = 'ipv4_only' }).rule_set, [group])
  assert.throws(() => editDns(t, x => x.rule_set.push({ tag: 'two' })), /duplicate tag two/)
})
test('DNS address and search-domain dependencies survive logical DNS and route rules', () => {
  for (const field of ['dns_server_address', 'dns_search_domain']) {
    const value = field === 'dns_server_address' ? ['192.0.2.1/32'] : ['example.org']
    for (const location of ['dns', 'route']) {
      const t = { dns: { servers: [{ tag: 'keep', type: 'local' }, { tag: 'remove', type: 'local' }] } }
      const logical = { type: 'logical', mode: 'and', rules: [{ [field]: { remove: value } }], action: 'route' }
      if (location === 'dns') t.dns.rules = [{ ...logical, server: 'keep' }]
      else t.rules = [{ ...logical, outbound: 'direct' }]
      assert.doesNotThrow(() => validateDns(t))
      assert.throws(() => editDns(t, x => x.dns.servers.pop()), /unknown DNS tag remove/)
      if (location === 'route') assert.throws(() => toggleDns(t, false, {}), /custom rules/)
    }
  }
})
test('route preferred_by still refers to outbounds, not DNS servers', () => {
  const t = seed()
  t.rules.push({ preferred_by: ['wireguard'], action: 'route', outbound: 'direct' })
  assert.doesNotThrow(() => validateDns(t))
})
