package seo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	// HTTPS endpoint to avoid plaintext token-in-query exposure on the wire.
	baiduPushEndpoint = "https://data.zz.baidu.com/urls"
	pushTimeout       = 8 * time.Second
	maxResponseBytes  = 64 * 1024
	maxBatchURLs      = 2000
	maxURLLength      = 1024
	maxPathLength     = 512

	baiduPushTickInterval     = 5 * time.Minute
	baiduPushStartupDelay     = 5 * time.Minute
	baiduPushFailBackoff      = 30 * time.Minute
	baiduPushMinIntervalHours = 1
	baiduPushMaxIntervalHours = 720 // 30 days
)

// pathDenyPrefixes blocks server-internal / admin paths from ever leaking
// into a sitemap or push payload, even if an operator misconfigures
// SitemapPaths or supplies arbitrary urls via the manual API.
var pathDenyPrefixes = []string{
	"/api/",
	"/v1/",
	"/admin/",
	"/admin",
	"/console/",
	"/console",
	"/panel/",
	"/panel",
}

var (
	baiduPushOnce    sync.Once
	baiduPushRunning atomic.Bool
	baiduPushNextAt  atomic.Int64 // unix seconds; 0 = run on next tick
	pushMu           sync.Mutex   // serializes PushToBaidu across manual + cron callers
)

// PushResult mirrors the JSON envelope returned by Baidu's push API.
// Fields not used by callers are kept for diagnostic logging.
type PushResult struct {
	Remain   int      `json:"remain"`
	Success  int      `json:"success"`
	NotSame  []string `json:"not_same,omitempty"`
	NotValid []string `json:"not_valid,omitempty"`
	Error    int      `json:"error,omitempty"`
	Message  string   `json:"message,omitempty"`
}

// hasControlChar returns true if s contains any ASCII control character.
// This guards against CR/LF injection that would inject extra URLs into
// Baidu's newline-delimited POST body.
func hasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// pathAllowed enforces the deny prefix list, length cap, and forbids
// control characters / scheme-bearing inputs in configured paths.
func pathAllowed(p string) bool {
	if p == "" || len(p) > maxPathLength {
		return false
	}
	if hasControlChar(p) {
		return false
	}
	if strings.Contains(p, "://") {
		return false
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for _, deny := range pathDenyPrefixes {
		if strings.HasPrefix(p, deny) {
			return false
		}
	}
	return true
}

// BuildSitemapURLs renders the configured sitemap paths into absolute URLs
// using the runtime ServerAddress. Empty / unconfigured input yields nil.
// Paths are validated and deny-listed.
func BuildSitemapURLs() []string {
	cfg := system_setting.GetSEOSettings()
	base := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if base == "" {
		return nil
	}
	out := make([]string, 0, len(cfg.SitemapPaths))
	for _, p := range cfg.SitemapPaths {
		p = strings.TrimSpace(p)
		if !pathAllowed(p) {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		out = append(out, base+p)
	}
	return out
}

// validateAndNormalizePushURLs enforces strict per-URL hygiene to prevent
// CRLF injection into Baidu's newline-delimited body, cross-site URL
// smuggling (someone configures a token then submits arbitrary external
// URLs to burn our quota), and admin-path leaks.
func validateAndNormalizePushURLs(urls []string, expectedHost string) ([]string, error) {
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if len(raw) > maxURLLength || hasControlChar(raw) {
			return nil, fmt.Errorf("invalid url: control chars or too long")
		}
		u, err := url.Parse(raw)
		if err != nil || u == nil {
			return nil, fmt.Errorf("invalid url: %s", raw)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("invalid url scheme: %s", u.Scheme)
		}
		if u.Host == "" {
			return nil, fmt.Errorf("invalid url: missing host")
		}
		if expectedHost != "" && !strings.EqualFold(u.Host, expectedHost) {
			return nil, fmt.Errorf("url host %q does not match site %q", u.Host, expectedHost)
		}
		if !pathAllowed(u.Path) {
			return nil, fmt.Errorf("url path is denied: %s", u.Path)
		}
		out = append(out, u.String())
	}
	return out, nil
}

// extractHost normalizes a configured site (which may include scheme) to a
// bare host. Returns "" when no host can be parsed (treated as "any").
func extractHost(site string) string {
	site = strings.TrimSpace(site)
	if site == "" {
		return ""
	}
	if !strings.Contains(site, "://") {
		// bare "example.com" or "example.com/"
		site = "http://" + site
	}
	if u, err := url.Parse(site); err == nil {
		return u.Host
	}
	return ""
}

// PushToBaidu submits given URLs to Baidu Search Resource Platform.
// Pass nil/empty to fall back to the configured sitemap paths.
// Serialized via pushMu so manual + cron callers cannot duplicate-submit.
func PushToBaidu(urls []string) (*PushResult, error) {
	pushMu.Lock()
	defer pushMu.Unlock()

	cfg := system_setting.GetSEOSettings()
	site := strings.TrimSpace(cfg.BaiduPushSite)
	token := strings.TrimSpace(cfg.BaiduPushToken)
	if site == "" || token == "" {
		return nil, fmt.Errorf("baidu_push_site or baidu_push_token not configured")
	}

	if len(urls) == 0 {
		urls = BuildSitemapURLs()
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no urls to push")
	}
	if len(urls) > maxBatchURLs {
		urls = urls[:maxBatchURLs]
	}

	expectedHost := extractHost(site)
	cleanURLs, err := validateAndNormalizePushURLs(urls, expectedHost)
	if err != nil {
		return nil, err
	}
	if len(cleanURLs) == 0 {
		return nil, fmt.Errorf("no valid urls after sanitization")
	}

	body := strings.Join(cleanURLs, "\n")
	pushURL := fmt.Sprintf("%s?site=%s&token=%s",
		baiduPushEndpoint,
		url.QueryEscape(site),
		url.QueryEscape(token),
	)

	req, err := http.NewRequest(http.MethodPost, pushURL, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: pushTimeout}
	resp, err := client.Do(req)
	if err != nil {
		// Avoid leaking the token-bearing URL through error chains.
		return nil, fmt.Errorf("baidu push request failed")
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if readErr != nil {
		return nil, fmt.Errorf("read baidu response failed: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		// Log full body server-side; surface only status to the caller.
		common.SysLog(fmt.Sprintf("baidu push http %d body=%s", resp.StatusCode, string(raw)))
		return nil, fmt.Errorf("baidu push http status %d", resp.StatusCode)
	}

	var result PushResult
	if err := common.Unmarshal(raw, &result); err != nil {
		common.SysLog(fmt.Sprintf("baidu push decode failed: %v raw=%s", err, string(raw)))
		return nil, fmt.Errorf("decode baidu response failed")
	}
	if result.Error != 0 {
		common.SysLog(fmt.Sprintf("baidu push api error %d: %s", result.Error, result.Message))
		return &result, fmt.Errorf("baidu push api error %d", result.Error)
	}

	// Successful push: bump cron's next-allowed timestamp so a manual
	// call doesn't immediately race with the auto task.
	cfgInterval := cfg.BaiduPushIntervalHours
	if cfgInterval < baiduPushMinIntervalHours {
		cfgInterval = baiduPushMinIntervalHours
	}
	if cfgInterval > baiduPushMaxIntervalHours {
		cfgInterval = baiduPushMaxIntervalHours
	}
	baiduPushNextAt.Store(time.Now().Unix() + int64(cfgInterval)*3600)

	common.SysLog(fmt.Sprintf("baidu push success=%d remain=%d urls=%d",
		result.Success, result.Remain, len(cleanURLs)))
	return &result, nil
}

// StartBaiduPushAutoTask launches the daemon goroutine that periodically
// pushes sitemap URLs to Baidu when BaiduPushAuto is enabled.
//
// Tick frequency is 5 min (so config toggles take effect quickly), but the
// actual push cadence is governed by BaiduPushIntervalHours. Failure backs
// off for 30 min before retry. Master node only.
func StartBaiduPushAutoTask() {
	baiduPushOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			time.Sleep(baiduPushStartupDelay)
			logger.LogInfo(context.Background(),
				fmt.Sprintf("baidu push auto task started: tick=%s", baiduPushTickInterval))

			runBaiduPushAutoOnce()
			ticker := time.NewTicker(baiduPushTickInterval)
			defer ticker.Stop()
			for range ticker.C {
				runBaiduPushAutoOnce()
			}
		})
	})
}

func runBaiduPushAutoOnce() {
	if !baiduPushRunning.CompareAndSwap(false, true) {
		return
	}
	defer baiduPushRunning.Store(false)

	cfg := system_setting.GetSEOSettings()
	if !cfg.BaiduPushAuto {
		return
	}
	if strings.TrimSpace(cfg.BaiduPushSite) == "" || strings.TrimSpace(cfg.BaiduPushToken) == "" {
		return
	}

	interval := cfg.BaiduPushIntervalHours
	if interval < baiduPushMinIntervalHours {
		interval = baiduPushMinIntervalHours
	}
	if interval > baiduPushMaxIntervalHours {
		interval = baiduPushMaxIntervalHours
	}

	now := time.Now().Unix()
	if next := baiduPushNextAt.Load(); next > 0 && now < next {
		return
	}

	ctx := context.Background()
	result, err := PushToBaidu(nil)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("baidu push auto failed: %v", err))
		baiduPushNextAt.Store(now + int64(baiduPushFailBackoff.Seconds()))
		return
	}
	// PushToBaidu already advanced baiduPushNextAt on success.
	logger.LogInfo(ctx, fmt.Sprintf("baidu push auto ok: success=%d remain=%d next_in=%dh",
		result.Success, result.Remain, interval))
}
