package util

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/sagernet/sing-box/option"
)

// SubscriptionDNSTag follows sing-box's implicit DNS transport tags.
func SubscriptionDNSTag(server map[string]interface{}, index int) string {
	if tag := AsString(server["tag"]); tag != "" {
		return tag
	}
	return strconv.Itoa(index)
}

// ValidateSubscriptionDNS checks the template's DNS structure and explicit references.
// It does not replace sing-box's native config check, and does not rewrite the template.
func ValidateSubscriptionDNS(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var t map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return fmt.Errorf("subJsonExt: %w", err)
	}
	if t == nil {
		return fmt.Errorf("subJsonExt: expected object")
	}
	// Match the shapes the subscription editor reads, without rewriting values.
	for _, key := range []string{"dns", "log", "experimental"} {
		if value, present := t[key]; present {
			if _, ok := value.(map[string]interface{}); !ok {
				return fmt.Errorf("%s: expected object", key)
			}
		}
	}
	for _, key := range []string{"rules", "rule_set", "inbounds"} {
		if value, present := t[key]; present {
			items, ok := value.([]interface{})
			if !ok {
				return fmt.Errorf("%s: expected array", key)
			}
			for _, item := range items {
				if _, ok := item.(map[string]interface{}); !ok {
					return fmt.Errorf("%s: expected object entries", key)
				}
			}
		}
	}
	d, exists := t["dns"]
	if !exists {
		if v, ok := t["default_domain_resolver"]; ok && v != "" {
			return fmt.Errorf("default_domain_resolver requires DNS")
		}
		return nil
	}
	dns := d.(map[string]interface{})
	list := func(v interface{}, path string) ([]interface{}, error) {
		if v == nil {
			return nil, nil
		}
		a, ok := v.([]interface{})
		if !ok {
			return nil, fmt.Errorf("%s: expected array", path)
		}
		for _, item := range a {
			if _, ok := item.(map[string]interface{}); !ok {
				return nil, fmt.Errorf("%s: expected object entries", path)
			}
		}
		return a, nil
	}
	for _, key := range []string{"servers", "rules"} {
		if value, present := dns[key]; present && value == nil {
			return fmt.Errorf("dns.%s: expected array", key)
		}
	}
	servers, err := list(dns["servers"], "dns.servers")
	if err != nil {
		return err
	}
	tags := map[string]bool{}
	for i, item := range servers {
		s := item.(map[string]interface{})
		tag, ok := s["tag"].(string)
		if _, exists := s["tag"]; exists && !ok {
			return fmt.Errorf("dns.servers[%d].tag: expected string", i)
		}
		tag = SubscriptionDNSTag(s, i)
		if tags[tag] {
			return fmt.Errorf("dns.servers: duplicate tag %q", tag)
		}
		tags[tag] = true
	}
	checkTag := func(v interface{}, path string) error {
		if v == nil || v == "" {
			return nil
		}
		s, ok := v.(string)
		if !ok || !tags[s] {
			return fmt.Errorf("%s: unknown DNS tag %v", path, v)
		}
		return nil
	}
	checkResolver := func(v interface{}, path string) error {
		if m, ok := v.(map[string]interface{}); ok {
			s, ok := m["server"].(string)
			if !ok || s == "" {
				return fmt.Errorf("%s.server: required", path)
			}
			return checkTag(s, path+".server")
		}
		return checkTag(v, path)
	}
	if err = checkTag(dns["final"], "dns.final"); err != nil {
		return err
	}
	if err = checkResolver(t["default_domain_resolver"], "default_domain_resolver"); err != nil {
		return err
	}
	for i, item := range servers {
		if err = checkResolver(item.(map[string]interface{})["domain_resolver"], fmt.Sprintf("dns.servers[%d].domain_resolver", i)); err != nil {
			return err
		}
	}
	// Check explicit domain_resolver links without inferring implicit defaults.
	resolverLinks := map[string]string{}
	serverTypes := map[string]string{}
	for i, item := range servers {
		s := item.(map[string]interface{})
		tag := SubscriptionDNSTag(s, i)
		target, _ := s["domain_resolver"].(string)
		if object, ok := s["domain_resolver"].(map[string]interface{}); ok {
			target, _ = object["server"].(string)
		}
		resolverLinks[tag] = target
		serverTypes[tag], _ = s["type"].(string)
	}
	for start := range resolverLinks {
		seen := map[string]bool{}
		for tag := start; ; {
			target, ok := resolverLinks[tag]
			if !ok {
				break
			}
			if seen[tag] {
				return fmt.Errorf("dns.servers: circular domain_resolver dependency %q", tag)
			}
			seen[tag] = true
			tag = target
		}
	}
	catalog, err := list(t["rule_set"], "rule_set")
	if err != nil {
		return err
	}
	sets := map[string]bool{}
	for _, item := range catalog {
		tag, ok := item.(map[string]interface{})["tag"].(string)
		if !ok || tag == "" {
			return fmt.Errorf("rule_set: tag required")
		}
		if sets[tag] {
			return fmt.Errorf("rule_set: duplicate tag %q", tag)
		}
		sets[tag] = true
	}
	asRefs := func(v interface{}) ([]string, error) {
		if v == nil {
			return nil, nil
		}
		if s, ok := v.(string); ok {
			return []string{s}, nil
		}
		if a, ok := v.([]interface{}); ok {
			r := make([]string, 0, len(a))
			for _, x := range a {
				s, ok := x.(string)
				if !ok {
					return nil, fmt.Errorf("expected string reference")
				}
				r = append(r, s)
			}
			return r, nil
		}
		return nil, fmt.Errorf("expected string or list")
	}
	evaluated := map[string]bool{}
	var namedMatch func(map[string]interface{}) bool
	namedMatch = func(rule map[string]interface{}) bool {
		if _, ok := rule["match_response"].(string); ok {
			return true
		}
		if children, ok := rule["rules"].([]interface{}); ok {
			for _, child := range children {
				if r, ok := child.(map[string]interface{}); ok && namedMatch(r) {
					return true
				}
			}
		}
		return false
	}
	var checkRules func(interface{}, string, bool) error
	checkRules = func(v interface{}, path string, top bool) error {
		a, e := list(v, path)
		if e != nil {
			return e
		}
		for i, item := range a {
			r := item.(map[string]interface{})
			p := fmt.Sprintf("%s[%d]", path, i)
			if e = checkTag(r["server"], p+".server"); e != nil {
				return e
			}
			action, isString := r["action"].(string)
			if r["action"] != nil && !isString {
				return fmt.Errorf("%s.action: expected string", p)
			}
			if top && (action == "" || action == "route" || action == "evaluate") {
				if server, ok := r["server"].(string); !ok || server == "" {
					return fmt.Errorf("%s.server: required", p)
				}
			}
			var match option.DNSRuleMatchResponse
			if value := r["match_response"]; value != nil {
				raw, err := json.Marshal(value)
				if err != nil {
					return err
				}
				if err = json.Unmarshal(raw, &match); err != nil {
					return fmt.Errorf("%s.match_response: %w", p, err)
				}
			}
			if match.IsEnabled() && !evaluated[match.ResponseTag()] {
				return fmt.Errorf("%s.match_response: preceding evaluate required", p)
			}
			if top && action == "respond" {
				if r["type"] == "logical" && namedMatch(r) {
					return fmt.Errorf("%s: logical respond cannot bind a named response", p)
				}
				if match.ResponseTag() == "" && !evaluated[""] {
					return fmt.Errorf("%s: preceding untagged evaluate required", p)
				}
			}
			rs, e := asRefs(r["rule_set"])
			if e != nil {
				return fmt.Errorf("%s.rule_set: %w", p, e)
			}
			for _, s := range rs {
				if !sets[s] {
					return fmt.Errorf("%s.rule_set: unknown tag %q", p, s)
				}
			}
			rs, e = asRefs(r["preferred_by"])
			if e != nil {
				return fmt.Errorf("%s.preferred_by: %w", p, e)
			}
			for _, s := range rs {
				if e = checkTag(s, p+".preferred_by"); e != nil {
					return e
				}
			}
			if children, present := r["rules"]; present && children == nil {
				return fmt.Errorf("%s.rules: expected array", p)
			}
			if e = checkRules(r["rules"], p+".rules", false); e != nil {
				return e
			}
			if top && action == "evaluate" {
				tag, ok := r["tag"].(string)
				if r["tag"] != nil && !ok {
					return fmt.Errorf("%s.tag: expected string", p)
				}
				if tag != "" && evaluated[tag] {
					return fmt.Errorf("%s: duplicate evaluate tag %q", p, tag)
				}
				server, _ := r["server"].(string)
				if serverTypes[server] == "fakeip" {
					return fmt.Errorf("%s: evaluate cannot use fakeip", p)
				}
				evaluated[tag] = true
			}
		}
		return nil
	}
	if err = checkRules(dns["rules"], "dns.rules", true); err != nil {
		return err
	}
	var walk func(interface{}, string) error
	walk = func(v interface{}, path string) error {
		switch x := v.(type) {
		case []interface{}:
			for i, a := range x {
				if e := walk(a, fmt.Sprintf("%s[%d]", path, i)); e != nil {
					return e
				}
			}
		case map[string]interface{}:
			if x["action"] == "resolve" {
				if e := checkTag(x["server"], path+".server"); e != nil {
					return e
				}
			}
			if e := checkResolver(x["domain_resolver"], path+".domain_resolver"); e != nil {
				return e
			}
			if e := walk(x["rules"], path+".rules"); e != nil {
				return e
			}
		}
		return nil
	}
	if err = walk(t["rules"], "rules"); err != nil {
		return err
	}
	for _, k := range []string{"inbounds", "http_clients"} {
		if a, ok := t[k].([]interface{}); ok {
			for i, v := range a {
				if item, ok := v.(map[string]interface{}); ok {
					if err = checkResolver(item["domain_resolver"], fmt.Sprintf("%s[%d].domain_resolver", k, i)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// ExplicitSubscriptionResolver preserves both native forms, including unknown
// object fields introduced by newer sing-box releases. Invalid old stored
// values keep the previous fallback behavior until the operator repairs them.
func ExplicitSubscriptionResolver(v interface{}) (interface{}, bool) {
	switch x := v.(type) {
	case string:
		return x, x != ""
	case map[string]interface{}:
		s, ok := x["server"].(string)
		return x, ok && s != ""
	}
	return nil, false
}
