package system_setting

import "github.com/QuantumNous/new-api/setting/config"

// SEOSettings holds SEO-related configuration that influences robots.txt,
// sitemap.xml and Baidu Search Resource Platform push behavior.
//
// Persisted via config.GlobalConfig under the "seo" namespace. Keys land in
// the existing options table (compatible with SQLite / MySQL / PostgreSQL).
type SEOSettings struct {
	SitemapEnabled         bool     `json:"sitemap_enabled"`
	SitemapPaths           []string `json:"sitemap_paths"`
	BaiduPushSite          string   `json:"baidu_push_site"`
	BaiduPushToken         string   `json:"baidu_push_token"`
	BaiduPushAuto          bool     `json:"baidu_push_auto"`
	BaiduPushIntervalHours int      `json:"baidu_push_interval_hours"`
}

var defaultSEOSettings = SEOSettings{
	SitemapEnabled:         true,
	SitemapPaths:           []string{"/", "/login", "/register", "/pricing"},
	BaiduPushSite:          "",
	BaiduPushToken:         "",
	BaiduPushAuto:          false,
	BaiduPushIntervalHours: 24,
}

func init() {
	config.GlobalConfig.Register("seo", &defaultSEOSettings)
}

func GetSEOSettings() *SEOSettings {
	return &defaultSEOSettings
}
