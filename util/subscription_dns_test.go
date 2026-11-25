package util

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestValidateSubscriptionDNS(t *testing.T) {
	for _, tc := range []struct {
		raw string
		ok  bool
	}{
		{"", true},
		{"{}", true},
		{"null", false},
		{"[]", false},
		{"{invalid", false},
		{`{"dns":null}`, false},
		{`{"dns":[]}`, false},
		{`{"dns":{"servers":null}}`, false},
		{`{"dns":{"servers":{}}}`, false},
		{`{"dns":{"rules":[null]}}`, false},
		{`{"rules":[1]}`, false},
		{`{"rule_set":{}}`, false},
		{`{"inbounds":[null]}`, false},
		{`{"dns":{"servers":[{"type":"local"}]}}`, true},
		{`{"dns":{"servers":[{"type":"future","future_option":true}]}}`, true},
		{`{"dns":{"rules":[{"future_action":true}]}}`, true},
		{`{"dns":{"servers":[{"tag":"a"}],"final":"b"}}`, true},
	} {
		if err := ValidateSubscriptionDNS(tc.raw); (err == nil) != tc.ok {
			t.Fatalf("got %v, expected valid=%v: %s", err, tc.ok, tc.raw)
		}
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

func TestSubscriptionImplicitDNSTags(t *testing.T) {
	if tag := SubscriptionDNSTag(map[string]interface{}{}, 0); tag != "0" {
		t.Fatal(tag)
	}
	if tag := SubscriptionDNSTag(map[string]interface{}{}, 3); tag != "3" {
		t.Fatal(tag)
	}
	if tag := SubscriptionDNSTag(map[string]interface{}{"tag": "custom"}, 0); tag != "custom" {
		t.Fatal(tag)
	}
}
