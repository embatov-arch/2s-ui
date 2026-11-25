package util

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestValidateSubscriptionDNS(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		ok        bool
	}{
		{"empty", "", true}, {"null", "null", false}, {"array", "[]", false}, {"custom tags", `{"dns":{"servers":[{"tag":"cloudflare","type":"https"},{"tag":"local","type":"local"}],"final":"cloudflare"},"default_domain_resolver":{"server":"local","strategy":"ipv4_only"}}`, true},
		{"unknown final", `{"dns":{"servers":[{"tag":"a"}],"final":"b"}}`, false}, {"duplicate tag", `{"dns":{"servers":[{"tag":"a"},{"tag":"a"}]}}`, false},
		{"implicit tags", `{"dns":{"servers":[{}],"final":"0"}}`, true},
		{"nested rule_set", `{"dns":{"servers":[{"tag":"a"}],"rules":[{"type":"logical","rules":[{"rule_set":"r"}],"action":"route","server":"a"}]},"rule_set":[{"tag":"r"}]}`, true},
		{"dangling nested", `{"dns":{"servers":[{"tag":"a"}],"rules":[{"type":"logical","rules":[{"rule_set":"r"}]}]}}`, false},
		{"dangling dial", `{"dns":{"servers":[{"tag":"a"}]},"inbounds":[{"domain_resolver":{"server":"b"}}]}`, false},
		{"dangling route resolve", `{"dns":{"servers":[{"tag":"a"}]},"rules":[{"action":"resolve","server":"b"}]}`, false},
		{"object unknown preserved", `{"dns":{"servers":[{"tag":"a","type":"future"}],"rules":[{"future_field":true,"action":"route","server":"a"}]},"default_domain_resolver":{"server":"a","future_option":true}}`, true},
		{"correct evaluate chain", `{"dns":{"servers":[{"tag":"a"}],"rules":[{"action":"evaluate","server":"a","tag":"a1"},{"match_response":"a1","action":"respond"}]}}`, true},
		{"wrong evaluate chain", `{"dns":{"servers":[{"tag":"a"}],"rules":[{"match_response":"a1","action":"respond"},{"action":"evaluate","server":"a","tag":"a1"}]}}`, false},
		{"no DNS resolver", `{"default_domain_resolver":"a"}`, false}, {"bad server list", `{"dns":{"servers":{}}}`, false}, {"bad rule list", `{"dns":{"rules":[null]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSubscriptionDNS(tc.raw); (err == nil) != tc.ok {
				t.Fatalf("got %v, expected valid=%v", err, tc.ok)
			}
		})
	}
}
func TestExplicitSubscriptionResolver(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{"server":"local","strategy":"ipv4_only","future":true}`), &m); err != nil {
		t.Fatal(err)
	}
	v, ok := ExplicitSubscriptionResolver(m)
	if !ok || !reflect.DeepEqual(v, m) {
		t.Fatalf("lost object %v", v)
	}
	if v, ok = ExplicitSubscriptionResolver("local"); !ok || v != "local" {
		t.Fatal(v)
	}
	for _, x := range []interface{}{nil, "", true, map[string]interface{}{"strategy": "ipv4_only"}} {
		if _, ok = ExplicitSubscriptionResolver(x); ok {
			t.Fatalf("invalid %v", x)
		}
	}
}
