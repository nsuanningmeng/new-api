package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/seo"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	baiduPushBodyLimit = 256 << 10 // 256 KiB cap for the manual push body
	maxPushURLsPerCall = 2000
)

// sitemapLastMod is fixed at process start so the sitemap doesn't appear to
// change every day (which would otherwise mislead crawlers about freshness).
var sitemapLastMod = time.Now().UTC().Format("2006-01-02")

// RobotsTxt renders robots.txt dynamically, embedding a Sitemap: line
// resolved from the runtime ServerAddress.
func RobotsTxt(c *gin.Context) {
	cfg := system_setting.GetSEOSettings()
	base := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")

	var sb strings.Builder
	sb.WriteString("# https://www.robotstxt.org/robotstxt.html\n")
	sb.WriteString("User-agent: *\n")
	sb.WriteString("Disallow: /api/\n")
	sb.WriteString("Disallow: /v1/\n")
	sb.WriteString("Disallow: /panel/\n")
	sb.WriteString("Disallow: /console/\n")
	sb.WriteString("Disallow: /admin/\n")
	sb.WriteString("Allow: /\n")
	if cfg.SitemapEnabled && base != "" {
		sb.WriteString(fmt.Sprintf("\nSitemap: %s/sitemap.xml\n", base))
	}

	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(sb.String()))
}

// SitemapXml renders sitemap.xml from configured public paths.
// Returns 404 when sitemap is disabled or ServerAddress is unset.
func SitemapXml(c *gin.Context) {
	cfg := system_setting.GetSEOSettings()
	if !cfg.SitemapEnabled {
		c.Status(http.StatusNotFound)
		return
	}
	urls := seo.BuildSitemapURLs()
	if len(urls) == 0 {
		c.Status(http.StatusNotFound)
		return
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for i, u := range urls {
		priority := "0.5"
		if i == 0 {
			priority = "1.0"
		}
		sb.WriteString("  <url>\n")
		sb.WriteString("    <loc>" + xmlEscape(u) + "</loc>\n")
		sb.WriteString("    <lastmod>" + sitemapLastMod + "</lastmod>\n")
		sb.WriteString("    <changefreq>weekly</changefreq>\n")
		sb.WriteString("    <priority>" + priority + "</priority>\n")
		sb.WriteString("  </url>\n")
	}
	sb.WriteString(`</urlset>` + "\n")

	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(sb.String()))
}

type baiduPushRequest struct {
	Urls []string `json:"urls"`
}

// BaiduPush proxies the Baidu Search Resource Platform "urls" push API.
// Caller must be root (enforced by router middleware).
// Body { "urls": [...] } is optional — empty falls back to sitemap paths.
//
// Body is hard-capped to 256 KiB and the URL list is capped to 2000 entries
// to prevent memory abuse on the management endpoint. Malformed JSON is
// rejected with 400 instead of being silently treated as an empty body —
// silent fallback was an attack vector for accidentally re-pushing the
// default sitemap on every malformed call.
func BaiduPush(c *gin.Context) {
	if c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, baiduPushBodyLimit)
	}

	var req baiduPushRequest
	if c.Request.ContentLength != 0 {
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			common.ApiErrorStr(c, "request body too large or unreadable")
			return
		}
		if len(raw) > 0 {
			if err := common.Unmarshal(raw, &req); err != nil {
				common.ApiErrorStr(c, "invalid json body")
				return
			}
		}
	}
	if len(req.Urls) > maxPushURLsPerCall {
		common.ApiErrorStr(c, fmt.Sprintf("too many urls (max %d)", maxPushURLsPerCall))
		return
	}

	result, err := seo.PushToBaidu(req.Urls)
	if err != nil {
		// Generalize upstream errors to avoid surfacing raw Baidu diagnostic
		// payloads (which may include account hints) to the API caller.
		common.SysLog(fmt.Sprintf("baidu push manual failed: %v", err))
		common.ApiError(c, errors.New("baidu push failed, see server logs"))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
