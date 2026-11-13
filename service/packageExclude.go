package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shenaba/2s-ui/database"
	"github.com/shenaba/2s-ui/database/model"
	"gorm.io/gorm"
)

// Package exclusions are compiled at startup and whenever settings or the
// fetched GitHub source change. Subscription requests only read the immutable
// prepared snapshot; they NEVER fetch, parse or scan routing rules.
const (
	packageSourceKey   = "packageExcludeURL"
	packageScheduleKey = "packageExcludeSchedule"
	packageRawKey      = "packageExcludeRaw"
	packageRawURLKey   = "packageExcludeRawURL"
	packageETagKey     = "packageExcludeETag"
	packageCheckedKey  = "packageExcludeCheckedAt"
	packageUpdatedKey  = "packageExcludeUpdatedAt"
	packagePreparedKey = "packageExcludePrepared"
)

const maxPackageFile = 512 * 1024
const maxPackageCount = 10000

var packageID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)+$`)

func validPackageID(id string) bool { return id == "android" || packageID.MatchString(id) }

var packageRefreshLock sync.Mutex

// Snapshot is never mutated after publication. Disabling the remote source
// restores the unchanged administrator-supplied TUN configuration.
type packageSnapshot struct {
	enabled    bool
	inbounds   []interface{}
	count      int
	conflicts  int
	preparedAt time.Time
}

var packageCurrent atomic.Pointer[packageSnapshot]

func getPackageSetting(key string) (string, error) {
	return (&SettingService{}).getString(key)
}

func putPackageSetting(tx *gorm.DB, key, value string) error {
	return tx.Where("key = ?", key).Assign(model.Setting{Value: value}).FirstOrCreate(&model.Setting{Key: key}).Error
}

// PackageExcludePreparedInbounds performs no per-client rules processing.
func PackageExcludePreparedInbounds() ([]interface{}, bool) {
	s := packageCurrent.Load()
	if s == nil || !s.enabled || len(s.inbounds) == 0 {
		return nil, false
	}
	return s.inbounds, true
}

func packageSourceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > 2048 {
		return "", errors.New("package source URL is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "", errors.New("invalid package source URL")
	}
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.RawQuery != "" || u.RawPath != "" {
		return "", errors.New("package list requires a plain HTTPS GitHub file URL")
	}
	if u.Hostname() == "github.com" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 5 || parts[2] != "blob" {
			return "", errors.New("use a GitHub file URL, not a repository homepage")
		}
		u.Host = "raw.githubusercontent.com"
		u.Path = "/" + strings.Join(append(parts[:2], parts[3:]...), "/")
	} else if u.Hostname() != "raw.githubusercontent.com" {
		return "", errors.New("only github.com/blob and raw.githubusercontent.com are supported")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return "", errors.New("GitHub URL must identify a file")
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsAny(p, "\\\r\n") {
			return "", errors.New("unsafe GitHub path")
		}
	}
	return u.String(), nil
}

func PackageExcludeValidateSettings(source, schedule string) error {
	if _, err := packageSourceURL(source); err != nil {
		return err
	}
	switch schedule {
	case "manual", "daily", "weekly", "monthly":
		return nil
	default:
		return errors.New("invalid package exclusion refresh schedule")
	}
}

func parsePackageFile(data []byte) ([]string, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("package list is empty")
	}
	var raw []string
	if data[0] == '[' || data[0] == '{' {
		if data[0] == '[' {
			if err := json.Unmarshal(data, &raw); err != nil {
				return nil, fmt.Errorf("invalid JSON array: %w", err)
			}
		} else {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(data, &obj); err != nil {
				return nil, err
			}
			val, ok := obj["exclude_package"]
			if !ok {
				val, ok = obj["packages"]
			}
			if !ok {
				return nil, errors.New("JSON object must contain exclude_package or packages")
			}
			if err := json.Unmarshal(val, &raw); err != nil {
				return nil, err
			}
		}
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
				continue
			}
			if i := strings.Index(line, "#"); i >= 0 {
				line = line[:i]
			}
			for _, part := range strings.Split(line, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					raw = append(raw, part)
				}
			}
		}
	}
	out, seen := make([]string, 0, len(raw)), map[string]bool{}
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if !validPackageID(id) {
			return nil, fmt.Errorf("invalid Android package ID %q", id)
		}
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
		if len(out) > maxPackageCount {
			return nil, errors.New("too many package IDs")
		}
	}
	if len(out) == 0 {
		return nil, errors.New("package list has no valid entries")
	}
	return out, nil
}

func stringPackages(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case string:
		return []string{t}
	case []interface{}:
		a := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				a = append(a, s)
			}
		}
		return a
	default:
		return nil
	}
}

// Extract exact Android package IDs from explicit app routing and DNS rules,
// including nested logical rules. Domain, IP and regex matches are not
// package IDs and cannot be reliably inferred from one another.
func protectedPackageIDs(value interface{}, found map[string]bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		for k, child := range v {
			if k == "exclude_package" {
				continue
			}
			if k == "package_name" || k == "include_package" {
				for _, id := range stringPackages(child) {
					if validPackageID(id) {
						found[id] = true
					}
				}
			} else {
				protectedPackageIDs(child, found)
			}
		}
	case []interface{}:
		for _, child := range v {
			protectedPackageIDs(child, found)
		}
	}
}

// preparePackageInbounds deep-copies the administrator's JSON template, edits
// only TUN exclude_package and returns the entire prepared inbound section.
// An invalid template never replaces an earlier good snapshot.
func preparePackageInbounds(template []byte, remote []string) ([]interface{}, int, int, error) {
	if len(bytes.TrimSpace(template)) == 0 {
		return nil, 0, 0, nil
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(template, &cfg); err != nil {
		return nil, 0, 0, err
	}
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return nil, 0, 0, nil
	}
	protected := map[string]bool{}
	protectedPackageIDs(cfg, protected)
	seenTotal := map[string]bool{}
	suppressed := map[string]bool{}
	total := 0
	for _, v := range inbounds {
		inbound, ok := v.(map[string]interface{})
		if !ok || inbound["type"] != "tun" {
			continue
		}
		source := append(stringPackages(inbound["exclude_package"]), remote...)
		cleaned, seen := make([]string, 0, len(source)), map[string]bool{}
		for _, id := range source {
			if !validPackageID(id) {
				return nil, 0, 0, fmt.Errorf("invalid manual package ID %q", id)
			}
			if protected[id] {
				suppressed[id] = true
				continue
			}
			if !seen[id] {
				cleaned = append(cleaned, id)
				seen[id] = true
			}
			seenTotal[id] = true
		}
		inbound["exclude_package"] = cleaned
		total += len(cleaned)
	}
	return inbounds, total, len(suppressed), nil
}

func packageReadRaw() ([]string, error) {
	data, err := getPackageSetting(packageRawKey)
	if err != nil {
		return nil, err
	}
	if data == "" {
		return nil, nil
	}
	var raw []string
	if err = json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func packageBuildSnapshot() (*packageSnapshot, error) {
	source, err := getPackageSetting(packageSourceKey)
	if err != nil {
		return nil, err
	}
	if source == "" {
		return &packageSnapshot{}, nil
	}
	cacheURL, err := getPackageSetting(packageRawURLKey)
	if err != nil {
		return nil, err
	}
	var remote []string
	if cacheURL == source {
		remote, err = packageReadRaw()
		if err != nil {
			return nil, err
		}
	}
	template, err := getPackageSetting("subJsonExt")
	if err != nil {
		return nil, err
	}
	inbound, count, conflicts, err := preparePackageInbounds([]byte(template), remote)
	if err != nil {
		return nil, err
	}
	return &packageSnapshot{enabled: true, inbounds: inbound, count: count, conflicts: conflicts, preparedAt: time.Now()}, nil
}

func packageSetSnapshot(s *packageSnapshot) error {
	if s == nil {
		return errors.New("nil exclusion snapshot")
	}
	b, err := json.Marshal(s.inbounds)
	if err != nil {
		return err
	}
	// Recording the prepared version is useful for audit/restarts; it is not
	// used as editable input so a later refresh cannot accumulate removals.
	if err = (&SettingService{}).setString(packagePreparedKey, string(b)); err != nil {
		return err
	}
	packageCurrent.Store(s)
	return nil
}

// Call on every APP.Start (including SIGHUP) and after a settings Save commit.
func PackageExcludeInventory() error {
	packageRefreshLock.Lock()
	defer packageRefreshLock.Unlock()
	s, err := packageBuildSnapshot()
	if err != nil {
		return err
	}
	return packageSetSnapshot(s)
}

// Remote download is an explicit action or a timed cron job, never part of a
// JSON subscription request, and never initiated merely by starting the app.
func PackageExcludeRefresh(ctx context.Context) error {
	packageRefreshLock.Lock()
	defer packageRefreshLock.Unlock()
	source, err := getPackageSetting(packageSourceKey)
	if err != nil {
		return err
	}
	normalized, err := packageSourceURL(source)
	if err != nil {
		return err
	}
	if normalized == "" {
		return errors.New("configure a GitHub file URL first")
	}
	oldURL, err := getPackageSetting(packageRawURLKey)
	if err != nil {
		return err
	}
	etag := ""
	if oldURL == source {
		etag, err = getPackageSetting(packageETagKey)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	req.Header.Set("Accept", "application/json,text/plain;q=0.9,*/*;q=0.1")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	now := strconv.FormatInt(time.Now().Unix(), 10)
	if resp.StatusCode == http.StatusNotModified && oldURL == source {
		return (&SettingService{}).setString(packageCheckedKey, now)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPackageFile+1))
	if err != nil {
		return err
	}
	if len(body) > maxPackageFile {
		return errors.New("GitHub package file exceeds 512 KiB")
	}
	remote, err := parsePackageFile(body)
	if err != nil {
		return err
	}
	template, err := getPackageSetting("subJsonExt")
	if err != nil {
		return err
	}
	inbounds, count, conflicts, err := preparePackageInbounds([]byte(template), remote)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(remote)
	// A server may omit ETag or answer 200 with identical content. Do not
	// rebuild or rewrite the prepared template when nothing actually changed.
	if oldURL == source {
		previous, readErr := getPackageSetting(packageRawKey)
		if readErr != nil {
			return readErr
		}
		if previous == string(data) {
			return database.GetDB().Transaction(func(tx *gorm.DB) error {
				if err := putPackageSetting(tx, packageCheckedKey, now); err != nil {
					return err
				}
				return putPackageSetting(tx, packageETagKey, resp.Header.Get("ETag"))
			})
		}
	}
	prepared, _ := json.Marshal(inbounds)
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		values := map[string]string{
			packageRawKey: string(data), packageRawURLKey: source, packageETagKey: resp.Header.Get("ETag"),
			packageCheckedKey: now, packageUpdatedKey: now, packagePreparedKey: string(prepared),
		}
		for k, v := range values {
			if err := putPackageSetting(tx, k, v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	packageCurrent.Store(&packageSnapshot{enabled: true, inbounds: inbounds, count: count, conflicts: conflicts, preparedAt: time.Now()})
	return nil
}

func packageInterval(schedule string) time.Duration {
	switch schedule {
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	case "monthly":
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}

// Poll hourly to pick up schedule changes without restarting the panel.
// A failed GitHub request preserves the cache and the served template.
func PackageExcludeRefreshIfDue(ctx context.Context) error {
	schedule, err := getPackageSetting(packageScheduleKey)
	if err != nil {
		return err
	}
	interval := packageInterval(schedule)
	if interval == 0 {
		return nil
	}
	source, err := getPackageSetting(packageSourceKey)
	if err != nil || source == "" {
		return err
	}
	checked, err := getPackageSetting(packageCheckedKey)
	if err != nil {
		return err
	}
	cachedURL, err := getPackageSetting(packageRawURLKey)
	if err != nil {
		return err
	}
	if cachedURL == source {
		if seconds, e := strconv.ParseInt(checked, 10, 64); e == nil && time.Since(time.Unix(seconds, 0)) < interval {
			return nil
		}
	}
	return PackageExcludeRefresh(ctx)
}

func PackageExcludeStatus() (map[string]interface{}, error) {
	source, err := getPackageSetting(packageSourceKey)
	if err != nil {
		return nil, err
	}
	schedule, err := getPackageSetting(packageScheduleKey)
	if err != nil {
		return nil, err
	}
	checked, err := getPackageSetting(packageCheckedKey)
	if err != nil {
		return nil, err
	}
	updated, err := getPackageSetting(packageUpdatedKey)
	if err != nil {
		return nil, err
	}
	cachedURL, err := getPackageSetting(packageRawURLKey)
	if err != nil {
		return nil, err
	}
	raw := []string{}
	if cachedURL == source {
		raw, err = packageReadRaw()
		if err != nil {
			return nil, err
		}
	}
	s := packageCurrent.Load()
	count, conflicts := 0, 0
	if s != nil {
		count = s.count
		conflicts = s.conflicts
	}
	return map[string]interface{}{
		"source": source, "schedule": schedule, "cached": len(raw), "prepared": count,
		"conflicts": conflicts, "checkedAt": checked, "updatedAt": updated, "sourceMatchesCache": source == cachedURL,
	}, nil
}
