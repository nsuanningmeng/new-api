package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func ScopeGuard(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, err := service.BuildDataScope(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
			return
		}
		if requiredRole != "" && scope.ActorRole != requiredRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
			return
		}
		c.Set(string(constant.ContextKeyDataScope), scope)
		c.Set(string(constant.ContextKeyActorRole), scope.ActorRole)
		c.Set(string(constant.ContextKeyResellerId), scope.ResellerId)
		c.Next()
	}
}
