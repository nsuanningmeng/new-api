package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type createResellerRequest struct {
	UserId           int    `json:"user_id" binding:"required"`
	ParentResellerId *int   `json:"parent_reseller_id"`
	Name             string `json:"name"`
	ContactEmail     string `json:"contact_email"`
	SettlementMode   string `json:"settlement_mode"`
	CommissionConfig string `json:"commission_config"`
	Remark           string `json:"remark"`
}

type updateResellerRequest struct {
	Status           *int    `json:"status"`
	Name             *string `json:"name"`
	ContactEmail     *string `json:"contact_email"`
	SettlementMode   *string `json:"settlement_mode"`
	CommissionConfig *string `json:"commission_config"`
	Remark           *string `json:"remark"`
}

func getTenantAdminScope(c *gin.Context) (service.DataScope, bool) {
	scope, ok := common.GetContextKeyType[service.DataScope](c, constant.ContextKeyDataScope)
	return scope, ok && scope.ActorRole == service.ActorRoleTenantAdmin && scope.TenantId > 0
}

func GetResellers(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	var resellers []model.Reseller
	q := service.ApplyScope(model.DB.Model(&model.Reseller{}), scope, "resellers")
	q.Count(&total)
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&resellers).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"data": resellers, "total": total})
}

func GetReseller(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	var reseller model.Reseller
	if err := service.ApplyScope(model.DB.Where("id = ?", id), scope, "resellers").First(&reseller).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, reseller)
}

func CreateReseller(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	var req createResellerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid params: "+err.Error())
		return
	}

	var tenant model.Tenant
	if err := model.DB.First(&tenant, scope.TenantId).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	maxLevel := tenant.MaxResellerLevel
	if maxLevel <= 0 || maxLevel > 2 {
		maxLevel = 2
	}

	// verify user belongs to this tenant
	var user model.User
	if err := model.DB.Select("id", "tenant_id").Where("id = ? AND tenant_id = ?", req.UserId, scope.TenantId).First(&user).Error; err != nil {
		common.ApiErrorMsg(c, "user does not belong to current tenant")
		return
	}

	level := 1
	if req.ParentResellerId != nil {
		var parent model.Reseller
		if err := model.DB.Where("id = ? AND tenant_id = ?", *req.ParentResellerId, scope.TenantId).First(&parent).Error; err != nil {
			common.ApiErrorMsg(c, "parent reseller not found in current tenant")
			return
		}
		if parent.Level >= 2 {
			common.ApiErrorMsg(c, "parent reseller level must be less than 2")
			return
		}
		level = parent.Level + 1
	}
	if level > maxLevel {
		common.ApiErrorMsg(c, "reseller level exceeds tenant limit")
		return
	}

	reseller := model.Reseller{
		TenantId:         scope.TenantId,
		UserId:           req.UserId,
		ParentResellerId: req.ParentResellerId,
		Level:            level,
		Status:           model.ResellerStatusEnabled,
		Name:             strings.TrimSpace(req.Name),
		ContactEmail:     strings.TrimSpace(req.ContactEmail),
		SettlementMode:   strings.TrimSpace(req.SettlementMode),
		CommissionConfig: req.CommissionConfig,
		Remark:           strings.TrimSpace(req.Remark),
	}
	if err := model.DB.Create(&reseller).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, reseller)
}

func UpdateReseller(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	var req updateResellerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid params")
		return
	}
	updates := map[string]any{}
	if req.Status != nil {
		if *req.Status != model.ResellerStatusEnabled && *req.Status != model.ResellerStatusDisabled {
			common.ApiErrorMsg(c, "invalid status")
			return
		}
		updates["status"] = *req.Status
	}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.ContactEmail != nil {
		updates["contact_email"] = strings.TrimSpace(*req.ContactEmail)
	}
	if req.SettlementMode != nil {
		updates["settlement_mode"] = strings.TrimSpace(*req.SettlementMode)
	}
	if req.CommissionConfig != nil {
		updates["commission_config"] = *req.CommissionConfig
	}
	if req.Remark != nil {
		updates["remark"] = strings.TrimSpace(*req.Remark)
	}
	if len(updates) == 0 {
		common.ApiSuccess(c, nil)
		return
	}
	tx := service.ApplyScope(model.DB.Model(&model.Reseller{}).Where("id = ?", id), scope, "resellers").Updates(updates)
	if tx.Error != nil {
		common.ApiError(c, tx.Error)
		return
	}
	if tx.RowsAffected == 0 {
		common.ApiErrorMsg(c, "reseller not found")
		return
	}
	common.ApiSuccess(c, nil)
}

func DeleteReseller(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	tx := service.ApplyScope(model.DB.Where("id = ?", id), scope, "resellers").Delete(&model.Reseller{})
	if tx.Error != nil {
		common.ApiError(c, tx.Error)
		return
	}
	if tx.RowsAffected == 0 {
		common.ApiErrorMsg(c, "reseller not found")
		return
	}
	common.ApiSuccess(c, nil)
}
