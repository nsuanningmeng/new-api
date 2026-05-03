package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ActorRolePlatformAdmin = "platform_admin"
	ActorRoleTenantAdmin   = "tenant_admin"
	ActorRoleResellerL1    = "reseller_l1"
	ActorRoleResellerL2    = "reseller_l2"
	ActorRoleEndUser       = "end_user"
)

type DataScope struct {
	ActorUserId int
	ActorRole   string
	TenantId    int
	ResellerId  int
	DownlineIds []int
	UserId      int
}

func scopedColumn(tableAlias, column string) string {
	if strings.TrimSpace(tableAlias) == "" {
		return column
	}
	// whitelist: only allow simple identifiers to prevent SQL injection
	for _, c := range tableAlias {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return column
		}
	}
	return tableAlias + "." + column
}

func denyAll(tx *gorm.DB) *gorm.DB {
	return tx.Where("1 = 0")
}

func ApplyScope(tx *gorm.DB, scope DataScope, tableAlias string) *gorm.DB {
	tenantCol := scopedColumn(tableAlias, "tenant_id")
	resellerCol := scopedColumn(tableAlias, "reseller_id")
	userCol := scopedColumn(tableAlias, "user_id")

	switch scope.ActorRole {
	case ActorRolePlatformAdmin:
		return tx
	case ActorRoleTenantAdmin:
		if scope.TenantId <= 0 {
			return denyAll(tx)
		}
		return tx.Where(tenantCol+" = ?", scope.TenantId)
	case ActorRoleResellerL1:
		if scope.TenantId <= 0 || scope.ResellerId <= 0 {
			return denyAll(tx)
		}
		ids := append([]int{scope.ResellerId}, scope.DownlineIds...)
		return tx.Where(tenantCol+" = ? AND "+resellerCol+" IN ?", scope.TenantId, ids)
	case ActorRoleResellerL2:
		if scope.TenantId <= 0 || scope.ResellerId <= 0 {
			return denyAll(tx)
		}
		return tx.Where(tenantCol+" = ? AND "+resellerCol+" = ?", scope.TenantId, scope.ResellerId)
	case ActorRoleEndUser:
		if scope.UserId <= 0 {
			return denyAll(tx)
		}
		return tx.Where(userCol+" = ?", scope.UserId)
	default:
		return denyAll(tx)
	}
}

func BuildDataScope(c *gin.Context) (DataScope, error) {
	actorUserId := c.GetInt(string(constant.ContextKeyUserId))
	scope := DataScope{
		ActorUserId: actorUserId,
		ActorRole:   strings.TrimSpace(c.GetString(string(constant.ContextKeyActorRole))),
		TenantId:    c.GetInt(string(constant.ContextKeyTenantId)),
		ResellerId:  c.GetInt(string(constant.ContextKeyResellerId)),
		UserId:      actorUserId,
	}
	if actorUserId <= 0 {
		return scope, errors.New("missing actor user id")
	}

	var user model.User
	if err := model.DB.Select("id", "role", "tenant_id", "reseller_id").First(&user, actorUserId).Error; err != nil {
		return scope, err
	}
	if scope.TenantId == 0 {
		scope.TenantId = user.TenantId
	}
	if scope.ResellerId == 0 {
		scope.ResellerId = user.ResellerId
	}

	if user.Role >= common.RoleRootUser {
		scope.ActorRole = ActorRolePlatformAdmin
		return scope, nil
	}

	if scope.TenantId <= 0 {
		scope.ActorRole = ActorRoleEndUser
		return scope, nil
	}
	if user.TenantId > 0 && user.TenantId != scope.TenantId {
		return scope, errors.New("user does not belong to current tenant")
	}

	var tenant model.Tenant
	if err := model.DB.Select("id", "owner_user_id", "status").First(&tenant, scope.TenantId).Error; err != nil {
		return scope, err
	}
	if tenant.Status != model.TenantStatusEnabled {
		return scope, errors.New("tenant is disabled")
	}
	isTenantAdmin := tenant.OwnerUserId == actorUserId || (user.Role >= common.RoleAdminUser && user.TenantId == scope.TenantId)

	var reseller *model.Reseller
	if scope.ResellerId == 0 {
		var owned model.Reseller
		err := model.DB.Where("tenant_id = ? AND user_id = ? AND status = ?", scope.TenantId, actorUserId, model.ResellerStatusEnabled).
			Order("level asc").First(&owned).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return scope, err
		}
		if err == nil {
			scope.ResellerId = owned.Id
			reseller = &owned
		}
	} else {
		var found model.Reseller
		if err := model.DB.First(&found, scope.ResellerId).Error; err != nil {
			return scope, err
		}
		if found.TenantId != scope.TenantId {
			return scope, errors.New("reseller does not belong to current tenant")
		}
		if found.Status != model.ResellerStatusEnabled {
			return scope, errors.New("reseller is disabled")
		}
		if !isTenantAdmin && found.UserId != actorUserId {
			return scope, errors.New("reseller does not belong to actor")
		}
		reseller = &found
	}

	if reseller != nil && reseller.Level == 1 {
		var downlineIds []int
		if err := model.DB.Model(&model.Reseller{}).
			Where("tenant_id = ? AND parent_reseller_id = ? AND status = ?", scope.TenantId, reseller.Id, model.ResellerStatusEnabled).
			Pluck("id", &downlineIds).Error; err != nil {
			return scope, err
		}
		scope.DownlineIds = downlineIds
	}

	switch {
	case isTenantAdmin:
		scope.ActorRole = ActorRoleTenantAdmin
	case reseller != nil && reseller.Level == 1:
		scope.ActorRole = ActorRoleResellerL1
	case reseller != nil && reseller.Level == 2:
		scope.ActorRole = ActorRoleResellerL2
	default:
		scope.ActorRole = ActorRoleEndUser
	}
	return scope, nil
}
