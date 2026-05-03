package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

const ticketUploadMaxBytes = 2 * 1024 * 1024

type ticketUploadCounter struct {
	window int64
	count  int
}

// 简单的内存型 token bucket：按分钟窗口聚合，每个 key 限频。
// 跨进程部署时不共享状态——前端有 client-side 预压缩 + 后端硬上限兜底，
// 单实例限频已可挡住绝大多数 abuse；分布式限频留作运维侧 nginx limit_req。
var ticketUploadLimiter = struct {
	sync.Mutex
	counters map[string]ticketUploadCounter
}{
	counters: make(map[string]ticketUploadCounter),
}

// TicketUploadLimit 拦截工单附件上传：
//   - Content-Length > 2MB 直接 413（不读 body）
//   - http.MaxBytesReader 兜底防止 Content-Length 谎报
//   - 用户/租户/全局三级速率限制，超限 429
func TicketUploadLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > ticketUploadMaxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "message": "file too large"})
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, ticketUploadMaxBytes+1024)

		userId := c.GetInt(string(constant.ContextKeyUserId))
		tenantId := c.GetInt(string(constant.ContextKeyTenantId))
		if userId <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
			return
		}

		if !allowTicketUpload(fmt.Sprintf("user:%d", userId), 10) ||
			!allowTicketUpload(fmt.Sprintf("tenant:%d", tenantId), 100) ||
			!allowTicketUpload("global", 1000) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "upload rate limit"})
			return
		}

		c.Next()
	}
}

func allowTicketUpload(key string, limit int) bool {
	nowWindow := time.Now().Unix() / 60

	ticketUploadLimiter.Lock()
	defer ticketUploadLimiter.Unlock()

	// 防止 map 长期膨胀：触发清理时只保留当前窗口
	if len(ticketUploadLimiter.counters) > 4096 {
		for k, v := range ticketUploadLimiter.counters {
			if v.window != nowWindow {
				delete(ticketUploadLimiter.counters, k)
			}
		}
	}

	counter := ticketUploadLimiter.counters[key]
	if counter.window != nowWindow {
		counter = ticketUploadCounter{window: nowWindow}
	}
	if counter.count >= limit {
		ticketUploadLimiter.counters[key] = counter
		return false
	}
	counter.count++
	ticketUploadLimiter.counters[key] = counter
	return true
}
