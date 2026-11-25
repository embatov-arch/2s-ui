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
