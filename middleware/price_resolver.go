package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func PriceResolver() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantId := common.GetContextKeyInt(c, constant.ContextKeyTenantId)
		resellerId := common.GetContextKeyInt(c, constant.ContextKeyResellerId)
		userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
		group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)

		tenantId, resellerId = completeSaaSContextFromUser(userId, tenantId, resellerId)
		if tenantId == -1 {
			// user belongs to a different tenant than the host — skip SaaS pricing
			c.Next()
			return
		}

		if userId > 0 && modelName != "" {
			quote, err := service.ResolveSaaSPriceQuote(tenantId, resellerId, userId, modelName, group)
			if err != nil {
				logger.LogError(c, fmt.Sprintf("saas price resolve failed: %s", err.Error()))
			} else if quote != nil {
				common.SetContextKey(c, constant.ContextKeySaaSPriceQuote, quote)
			}
		}
		c.Next()
	}
}

func completeSaaSContextFromUser(userId, tenantId, resellerId int) (int, int) {
	if userId <= 0 {
		return tenantId, resellerId
	}
	var user model.User
	if err := model.DB.Select("tenant_id", "reseller_id").First(&user, userId).Error; err != nil {
		return tenantId, resellerId
	}
	// tenant isolation: if host resolved a tenant, user must belong to it
	if tenantId > 0 && user.TenantId > 0 && user.TenantId != tenantId {
		return -1, -1 // signal mismatch; caller will skip price resolution
	}
	if tenantId == 0 {
		tenantId = user.TenantId
	}
	if resellerId == 0 {
		resellerId = user.ResellerId
	}
	return tenantId, resellerId
}
