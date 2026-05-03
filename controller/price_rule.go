package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const priceRuleStatusEnabled = 1

type createPriceRuleRequest struct {
	OwnerType               string `json:"owner_type" binding:"required"`
	OwnerId                 int    `json:"owner_id"`
	TenantId                int    `json:"tenant_id"`
	ResellerId              int    `json:"reseller_id"`
	ModelName               string `json:"model_name"`
	Group                   string `json:"group"`
	BillingUnit             string `json:"billing_unit"`
	PlatformCostQuota       int64  `json:"platform_cost_quota"`
	TenantSettlementQuota   int64  `json:"tenant_settlement_quota"`
	ResellerSettlementQuota int64  `json:"reseller_settlement_quota"`
	RetailPriceQuota        int64  `json:"retail_price_quota"`
	Currency                string `json:"currency"`
	Priority                int    `json:"priority"`
	Status                  int    `json:"status"`
	EffectiveAt             int64  `json:"effective_at"`
	ExpiredAt               int64  `json:"expired_at"`
	Remark                  string `json:"remark"`
}

type updatePriceRuleRequest struct {
	ModelName               *string `json:"model_name"`
	Group                   *string `json:"group"`
	BillingUnit             *string `json:"billing_unit"`
	PlatformCostQuota       *int64  `json:"platform_cost_quota"`
	TenantSettlementQuota   *int64  `json:"tenant_settlement_quota"`
	ResellerSettlementQuota *int64  `json:"reseller_settlement_quota"`
	RetailPriceQuota        *int64  `json:"retail_price_quota"`
	Currency                *string `json:"currency"`
	Priority                *int    `json:"priority"`
	Status                  *int    `json:"status"`
	EffectiveAt             *int64  `json:"effective_at"`
	ExpiredAt               *int64  `json:"expired_at"`
	Remark                  *string `json:"remark"`
}

func GetPriceRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	db := model.DB.Model(&model.PriceRule{})
	if v := strings.TrimSpace(c.Query("owner_type")); v != "" {
		db = db.Where("owner_type = ?", v)
	}
	if v, _ := strconv.Atoi(c.Query("tenant_id")); v > 0 {
		db = db.Where("tenant_id = ?", v)
	}
	if v := strings.TrimSpace(c.Query("model_name")); v != "" {
		db = db.Where("model_name = ?", v)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var rules []model.PriceRule
	if err := db.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rules).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"data": rules, "total": total})
}

func CreatePriceRule(c *gin.Context) {
	var req createPriceRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorStr(c, "invalid params: "+err.Error())
		return
	}
	rule := model.PriceRule{
		OwnerType: strings.TrimSpace(req.OwnerType), OwnerId: req.OwnerId,
		TenantId: req.TenantId, ResellerId: req.ResellerId,
		ModelName: strings.TrimSpace(req.ModelName), Group: strings.TrimSpace(req.Group),
		BillingUnit: strings.TrimSpace(req.BillingUnit),
		PlatformCostQuota: req.PlatformCostQuota, TenantSettlementQuota: req.TenantSettlementQuota,
		ResellerSettlementQuota: req.ResellerSettlementQuota, RetailPriceQuota: req.RetailPriceQuota,
		Currency: strings.TrimSpace(req.Currency), Priority: req.Priority,
		Status: req.Status, EffectiveAt: req.EffectiveAt, ExpiredAt: req.ExpiredAt, Remark: req.Remark,
	}
	normalizePriceRule(&rule)
	if err := validatePriceRule(&rule); err != nil {
		common.ApiErrorStr(c, err.Error())
		return
	}
	if err := model.DB.Create(&rule).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func UpdatePriceRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	var rule model.PriceRule
	if err := model.DB.First(&rule, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var req updatePriceRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorStr(c, "invalid params")
		return
	}
	if req.ModelName != nil {
		rule.ModelName = strings.TrimSpace(*req.ModelName)
	}
	if req.Group != nil {
		rule.Group = strings.TrimSpace(*req.Group)
	}
	if req.BillingUnit != nil {
		rule.BillingUnit = strings.TrimSpace(*req.BillingUnit)
	}
	if req.PlatformCostQuota != nil {
		rule.PlatformCostQuota = *req.PlatformCostQuota
	}
	if req.TenantSettlementQuota != nil {
		rule.TenantSettlementQuota = *req.TenantSettlementQuota
	}
	if req.ResellerSettlementQuota != nil {
		rule.ResellerSettlementQuota = *req.ResellerSettlementQuota
	}
	if req.RetailPriceQuota != nil {
		rule.RetailPriceQuota = *req.RetailPriceQuota
	}
	if req.Currency != nil {
		rule.Currency = strings.TrimSpace(*req.Currency)
	}
	if req.Priority != nil {
		rule.Priority = *req.Priority
	}
	if req.Status != nil {
		rule.Status = *req.Status
	}
	if req.EffectiveAt != nil {
		rule.EffectiveAt = *req.EffectiveAt
	}
	if req.ExpiredAt != nil {
		rule.ExpiredAt = *req.ExpiredAt
	}
	if req.Remark != nil {
		rule.Remark = *req.Remark
	}
	normalizePriceRule(&rule)
	if err := validatePriceRule(&rule); err != nil {
		common.ApiErrorStr(c, err.Error())
		return
	}
	if err := model.DB.Save(&rule).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func DeletePriceRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorStr(c, "invalid id")
		return
	}
	if err := model.DB.Delete(&model.PriceRule{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func normalizePriceRule(rule *model.PriceRule) {
	if rule.ModelName == "" {
		rule.ModelName = "*"
	}
	if rule.Group == "" {
		rule.Group = "*"
	}
	if rule.BillingUnit == "" {
		rule.BillingUnit = "quota"
	}
	if rule.Currency == "" {
		rule.Currency = "USD"
	}
	if rule.Status == 0 {
		rule.Status = priceRuleStatusEnabled
	}
	if rule.Version == 0 {
		rule.Version = 1
	}
}

func validatePriceRule(rule *model.PriceRule) error {
	switch rule.OwnerType {
	case model.PriceRuleOwnerPlatform:
		rule.OwnerId = 0
	case model.PriceRuleOwnerTenant:
		if rule.OwnerId == 0 {
			rule.OwnerId = rule.TenantId
		}
	case model.PriceRuleOwnerReseller:
		if rule.OwnerId == 0 {
			rule.OwnerId = rule.ResellerId
		}
	case model.PriceRuleOwnerUser:
	default:
		return common.NewError("invalid owner_type")
	}
	if rule.OwnerType != model.PriceRuleOwnerPlatform && rule.OwnerId <= 0 {
		return common.NewError("owner_id is required")
	}
	if rule.ExpiredAt != 0 && rule.ExpiredAt <= rule.EffectiveAt {
		return common.NewError("expired_at must be > effective_at")
	}
	return service.ValidatePriceMonotonic(&relaycommon.SaaSPriceQuote{
		PlatformCostQuota:       rule.PlatformCostQuota,
		TenantSettlementQuota:   rule.TenantSettlementQuota,
		ResellerSettlementQuota: rule.ResellerSettlementQuota,
		RetailPriceQuota:        rule.RetailPriceQuota,
		Currency:                rule.Currency,
	})
}
