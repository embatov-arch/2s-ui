package sub

import (
	"encoding/json"
	"github.com/shenaba/2s-ui/database"
	"github.com/shenaba/2s-ui/database/model"
	"testing"
)

func TestSubscriptionDNSObjectResolverPriority(t *testing.T) {
	setupSubDB(t)
	seedLocalIPv6Client(t, "dns-object-resolver", "")
	template := `{"dns":{"servers":[{"tag":"cloudflare","type":"https","server":"1.1.1.1"},{"tag":"local","type":"local"}],"final":"cloudflare"},"default_domain_resolver":{"server":"local","strategy":"ipv4_only","timeout":"3s"},"rules":[]}`
	if err := database.GetDB().Create(&model.Setting{Key: "subJsonExt", Value: template}).Error; err != nil {
		t.Fatal(err)
	}
	raw, _, err := (&JsonService{}).GetJson("dns-object-resolver", "json")
	if err != nil {
		t.Fatal(err)
	}
	if raw == nil {
		t.Fatal("no JSON")
	}
	var cfg struct {
		Route map[string]interface{} `json:"route"`
		DNS   map[string]interface{} `json:"dns"`
	}
	if err = json.Unmarshal([]byte(*raw), &cfg); err != nil {
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
	setupSubDB(t)
	seedLocalIPv6Client(t, "dns-native", "")
	template := `{"dns":{"servers":[{"tag":"local","type":"local"},{"tag":"cloudflare","type":"https","server":"1.1.1.1"},{"tag":"google","type":"https","server":"8.8.8.8"}],"final":"cloudflare","rules":[{"action":"predefined","query_type":["AAAA","HTTPS"],"domain_suffix":["ru"],"rcode":"NOERROR"},{"action":"predefined","query_type":["AAAA","HTTPS"],"rule_set":["geosite-ru-whitelist","category-ru"],"rcode":"NOERROR"},{"action":"route","server":"local","domain_suffix":["ru"]},{"action":"route","server":"local","rule_set":["geosite-ru-whitelist","category-ru"]},{"action":"predefined","query_type":"HTTPS","rcode":"NOERROR"},{"action":"evaluate","server":"cloudflare","tag":"cf","timeout":"2s"},{"action":"respond","match_response":"cf","response_rcode":"NOERROR"},{"action":"respond","match_response":"cf","response_rcode":"NXDOMAIN"},{"action":"route","server":"google","timeout":"5s"}]},"default_domain_resolver":"cloudflare","rules":[{"action":"sniff"},{"protocol":"dns","action":"hijack-dns"}],"rule_set":[{"type":"inline","tag":"geosite-ru-whitelist","rules":[{"domain_suffix":["ru"]}]},{"type":"inline","tag":"category-ru","rules":[{"domain_suffix":["su"]}]}]}`
	if err := database.GetDB().Create(&model.Setting{Key: "subJsonExt", Value: template}).Error; err != nil {
		t.Fatal(err)
	}
	raw, _, err := (&JsonService{}).GetJson("dns-native", "json")
	if err != nil {
		t.Fatal(err)
	}
	if raw == nil {
		t.Fatal("no JSON")
	}
	buildSingBox(t, *raw)
}
