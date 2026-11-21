package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shenaba/2s-ui/logger"
)

// Remote Android package lists are prepared before subscription requests.
// Only packageExcludeSources and packageExcludeSchedule are operator-editable.
const (
	packageSourcesKey             = "packageExcludeSources"
	packageScheduleKey            = "packageExcludeSchedule"
	packageCacheKey               = "packageExcludeCache"
	maxPackageFile                = 512 * 1024
	maxPackageCount               = 10000
	maxPackageSources             = 128
	packageExcludeMaxScheduleDays = 3650
)

var packageID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)+$`)

func validPackageID(id string) bool { return id == "android" || packageID.MatchString(id) }

var (
	packageSettings SettingService
	packageLock     sync.Mutex
	packageCurrent  atomic.Pointer[packageSnapshot]
	packageHTTP     = &http.Client{
		Timeout: 15 * time.Second,
		// Explicitly ignore environment proxies and validate every resolved IP.
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           packageSafeDialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			DisableKeepAlives:     true,
		},
		CheckRedirect: packageSafeRedirect,
	}
)

type packageSnapshot struct {
	enabled   bool
	packages  []string
	protected map[string]bool
	blocked   map[string]bool // unique remote IDs removed due to route protection
	prepared  int
	conflicts int
}

// ApplyPackageExcludes modifies only TUN exclude_package in the per-request
// config map. It performs no I/O and does not modify shared snapshot slices.
func ApplyPackageExcludes(inbounds interface{}) {
	s := packageCurrent.Load()
	if s == nil || !s.enabled {
		return
	}
	list, _ := inbounds.([]interface{})
	for _, item := range list {
		tun, ok := item.(map[string]interface{})
		if !ok || tun["type"] != "tun" {
			continue
		}
		manual := stringPackages(tun["exclude_package"])
		have := make(map[string]bool, len(manual)+len(s.packages))
		merged := make([]string, 0, len(manual)+len(s.packages))
		add := func(id string) {
			// Legacy inline links are metadata, not valid sing-box package IDs.
			// They must never escape into a client configuration.
			if !validPackageID(id) || s.protected[id] || have[id] {
				return
			}
			have[id] = true
			merged = append(merged, id)
		}
		for _, id := range manual {
			add(id)
		}
		for _, id := range s.packages {
			add(id)
		}
		tun["exclude_package"] = merged
	}
}

// Block private, loopback, link-local, shared, benchmark, documentation and
// reserved addresses. IsGlobalUnicast alone does NOT exclude all of these.
var packageNonPublicNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fec0::/10"),
}

func packagePublicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok || address.Zone() != "" {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	for _, blocked := range packageNonPublicNetworks {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

func packagePublicHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || strings.ContainsAny(host, string([]byte{92, 37, 32, 9, 13, 10})) {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return packagePublicIP(ip)
	}
	if !strings.Contains(host, ".") {
		return false
	}
	for _, suffix := range []string{".localhost", ".local", ".lan", ".internal", ".home.arpa", ".test", ".invalid", ".example", ".onion", ".localdomain"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	return true
}

// Resolve then dial the checked IP itself. Dialing the hostname again would
// permit DNS rebinding between the check and the actual TCP connection.
func packageSafeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if !packagePublicHost(host) {
		return nil, fmt.Errorf("package source host is not public: %q", host)
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(resolved) == 0 {
		return nil, errors.New("package source host has no IP addresses")
	}
	for _, ip := range resolved {
		if !packagePublicIP(ip.IP) || ip.Zone != "" {
			return nil, fmt.Errorf("package source host resolves to a non-public IP: %q", host)
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var failures []error
	for _, ip := range resolved {
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return connection, nil
		}
		failures = append(failures, err)
	}
	return nil, errors.Join(failures...)
}

// Redirects are common with GitLab, object storage and download CDNs.
// Validate every hop; the transport independently checks and pins DNS IPs.
func packageSafeRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many package source redirects")
	}
	_, err := packageSourceURL(req.URL.String())
	if err != nil {
		return fmt.Errorf("unsafe package source redirect: %w", err)
	}
	return nil
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
	if u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Hostname() == "" {
		return "", errors.New("package list requires a public HTTPS URL without credentials or fragments")
	}
	if !packagePublicHost(u.Hostname()) {
		return "", errors.New("package source must use a public HTTPS hostname or IP")
	}
	if u.Path == "" || u.Path == "/" || strings.ContainsAny(u.Path, string([]byte{92, 13, 10})) {
		return "", errors.New("package source URL must identify a file path")
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return "", errors.New("unsafe package source file path")
		}
	}
	// Preserve the user-friendly GitHub file URL and the historical cache key.
	if strings.EqualFold(u.Hostname(), "github.com") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 5 && parts[2] == "blob" {
			if u.RawQuery != "" {
				return "", errors.New("GitHub blob links must not contain query parameters")
			}
			u.Host = "raw.githubusercontent.com"
			u.Path = "/" + strings.Join(append(parts[:2], parts[3:]...), "/")
			u.RawPath = ""
		}
	}
	return u.String(), nil
}

func parsePackageSources(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var input []string
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, fmt.Errorf("package sources must be a JSON array: %w", err)
	}
	if len(input) > maxPackageSources {
		return nil, fmt.Errorf("too many remote sources (maximum %d)", maxPackageSources)
	}
	out := make([]string, 0, len(input))
	for _, entry := range input {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		source, err := packageSourceURL(entry)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, source) {
			out = append(out, source)
		}
	}
	return out, nil
}

func packageSources() ([]string, error) {
	raw, err := packageSettings.getString(packageSourcesKey)
	if err != nil {
		return nil, err
	}
	return parsePackageSources(raw)
}

func packageScheduleDays(value string) (int, error) {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || days < 0 || days > packageExcludeMaxScheduleDays {
		return 0, fmt.Errorf("package refresh interval must be an integer from 0 to %d days", packageExcludeMaxScheduleDays)
	}
	return days, nil
}

func ValidatePackageSetting(key, value string) error {
	switch key {
	case packageSourcesKey:
		_, err := parsePackageSources(value)
		return err
	case packageScheduleKey:
		_, err := packageScheduleDays(value)
		return err
	}
	return nil
}

func parsePackageFile(data []byte) ([]string, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("package list is empty")
	}
	var raw []string
	switch data[0] {
	case '[':
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
	case '{':
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
	default:
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			if i := strings.Index(line, "#"); i >= 0 {
				line = line[:i]
			}
			if i := strings.Index(line, "//"); i >= 0 {
				line = line[:i]
			}
			for _, part := range strings.Split(line, ",") {
				if id := strings.TrimSpace(part); id != "" {
					raw = append(raw, id)
				}
			}
		}
	}
	out, seen := make([]string, 0, len(raw)), make(map[string]bool, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if !validPackageID(id) {
			return nil, fmt.Errorf("invalid Android package ID %q", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
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
	case string:
		return []string{t}
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func protectedPackageIDs(value interface{}, found map[string]bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		for k, child := range v {
			switch k {
			case "exclude_package": // this field is the output, never a protection rule
			case "package_name", "include_package":
				for _, id := range stringPackages(child) {
					if validPackageID(id) {
						found[id] = true
					}
				}
			default:
				protectedPackageIDs(child, found)
			}
		}
	case []interface{}:
		for _, child := range v {
			protectedPackageIDs(child, found)
		}
	}
}

type packageSourceCache struct {
	Packages  []string `json:"packages"`
	ETag      string   `json:"etag,omitempty"`
	CheckedAt int64    `json:"checkedAt,omitempty"`
	UpdatedAt int64    `json:"updatedAt,omitempty"`
	Error     string   `json:"error,omitempty"`
}
type packageCache struct {
	Sources map[string]packageSourceCache `json:"sources"`
}

// Never overwrite a corrupt cache with empty data. Keep the last published
// snapshot intact and report the corruption for manual intervention.
func packageLoadCache() (packageCache, error) {
	cache := packageCache{}
	raw, err := packageSettings.getString(packageCacheKey)
	if err != nil {
		return cache, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cache); err != nil {
			return packageCache{}, fmt.Errorf("invalid package exclusion cache (preserved): %w", err)
		}
	}
	if cache.Sources == nil {
		cache.Sources = make(map[string]packageSourceCache)
	}
	return cache, nil
}

func packagePrune(cache packageCache, sources []string) packageCache {
	out := packageCache{Sources: make(map[string]packageSourceCache, len(sources))}
	for _, source := range sources {
		if entry, ok := cache.Sources[source]; ok {
			out.Sources[source] = entry
		}
	}
	return out
}

// preparedCount mirrors what the JSON composer will emit. The default
// subscription has one TUN; a custom template may contain several.
func preparedCount(cfg map[string]interface{}, s *packageSnapshot) (int, int) {
	inbounds, _ := cfg["inbounds"].([]interface{})
	if _, exists := cfg["inbounds"]; !exists {
		return len(s.packages), len(s.blocked)
	}
	if len(inbounds) == 0 {
		return 0, len(s.blocked)
	}
	final := make(map[string]bool)
	removed := make(map[string]bool, len(s.blocked))
	for id := range s.blocked {
		removed[id] = true
	}
	hasTun := false
	for _, raw := range inbounds {
		tun, ok := raw.(map[string]interface{})
		if !ok || tun["type"] != "tun" {
			continue
		}
		hasTun = true
		for _, id := range stringPackages(tun["exclude_package"]) {
			if !validPackageID(id) {
				continue
			}
			if s.protected[id] {
				removed[id] = true
			} else {
				final[id] = true
			}
		}
		for _, id := range s.packages {
			final[id] = true
		}
	}
	if !hasTun {
		return 0, len(removed)
	}
	return len(final), len(removed)
}

func packageCompile(sources []string, cache packageCache) (*packageSnapshot, error) {
	if len(sources) == 0 {
		return &packageSnapshot{}, nil
	}
	template, err := packageSettings.GetSubJsonExt()
	if err != nil {
		return nil, err
	}
	cfg := make(map[string]interface{})
	if strings.TrimSpace(template) != "" {
		if err := json.Unmarshal([]byte(template), &cfg); err != nil {
			return nil, fmt.Errorf("subJsonExt is not valid JSON: %w", err)
		}
	}
	protected := make(map[string]bool)
	protectedPackageIDs(cfg, protected)
	snap := &packageSnapshot{enabled: true, protected: protected, blocked: make(map[string]bool)}
	seen := make(map[string]bool)
	for _, source := range sources {
		for _, id := range cache.Sources[source].Packages {
			if !validPackageID(id) {
				return nil, fmt.Errorf("invalid cached Android package ID %q", id)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			if protected[id] {
				snap.blocked[id] = true
				continue
			}
			snap.packages = append(snap.packages, id)
			if len(snap.packages) > maxPackageCount {
				return nil, errors.New("too many merged package IDs")
			}
		}
	}
	snap.prepared, snap.conflicts = preparedCount(cfg, snap)
	return snap, nil
}

func packageCommit(sources []string, cache packageCache) error {
	snap, err := packageCompile(sources, cache)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	if err := packageSettings.setString(packageCacheKey, string(raw)); err != nil {
		return err
	}
	packageCurrent.Store(snap)
	return nil
}

func packageRebuild() error {
	packageLock.Lock()
	defer packageLock.Unlock()
	sources, err := packageSources()
	if err != nil {
		return err
	}
	cache, err := packageLoadCache()
	if err != nil {
		return err
	}
	return packageCommit(sources, packagePrune(cache, sources))
}

func packageFetch(ctx context.Context, source string, old packageSourceCache) (packageSourceCache, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return old, err
	}
	if len(old.Packages) > 0 && old.ETag != "" {
		req.Header.Set("If-None-Match", old.ETag)
	}
	req.Header.Set("Accept", "application/json,text/plain;q=0.9,*/*;q=0.1")
	resp, err := packageHTTP.Do(req)
	if err != nil {
		return old, err
	}
	defer resp.Body.Close()
	now := time.Now().Unix()
	if resp.StatusCode == http.StatusNotModified && len(old.Packages) > 0 {
		old.CheckedAt, old.Error = now, ""
		return old, nil
	}
	if resp.StatusCode != http.StatusOK {
		return old, fmt.Errorf("remote source returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPackageFile+1))
	if err != nil {
		return old, err
	}
	if len(body) > maxPackageFile {
		return old, errors.New("remote package file exceeds 512 KiB")
	}
	ids, err := parsePackageFile(body)
	if err != nil {
		return old, err
	}
	if !slices.Equal(old.Packages, ids) {
		old.UpdatedAt = now
	}
	old.Packages, old.ETag, old.CheckedAt, old.Error = ids, resp.Header.Get("ETag"), now, ""
	return old, nil
}

func packageRefresh(ctx context.Context, want func(string, packageSourceCache) bool) error {
	packageLock.Lock()
	defer packageLock.Unlock()
	sources, err := packageSources()
	if err != nil {
		return err
	}
	cache, err := packageLoadCache()
	if err != nil {
		return err
	}
	cache = packagePrune(cache, sources)
	var jobs []string
	for _, source := range sources {
		if want(source, cache.Sources[source]) {
			jobs = append(jobs, source)
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	fetched := make([]packageSourceCache, len(jobs))
	failed := make([]error, len(jobs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, source := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, source string) {
			defer wg.Done()
			defer func() { <-sem }()
			fetched[i], failed[i] = packageFetch(ctx, source, cache.Sources[source])
		}(i, source)
	}
	wg.Wait()
	var issues []error
	for i, source := range jobs {
		entry := fetched[i]
		if failed[i] != nil {
			entry.Error = failed[i].Error()
			issues = append(issues, fmt.Errorf("%s: %w", source, failed[i]))
		}
		cache.Sources[source] = entry
	}
	// Settings can be saved while downloading. Never publish stale sources.
	current, checkErr := packageSources()
	if checkErr != nil {
		return errors.Join(append(issues, checkErr)...)
	}
	if !slices.Equal(current, sources) {
		return errors.Join(issues...)
	}
	return errors.Join(append(issues, packageCommit(sources, cache))...)
}

func packageMissing(_ string, cached packageSourceCache) bool { return len(cached.Packages) == 0 }

func PackageExcludeReload() {
	if err := packageRebuild(); err != nil {
		logger.Warning("package exclusions: rebuild failed, keeping previous snapshot: ", err)
		return // A corrupt snapshot/cache must not trigger destructive writes.
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := packageRefresh(ctx, packageMissing); err != nil {
			logger.Warning("package exclusions: initial download failed: ", err)
		}
	}()
}

func PackageExcludeRefresh(ctx context.Context) error {
	sources, err := packageSources()
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return errors.New("configure a public HTTPS file URL first")
	}
	return packageRefresh(ctx, func(string, packageSourceCache) bool { return true })
}

func PackageExcludeRefreshSource(ctx context.Context, source string) error {
	want, err := packageSourceURL(source)
	if err != nil {
		return err
	}
	sources, err := packageSources()
	if err != nil {
		return err
	}
	if !slices.Contains(sources, want) {
		return errors.New("remote source is not configured")
	}
	return packageRefresh(ctx, func(s string, _ packageSourceCache) bool { return s == want })
}

func PackageExcludeRefreshIfDue(ctx context.Context) error {
	schedule, err := packageSettings.getString(packageScheduleKey)
	if err != nil {
		return err
	}
	days, err := packageScheduleDays(schedule)
	if err != nil {
		return err
	}
	if days == 0 {
		return nil
	}
	interval := time.Duration(days) * 24 * time.Hour
	return packageRefresh(ctx, func(_ string, cached packageSourceCache) bool {
		return len(cached.Packages) == 0 || time.Since(time.Unix(cached.CheckedAt, 0)) >= interval
	})
}

func PackageExcludeStatus() (map[string]interface{}, error) {
	sources, err := packageSources()
	if err != nil {
		return nil, err
	}
	cache, err := packageLoadCache()
	if err != nil {
		return nil, err
	}
	schedule, err := packageSettings.getString(packageScheduleKey)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(sources))
	var checked, updated int64
	for _, source := range sources {
		c := cache.Sources[source]
		checked, updated = max(checked, c.CheckedAt), max(updated, c.UpdatedAt)
		items = append(items, map[string]interface{}{
			"url": source, "cached": len(c.Packages), "checkedAt": c.CheckedAt,
			"updatedAt": c.UpdatedAt, "error": c.Error, "loaded": len(c.Packages) > 0,
		})
	}
	prepared, conflicts := 0, 0
	if s := packageCurrent.Load(); s != nil {
		prepared, conflicts = s.prepared, s.conflicts
	}
	return map[string]interface{}{
		"sources": items, "schedule": schedule, "prepared": prepared,
		"conflicts": conflicts, "checkedAt": checked, "updatedAt": updated,
	}, nil
}
