package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestPackageMultipleSourcesPreserveSharedID(t *testing.T) {
	u1, u2 := "https://raw.githubusercontent.com/o/r/main/one.txt", "https://raw.githubusercontent.com/o/r/main/two.txt"
	cfg, err := parsePackageSources(fmt.Sprintf(`[%q,%q,%q]`, u1, u2, u1))
	if err != nil || !reflect.DeepEqual(cfg, []string{u1, u2}) {
		t.Fatalf("normalized: %v %v", cfg, err)
	}
	sourceCache := packageCache{Sources: map[string]packageSourceCache{
		u1: {Packages: []string{"com.one", "com.shared"}, ETag: "one"},
		u2: {Packages: []string{"com.shared", "com.two"}, ETag: "two"},
	}}
	pruned := packagePrune(sourceCache, []string{u2})
	if len(pruned.Sources) != 1 || !reflect.DeepEqual(pruned.Sources[u2].Packages, []string{"com.shared", "com.two"}) {
		t.Fatalf("after removal: %+v", pruned)
	}
	if len(sourceCache.Sources) != 2 {
		t.Fatalf("original cache modified: %+v", sourceCache)
	}
}

func TestPackageSourceLimits(t *testing.T) {
	var many []string
	for i := 0; i < maxPackageSources+1; i++ {
		many = append(many, fmt.Sprintf("https://raw.githubusercontent.com/o/r/main/%d.txt", i))
	}
	if _, err := parsePackageSources(fmt.Sprintf("[%q]", strings.Join(many, `","`))); err == nil {
		t.Fatal("too many sources accepted")
	}
	var ids []string
	for i := 0; i < maxPackageCount+1; i++ {
		ids = append(ids, fmt.Sprintf("com.example.app%d", i))
	}
	if _, err := parsePackageFile([]byte(strings.Join(ids, "\n"))); err == nil {
		t.Fatal("too many IDs accepted")
	}
}

func TestPackagePreparedCounterDoesNotCountNoTun(t *testing.T) {
	snap := &packageSnapshot{enabled: true, packages: []string{"a.b", "c.d"}, protected: map[string]bool{}, conflicts: 0}
	for _, v := range []interface{}{
		[]interface{}{},
		[]interface{}{map[string]interface{}{"type": "mixed"}},
	} {
		cfg := map[string]interface{}{"inbounds": v}
		count, _ := preparedCount(cfg, snap)
		if count != 0 {
			t.Errorf("custom inbound with no TUN: %d", count)
		}
	}
	count, _ := preparedCount(map[string]interface{}{}, snap)
	if count != 2 {
		t.Fatalf("default TUN count %d", count)
	}
}

func TestPackageConflictCounterDeduplicatesManualRemote(t *testing.T) {
	id := "com.protected.app"
	s := &packageSnapshot{
		enabled:   true,
		packages:  []string{"com.allowed.app"},
		protected: map[string]bool{id: true},
		blocked:   map[string]bool{id: true},
	}
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{"type": "tun", "exclude_package": []interface{}{id, "com.manual.app"}},
		},
	}
	prepared, conflicts := preparedCount(cfg, s)
	if prepared != 2 || conflicts != 1 {
		t.Fatalf("prepared=%d conflicts=%d", prepared, conflicts)
	}
}
