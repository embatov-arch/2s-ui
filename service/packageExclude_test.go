package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestPackageSourceURL(t *testing.T) {
	got, err := packageSourceURL("https://github.com/o/r/blob/main/dir/list.txt")
	if err != nil || got != "https://raw.githubusercontent.com/o/r/main/dir/list.txt" {
		t.Fatalf("canonical: %q %v", got, err)
	}
	for _, bad := range []string{"http://raw.githubusercontent.com/o/r/main/a.txt", "https://example.com/a.txt", "https://github.com/o/r", "https://github.com/o/r/blob/main/a.txt?token=x", "https://github.com/o/r/blob/main/../a.txt"} {
		if _, err := packageSourceURL(bad); err == nil {
			t.Errorf("expected invalid URL: %s", bad)
		}
	}
}

func TestParseSourcesAndSchedule(t *testing.T) {
	got, err := parsePackageSources(`["https://github.com/o/r/blob/main/a.txt","https://raw.githubusercontent.com/o/r/main/a.txt",""]`)
	if err != nil || len(got) != 1 {
		t.Fatalf("dedup %v %v", got, err)
	}
	for _, v := range []string{"0", "7", "3650"} {
		if _, err := packageScheduleDays(v); err != nil {
			t.Errorf("expected valid %s: %v", v, err)
		}
	}
	for _, v := range []string{"-1", "3651", "oops", "manual", "daily", "weekly", "monthly"} {
		if _, err := packageScheduleDays(v); err == nil {
			t.Errorf("expected invalid %s", v)
		}
	}
	if err := ValidatePackageSetting(packageSourcesKey, "not json"); err == nil {
		t.Fatal("invalid sources accepted")
	}
}

func TestPackageFileFormats(t *testing.T) {
	want := []string{"com.a.b", "org.c.d"}
	for name, s := range map[string]string{
		"text":   "# header\ncom.a.b, org.c.d # trailing\ncom.a.b\n",
		"array":  `["com.a.b","org.c.d","com.a.b"]`,
		"object": `{"exclude_package":["com.a.b","org.c.d"]}`,
	} {
		got, err := parsePackageFile([]byte(s))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s %v %v", name, got, err)
		}
	}
	if _, err := parsePackageFile([]byte("not a package")); err == nil {
		t.Fatal("invalid package accepted")
	}
}

func TestApplyPackagesIsIdempotent(t *testing.T) {
	old := packageCurrent.Load()
	t.Cleanup(func() { packageCurrent.Store(old) })
	packageCurrent.Store(&packageSnapshot{enabled: true, packages: []string{"a.b", "c.d"}, protected: map[string]bool{"x.y": true}})
	list := []interface{}{
		map[string]interface{}{"type": "tun", "exclude_package": []interface{}{"a.b", "x.y", "m.n", "https://raw.githubusercontent.com/o/r/main/a.txt"}},
		map[string]interface{}{"type": "mixed"},
	}
	ApplyPackageExcludes(list)
	first := list[0].(map[string]interface{})["exclude_package"]
	if !reflect.DeepEqual(first, []string{"a.b", "m.n", "c.d"}) {
		t.Fatalf("first: %v", first)
	}
	ApplyPackageExcludes(list)
	second := list[0].(map[string]interface{})["exclude_package"]
	if !reflect.DeepEqual(second, first) {
		t.Fatalf("second call changed output: %v vs %v", second, first)
	}
	if _, ok := list[1].(map[string]interface{})["exclude_package"]; ok {
		t.Fatal("non TUN modified")
	}
	payload, err := json.Marshal(list)
	if err != nil || strings.Contains(string(payload), "https://") {
		t.Fatalf("invalid client JSON: %s %v", payload, err)
	}
	packageCurrent.Store(&packageSnapshot{})
	plain := []interface{}{map[string]interface{}{"type": "tun", "exclude_package": []interface{}{"x.y"}}}
	ApplyPackageExcludes(plain)
	if !reflect.DeepEqual(plain[0].(map[string]interface{})["exclude_package"], []interface{}{"x.y"}) {
		t.Fatal("disabled snapshot changed config")
	}
}

func TestProtectedIDsAndManualCounting(t *testing.T) {
	cfg := map[string]interface{}{
		"rules":    []interface{}{map[string]interface{}{"package_name": []interface{}{"x.y"}}},
		"dns":      map[string]interface{}{"rules": []interface{}{map[string]interface{}{"type": "logical", "rules": []interface{}{map[string]interface{}{"include_package": "a.b"}}}}},
		"inbounds": []interface{}{map[string]interface{}{"type": "tun", "exclude_package": []interface{}{"m.n", "x.y"}}},
	}
	protected := map[string]bool{}
	protectedPackageIDs(cfg, protected)
	if !protected["x.y"] || !protected["a.b"] || protected["m.n"] {
		t.Fatalf("protected=%v", protected)
	}
	snap := &packageSnapshot{enabled: true, packages: []string{"c.d"}, protected: protected, blocked: map[string]bool{"a.b": true}, conflicts: 1}
	count, conflicts := preparedCount(cfg, snap)
	if count != 2 || conflicts != 2 {
		t.Fatalf("count=%d conflicts=%d", count, conflicts)
	}
	// Explicitly empty inbound list should NOT be counted as the default TUN.
	emptyCfg := map[string]interface{}{"inbounds": []interface{}{}}
	count, _ = preparedCount(emptyCfg, snap)
	if count != 0 {
		t.Fatalf("empty inbounds counted as default TUN: %d", count)
	}
}

func TestMultipleSourcesAndLastGoodCache(t *testing.T) {
	oldClient := packageHTTP
	t.Cleanup(func() { packageHTTP = oldClient })
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		if status == http.StatusNotModified && r.Header.Get("If-None-Match") != `"v1"` {
			t.Errorf("missing ETag")
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte("a.b\nc.d\na.b\n"))
		}
	}))
	defer server.Close()
	packageHTTP = server.Client()
	one, err := packageFetch(context.Background(), server.URL, packageSourceCache{})
	if err != nil || !reflect.DeepEqual(one.Packages, []string{"a.b", "c.d"}) {
		t.Fatalf("first %v %v", one, err)
	}
	status = http.StatusNotModified
	two, err := packageFetch(context.Background(), server.URL, one)
	if err != nil || !reflect.DeepEqual(two.Packages, one.Packages) {
		t.Fatalf("304 %v %v", two, err)
	}
	status = http.StatusInternalServerError
	three, err := packageFetch(context.Background(), server.URL, two)
	if err == nil || !reflect.DeepEqual(three.Packages, one.Packages) {
		t.Fatalf("failed fetch lost cached list %v %v", three, err)
	}
	cache := packageCache{Sources: map[string]packageSourceCache{"a": one, "b": {Packages: []string{"c.d", "e.f"}}}}
	pruned := packagePrune(cache, []string{"b"})
	if len(pruned.Sources) != 1 || !reflect.DeepEqual(pruned.Sources["b"].Packages, []string{"c.d", "e.f"}) {
		t.Fatalf("pruned %v", pruned)
	}
}
