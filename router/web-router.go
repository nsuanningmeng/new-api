package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// ThemeAssets holds the embedded frontend assets.
type ThemeAssets struct {
	ClassicBuildFS   embed.FS
	ClassicIndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
	classicFS := common.EmbedFolder(assets.ClassicBuildFS, "web/classic/dist")

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	// SEO endpoints must run BEFORE static.Serve so that the dynamic
	// robots.txt / sitemap.xml win over any static fallback in dist/.
	router.Use(seoOverride)
	router.Use(static.Serve("/", classicFS))
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", assets.ClassicIndexPage)
	})
}

// seoOverride intercepts /robots.txt and /sitemap.xml so they are served by
// the dynamic SEO controllers instead of any embedded static asset.
func seoOverride(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		switch c.Request.URL.Path {
		case "/robots.txt":
			controller.RobotsTxt(c)
			c.Abort()
			return
		case "/sitemap.xml":
			controller.SitemapXml(c)
			c.Abort()
			return
		}
	}
	c.Next()
}
