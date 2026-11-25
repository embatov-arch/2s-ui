package util

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
	d, exists := t["dns"]
	if !exists {
		if v, ok := t["default_domain_resolver"]; ok && v != "" {
			return fmt.Errorf("default_domain_resolver requires DNS")
		}
		return nil
	}
	dns, ok := d.(map[string]interface{})
	if !ok {
		return fmt.Errorf("dns: expected object")
	}
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
	servers, err := list(dns["servers"], "dns.servers")
	if err != nil {
		return err
	}
	tags := map[string]bool{}
	allNamed := true
	for i, item := range servers {
		s := item.(map[string]interface{})
		tag, ok := s["tag"].(string)
		if _, exists := s["tag"]; exists && !ok {
			return fmt.Errorf("dns.servers[%d].tag: expected string", i)
		}
		if tag == "" {
			allNamed = false
			continue
		}
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
		if !ok || allNamed && !tags[s] {
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
	catalog, err := list(t["rule_set"], "rule_set")
	if err != nil {
		return err
	}
	sets := map[string]bool{}
	for _, item := range catalog {
		if tag, ok := item.(map[string]interface{})["tag"].(string); ok {
			sets[tag] = true
		}
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
			action, _ := r["action"].(string)
			if top && (action == "" || action == "route" || action == "evaluate") {
				if server, ok := r["server"].(string); !ok || server == "" {
					return fmt.Errorf("%s.server: required", p)
				}
			}
			if r["match_response"] == true && !evaluated[""] {
				return fmt.Errorf("%s.match_response: preceding untagged evaluate required", p)
			}
			if tag, ok := r["match_response"].(string); ok && !evaluated[tag] {
				return fmt.Errorf("%s.match_response: preceding evaluate required", p)
			}
			if top && action == "respond" && len(evaluated) == 0 {
				return fmt.Errorf("%s: preceding evaluate required", p)
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
			if e = checkRules(r["rules"], p+".rules", false); e != nil {
				return e
			}
			if top && action == "evaluate" {
				tag, _ := r["tag"].(string)
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
