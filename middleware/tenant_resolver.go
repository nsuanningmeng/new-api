package middleware

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TenantResolver resolves tenant from Host only.
// Sets saas_tenant_id/saas_tenant_code in context. Non-blocking: missing tenant means platform-direct (id=0).
func TenantResolver() gin.HandlerFunc {
	return func(c *gin.Context) {
		host := strings.ToLower(strings.Split(c.Request.Host, ":")[0])

		tenant, err := service.ResolveTenantByHost(host)
		if err != nil {
			// real DB error: fail-closed to prevent cross-tenant data leakage
			c.AbortWithStatusJSON(503, gin.H{"success": false, "message": "service unavailable"})
			return
		}

		if tenant != nil {
			if tenant.Status != model.TenantStatusEnabled {
				c.AbortWithStatusJSON(403, gin.H{"success": false, "message": "tenant is disabled"})
				return
			}
			c.Set(string(constant.ContextKeyTenantId), tenant.Id)
			c.Set(string(constant.ContextKeyTenantCode), tenant.Code)
		}
		c.Next()
	}
}
