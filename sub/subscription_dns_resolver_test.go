package sub

import (
	"encoding/json"
	"testing"

	"github.com/shenaba/2s-ui/database"
	"github.com/shenaba/2s-ui/database/model"
)

func dnsSubscription(t *testing.T, name, template string) string {
	t.Helper()
	setupSubDB(t)
	seedLocalIPv6Client(t, name, "")
	if err := database.GetDB().Create(&model.Setting{Key: "subJsonExt", Value: template}).Error; err != nil {
		t.Fatal(err)
	}
	raw, _, err := (&JsonService{}).GetJson(name, "json")
	if err != nil {
		t.Fatal(err)
	}
	if raw == nil {
		t.Fatal("no JSON")
	}
	return *raw
}

func TestSubscriptionDNSObjectResolverPriority(t *testing.T) {
	raw := dnsSubscription(t, "dns-object-resolver", `{"dns":{"servers":[{"tag":"cloudflare","type":"https","server":"1.1.1.1"},{"tag":"local","type":"local"}],"final":"cloudflare"},"default_domain_resolver":{"server":"local","strategy":"ipv4_only","timeout":"3s"},"rules":[]}`)
	var cfg struct {
		Route map[string]interface{} `json:"route"`
		DNS   map[string]interface{} `json:"dns"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	r, ok := cfg.Route["default_domain_resolver"].(map[string]interface{})
	if !ok || r["server"] != "local" || r["strategy"] != "ipv4_only" || r["timeout"] != "3s" {
		t.Fatalf("resolver overwritten: %v", cfg.Route["default_domain_resolver"])
	}
	rules, ok := cfg.Route["rules"].([]interface{})
	if !ok || len(rules) != 0 {
		t.Fatalf("explicit empty rules overwritten: %v", cfg.Route["rules"])
	}
	if cfg.DNS["final"] != "cloudflare" {
		t.Fatalf("DNS final overwritten: %v", cfg.DNS)
	}
}

func TestSubscriptionDNSNativeConfig(t *testing.T) {
	buildSingBox(t, dnsSubscription(t, "dns-native", `{"dns":{"servers":[{"tag":"local","type":"local"},{"tag":"cloudflare","type":"https","server":"1.1.1.1"},{"tag":"google","type":"https","server":"8.8.8.8"}],"final":"cloudflare","rules":[{"action":"predefined","query_type":["AAAA","HTTPS"],"domain_suffix":["ru"],"rcode":"NOERROR"},{"action":"predefined","query_type":["AAAA","HTTPS"],"rule_set":["geosite-ru-whitelist","category-ru"],"rcode":"NOERROR"},{"action":"route","server":"local","domain_suffix":["ru"]},{"action":"route","server":"local","rule_set":["geosite-ru-whitelist","category-ru"]},{"action":"predefined","query_type":"HTTPS","rcode":"NOERROR"},{"action":"evaluate","server":"cloudflare","tag":"cf","timeout":"2s"},{"action":"respond","match_response":"cf","response_rcode":"NOERROR"},{"action":"respond","match_response":"cf","response_rcode":"NXDOMAIN"},{"action":"route","server":"google","timeout":"5s"}]},"default_domain_resolver":"cloudflare","rules":[{"action":"sniff"},{"protocol":"dns","action":"hijack-dns"}],"rule_set":[{"type":"inline","tag":"geosite-ru-whitelist","rules":[{"domain_suffix":["ru"]}]},{"type":"inline","tag":"category-ru","rules":[{"domain_suffix":["su"]}]}]}`))
}

func TestSubscriptionDNSNativeNestedConfig(t *testing.T) {
	buildSingBox(t, dnsSubscription(t, "dns-native-nested", `{"dns":{"servers":[{"tag":"local","type":"local"},{"tag":"cloudflare","type":"https","server":"1.1.1.1"},{"tag":"google","type":"https","server":"8.8.8.8"}],"final":"cloudflare","rules":[{"type":"logical","mode":"and","rules":[{"query_type":["AAAA","HTTPS"]},{"type":"logical","mode":"or","rules":[{"domain_suffix":["ru"]},{"rule_set":["geosite-ru-whitelist","category-ru"]}]}],"action":"predefined","rcode":"NOERROR"},{"type":"logical","mode":"or","rules":[{"domain_suffix":["ru"]},{"rule_set":["geosite-ru-whitelist","category-ru"]}],"action":"route","server":"local"},{"action":"evaluate","server":"cloudflare","tag":"cf","timeout":"2s"},{"action":"respond","match_response":"cf","response_rcode":"NOERROR"},{"action":"respond","match_response":"cf","response_rcode":"NXDOMAIN"},{"action":"route","server":"google","timeout":"5s"}]},"default_domain_resolver":"cloudflare","rules":[{"action":"sniff"},{"protocol":"dns","action":"hijack-dns"}],"rule_set":[{"type":"inline","tag":"geosite-ru-whitelist","rules":[{"domain_suffix":["ru"]}]},{"type":"inline","tag":"category-ru","rules":[{"domain_suffix":["su"]}]}]}`))
}
