package util

import (
	"reflect"
	"testing"
)

func TestValidateSubscriptionDNS(t *testing.T) {
	for _, cases := range []struct {
		ok   bool
		raws []string
	}{
		{true, []string{"", "{}", `{"dns":{"servers":[{"type":"local"}]}}`,
			`{"dns":{"servers":[{"type":"future","future_option":true}]}}`,
			`{"dns":{"rules":[{"future_action":true}]}}`, `{"dns":{"servers":[{"tag":"a"}],"final":"b"}}`}},
		{false, []string{"null", "[]", "{invalid", `{"dns":null}`, `{"dns":[]}`,
			`{"dns":{"servers":null}}`, `{"dns":{"servers":{}}}`, `{"dns":{"rules":[null]}}`,
			`{"rules":[1]}`, `{"rule_set":{}}`, `{"inbounds":[null]}`}},
	} {
		for _, raw := range cases.raws {
			if err := ValidateSubscriptionDNS(raw); (err == nil) != cases.ok {
				t.Errorf("%s: expected valid=%v, got %v", raw, cases.ok, err)
			}
		}
	}
}

func TestExplicitSubscriptionResolver(t *testing.T) {
	m := map[string]interface{}{"server": "local", "strategy": "ipv4_only", "future": true}
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

func TestSubscriptionImplicitDNSTags(t *testing.T) {
	for _, c := range []struct {
		server map[string]interface{}
		index  int
		want   string
	}{
		{map[string]interface{}{}, 0, "0"}, {map[string]interface{}{}, 3, "3"},
		{map[string]interface{}{"tag": "custom"}, 0, "custom"},
	} {
		if got := SubscriptionDNSTag(c.server, c.index); got != c.want {
			t.Fatalf("got %s, want %s", got, c.want)
		}
	}
}
