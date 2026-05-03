package controller

import (
	"strings"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type createTenantRequest struct {
	Code               string `json:"code" binding:"required"`
	Name               string `json:"name" binding:"required"`
	BrandName          string `json:"brand_name"`
	LogoUrl            string `json:"logo_url"`
	FaviconUrl         string `json:"favicon_url"`
	PrimaryColor       string `json:"primary_color"`
	SettlementMode     string `json:"settlement_mode"`
	SettlementCycle    string `json:"settlement_cycle"`
	SettlementCurrency string `json:"settlement_currency"`
	MinWithdrawAmount  int64  `json:"min_withdraw_amount"`
	MaxResellerLevel   int    `json:"max_reseller_level"`
	Remark             string `json:"remark"`
}

type updateTenantRequest struct {
	Name               *string `json:"name"`
	BrandName          *string `json:"brand_name"`
	LogoUrl            *string `json:"logo_url"`
	FaviconUrl         *string `json:"favicon_url"`
	PrimaryColor       *string `json:"primary_color"`
	Status             *int    `json:"status"`
	SettlementMode     *string `json:"settlement_mode"`
	SettlementCycle    *string `json:"settlement_cycle"`
	SettlementCurrency *string `json:"settlement_currency"`
	MinWithdrawAmount  *int64  `json:"min_withdraw_amount"`
	MaxResellerLevel   *int    `json:"max_reseller_level"`
	Remark             *string `json:"remark"`
}

func GetTenants(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var tenants []model.Tenant
	var total int64
	db := model.DB.Model(&model.Tenant{})
	db.Count(&total)
	if err := db.Offset((page - 1) * pageSize).Limit(pageSize).Find(&tenants).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"data": tenants, "total": total})
}

func GetTenant(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	var tenant model.Tenant
	if err := model.DB.First(&tenant, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, tenant)
}

func CreateTenant(c *gin.Context) {
	var req createTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorStr(c, "invalid params: "+err.Error())
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" {
		common.ApiErrorStr(c, "code and name are required")
		return
	}
	if req.MaxResellerLevel == 0 {
		req.MaxResellerLevel = 2
	}
	tenant := model.Tenant{
		Code:               req.Code,
		Name:               req.Name,
		BrandName:          req.BrandName,
		LogoUrl:            req.LogoUrl,
		FaviconUrl:         req.FaviconUrl,
		PrimaryColor:       req.PrimaryColor,
		SettlementMode:     req.SettlementMode,
		SettlementCycle:    req.SettlementCycle,
		SettlementCurrency: req.SettlementCurrency,
		MinWithdrawAmount:  req.MinWithdrawAmount,
		MaxResellerLevel:   req.MaxResellerLevel,
		Remark:             req.Remark,
		Status:             model.TenantStatusEnabled,
	}
	if err := model.DB.Create(&tenant).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateTenantDomainCache()
	common.ApiSuccess(c, tenant)
}

func UpdateTenant(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	var req updateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorStr(c, "invalid params")
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.BrandName != nil {
		updates["brand_name"] = *req.BrandName
	}
	if req.LogoUrl != nil {
		updates["logo_url"] = *req.LogoUrl
	}
	if req.FaviconUrl != nil {
		updates["favicon_url"] = *req.FaviconUrl
	}
	if req.PrimaryColor != nil {
		updates["primary_color"] = *req.PrimaryColor
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.SettlementMode != nil {
		updates["settlement_mode"] = *req.SettlementMode
	}
	if req.SettlementCycle != nil {
		updates["settlement_cycle"] = *req.SettlementCycle
	}
	if req.SettlementCurrency != nil {
		updates["settlement_currency"] = *req.SettlementCurrency
	}
	if req.MinWithdrawAmount != nil {
		updates["min_withdraw_amount"] = *req.MinWithdrawAmount
	}
	if req.MaxResellerLevel != nil {
		updates["max_reseller_level"] = *req.MaxResellerLevel
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if len(updates) == 0 {
		common.ApiSuccess(c, nil)
		return
	}
	if err := model.DB.Model(&model.Tenant{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateTenantDomainCache()
	common.ApiSuccess(c, nil)
}

func DeleteTenant(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	var activeDomainCount int64
	if err := model.DB.Model(&model.TenantDomain{}).Where("tenant_id = ? AND status = ?", id, 1).Count(&activeDomainCount).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if activeDomainCount > 0 {
		common.ApiErrorStr(c, "please delete tenant domains first")
		return
	}
	var activeUserCount int64
	if err := model.DB.Model(&model.User{}).Where("tenant_id = ?", id).Count(&activeUserCount).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if activeUserCount > 0 {
		common.ApiErrorStr(c, "please delete or migrate active users first")
		return
	}
	if err := model.DB.Delete(&model.Tenant{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateTenantDomainCache()
	common.ApiSuccess(c, nil)
}

func GetTenantDomains(c *gin.Context) {
	tenantId, err := strconv.Atoi(c.Param("id"))
	if err != nil || tenantId <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	var domains []model.TenantDomain
	if err := model.DB.Where("tenant_id = ?", tenantId).Find(&domains).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, domains)
}

func CreateTenantDomain(c *gin.Context) {
	tenantId, err := strconv.Atoi(c.Param("id"))
	if err != nil || tenantId <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	var req struct {
		Domain    string `json:"domain" binding:"required"`
		IsPrimary bool   `json:"is_primary"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorStr(c, "invalid params")
		return
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.Split(domain, "/")[0]
	domain = strings.Split(domain, ":")[0]
	if domain == "" {
		common.ApiErrorStr(c, "invalid domain")
		return
	}
	d := model.TenantDomain{
		TenantId:  tenantId,
		Domain:    domain,
		IsPrimary: req.IsPrimary,
		Status:    1,
	}
	if err := model.DB.Create(&d).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateTenantDomainCache()
	common.ApiSuccess(c, d)
}

func DeleteTenantDomain(c *gin.Context) {
	tenantId, err := strconv.Atoi(c.Param("id"))
	if err != nil || tenantId <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	domainId, err := strconv.Atoi(c.Param("domain_id"))
	if err != nil || domainId <= 0 {
		common.ApiErrorStr(c, "invalid domain_id")
		return
	}
	// verify domain belongs to this tenant
	if err := model.DB.Where("id = ? AND tenant_id = ?", domainId, tenantId).Delete(&model.TenantDomain{}).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateTenantDomainCache()
	common.ApiSuccess(c, nil)
}
