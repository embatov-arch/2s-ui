package util

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SubscriptionDNSTag follows sing-box's implicit DNS transport tags.
func SubscriptionDNSTag(server map[string]interface{}, index int) string {
	if tag := AsString(server["tag"]); tag != "" {
		return tag
	}
	return strconv.Itoa(index)
}

// ValidateSubscriptionDNS checks the JSON shapes consumed by the panel.
// DNS semantics belong to the subscriber's sing-box version.
func ValidateSubscriptionDNS(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var template map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &template); err != nil {
		return fmt.Errorf("subJsonExt: %w", err)
	}
	if template == nil {
		return fmt.Errorf("subJsonExt: expected object")
	}
	check := func(object map[string]interface{}, prefix string, keys ...string) error {
		for _, key := range keys {
			value, exists := object[key]
			if !exists {
				continue
			}
			items, ok := value.([]interface{})
			if !ok {
				return fmt.Errorf("%s%s: expected array", prefix, key)
			}
			for _, item := range items {
				if _, ok := item.(map[string]interface{}); !ok {
					return fmt.Errorf("%s%s: expected object entries", prefix, key)
				}
			}
		}
		return nil
	}
	for _, key := range []string{"dns", "log", "experimental"} {
		if value, exists := template[key]; exists {
			if _, ok := value.(map[string]interface{}); !ok {
				return fmt.Errorf("%s: expected object", key)
			}
		}
	}
	if err := check(template, "", "rules", "rule_set", "inbounds"); err != nil {
		return err
	}
	if dns, ok := template["dns"].(map[string]interface{}); ok {
		return check(dns, "dns.", "servers", "rules")
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
