package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPackageExcludeParseAndSafeURL(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"com.example.one\ncom.example.two\ncom.example.one\n# note\n", []string{"com.example.one", "com.example.two"}},
		{`["com.example.one", "com.example.two", "com.example.one"]`, []string{"com.example.one", "com.example.two"}},
		{`{"exclude_package":["com.example.one"]}`, []string{"com.example.one"}},
		{`{"packages":["com.example.one","android"]}`, []string{"com.example.one", "android"}},
	}
	for _, tt := range cases {
		got, err := parsePackageFile([]byte(tt.in))
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parse %q: got=%v err=%v", tt.in, got, err)
		}
	}
	for _, bad := range []string{"https://127.0.0.1/list", "http://raw.githubusercontent.com/a/b/main/c", "https://github.com/a/b", "https://raw.githubusercontent.com/a/b/main/../file", "https://github.com@127.0.0.1/a", "https://raw.githubusercontent.com:8443/a/b/main/c"} {
		if _, err := packageSourceURL(bad); err == nil {
			t.Errorf("unsafe URL was accepted: %s", bad)
		}
	}
	url, err := packageSourceURL("https://github.com/o/r/blob/main/android/packages.txt")
	if err != nil || url != "https://raw.githubusercontent.com/o/r/main/android/packages.txt" {
		t.Fatalf("unexpected conversion %q %v", url, err)
	}
	for _, bad := range []string{"com.valid.pkg\ninvalid domain\n", `{"packages":[]}`, `{"other":[]}`} {
		if _, err := parsePackageFile([]byte(bad)); err == nil {
			t.Errorf("invalid package list accepted: %q", bad)
		}
	}
}

func TestPackageExcludeInventory(t *testing.T) {
	const source = `{
    "inbounds":[
      {"type":"mixed","tag":"proxy"},
      {"type":"tun","tag":"tun-in","exclude_package":["org.telegram.messenger","ru.bank.app","ru.bank.app"]},
      {"type":"tun","tag":"tun-2","exclude_package":["com.manual.only"]}
    ],
    "dns":{"rules":[{"package_name":["ru.bank.app"]}]},
    "rules":[{"type":"logical","rules":[{"package_name":"org.telegram.messenger","outbound":"proxy"}]}]
  }`
	raw := []string{"ru.bank.app", "org.telegram.messenger", "ru.music.app", "ru.music.app"}
	got, n, conflicts, err := preparePackageInbounds([]byte(source), raw)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || conflicts != 2 {
		t.Fatalf("count=%d conflicts=%d", n, conflicts)
	}
	if got[0].(map[string]interface{})["tag"] != "proxy" {
		t.Fatal("unrelated inbound changed")
	}
	for i, want := range [][]string{{"ru.music.app"}, {"com.manual.only", "ru.music.app"}} {
		exclusions := stringPackages(got[i+1].(map[string]interface{})["exclude_package"])
		if !reflect.DeepEqual(exclusions, want) {
			t.Errorf("tun %d got=%v want=%v", i, exclusions, want)
		}
	}
	// No mutation of the source template: tomorrow's inventory can reapply
	// everything again instead of permanently dropping conflicts.
	var original map[string]interface{}
	if err := json.Unmarshal([]byte(source), &original); err != nil {
		t.Fatal(err)
	}
	origTUN := original["inbounds"].([]interface{})[1].(map[string]interface{})["exclude_package"].([]interface{})
	if len(origTUN) != 3 {
		t.Fatal("input template unexpectedly mutated")
	}
	// Determinism: same input => same JSON, no growth on repeated rebuild.
	again, _, _, _ := preparePackageInbounds([]byte(source), raw)
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Errorf("non-deterministic rebuild: %s != %s", a, b)
	}
}

func TestPackageExcludeWithoutTunOrInvalidTemplate(t *testing.T) {
	v, c, conflicts, err := preparePackageInbounds([]byte(`{"rules":[{"package_name":"org.telegram.messenger"}]}`), []string{"org.telegram.messenger"})
	if err != nil || v != nil || c != 0 || conflicts != 0 {
		t.Fatalf("unexpected %v %d %d %v", v, c, conflicts, err)
	}
	if _, _, _, err = preparePackageInbounds([]byte(`{bad json`), []string{"com.example.pkg"}); err == nil {
		t.Fatal("invalid template accepted")
	}
	if _, _, _, err = preparePackageInbounds([]byte(`{"inbounds":[{"type":"tun","exclude_package":["bad package name"]}]}`), nil); err == nil {
		t.Fatal("invalid manual id accepted")
	}
	if _, err = packageSourceURL(strings.Repeat("x", 3000)); err == nil {
		t.Fatal("oversized URL accepted")
	}
}

func TestPackageExcludeSchedule(t *testing.T) {
	if err := PackageExcludeValidateSettings("https://github.com/owner/repo/blob/main/list.txt", "weekly"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "hourly", "every-client", "on-start"} {
		if err := PackageExcludeValidateSettings("", bad); err == nil {
			t.Errorf("accepted schedule %q", bad)
		}
	}
	if d := packageInterval("manual"); d != 0 {
		t.Fatalf("manual interval=%v", d)
	}
	if d := packageInterval("daily"); d.Hours() != 24 {
		t.Fatalf("daily interval=%v", d)
	}
	if d := packageInterval("weekly"); d.Hours() != 168 {
		t.Fatalf("weekly interval=%v", d)
	}
	if d := packageInterval("monthly"); d.Hours() != 720 {
		t.Fatalf("monthly interval=%v", d)
	}
}
