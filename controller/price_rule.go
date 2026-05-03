package controller

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	// 事务内创建 PriceRule + 同步 v1 PriceRuleVersion，让所有新规则一进库就有版本锚点。
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		rule.CurrentVersion = 1
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
		version := buildPriceRuleVersion(&rule, 1)
		return tx.Create(&version).Error
	})
	if err != nil {
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
	oldRule := rule
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

	// 价格字段变更 → expire 旧 active version + 创建新 version + 自增 PriceRule.CurrentVersion
	// 非价格字段变更（priority/status/remark）→ 原地 Save，不动 versions
	priceChanged := priceFieldsChanged(&oldRule, &rule)
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if priceChanged {
			now := time.Now().Unix()
			if err := tx.Model(&model.PriceRuleVersion{}).
				Where("rule_id = ? AND status = ? AND (expires_at = ? OR expires_at > ?)",
					rule.Id, priceRuleStatusEnabled, int64(0), now).
				Updates(map[string]any{"status": 0, "expires_at": now}).Error; err != nil {
				return err
			}
			rule.CurrentVersion = oldRule.CurrentVersion + 1
			if rule.CurrentVersion <= 1 {
				rule.CurrentVersion = 2
			}
		}
		if err := tx.Save(&rule).Error; err != nil {
			return err
		}
		if priceChanged {
			version := buildPriceRuleVersion(&rule, rule.CurrentVersion)
			version.EffectiveAt = time.Now().Unix()
			return tx.Create(&version).Error
		}
		return nil
	})
	if err != nil {
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
		return errors.New("invalid owner_type")
	}
	if rule.OwnerType != model.PriceRuleOwnerPlatform && rule.OwnerId <= 0 {
		return errors.New("owner_id is required")
	}
	if rule.ExpiredAt != 0 && rule.ExpiredAt <= rule.EffectiveAt {
		return errors.New("expired_at must be > effective_at")
	}
	return service.ValidatePriceMonotonic(&relaycommon.SaaSPriceQuote{
		PlatformCostQuota:       rule.PlatformCostQuota,
		TenantSettlementQuota:   rule.TenantSettlementQuota,
		ResellerSettlementQuota: rule.ResellerSettlementQuota,
		RetailPriceQuota:        rule.RetailPriceQuota,
		Currency:                rule.Currency,
	})
}

// buildPriceRuleVersion 把 rule 当前价目快照写入一条 PriceRuleVersion。
// EffectiveAt 默认沿用 rule.EffectiveAt，调用方可在 Update 路径覆盖为 now。
func buildPriceRuleVersion(rule *model.PriceRule, version int) model.PriceRuleVersion {
	return model.PriceRuleVersion{
		RuleId:                  rule.Id,
		Version:                 version,
		ModelName:               rule.ModelName,
		Group:                   rule.Group,
		BillingUnit:             rule.BillingUnit,
		PlatformCostQuota:       rule.PlatformCostQuota,
		TenantSettlementQuota:   rule.TenantSettlementQuota,
		ResellerSettlementQuota: rule.ResellerSettlementQuota,
		RetailPriceQuota:        rule.RetailPriceQuota,
		Currency:                rule.Currency,
		Status:                  priceRuleStatusEnabled,
		EffectiveAt:             rule.EffectiveAt,
		ExpiresAt:               rule.ExpiredAt,
	}
}

// priceFieldsChanged 判断关键价格/作用域字段是否变化；只有变化才触发 expire-and-create。
// 不计入：Priority / Status / Remark（这些只是元数据，不影响订单结算）。
func priceFieldsChanged(old, neu *model.PriceRule) bool {
	if old.PlatformCostQuota != neu.PlatformCostQuota ||
		old.TenantSettlementQuota != neu.TenantSettlementQuota ||
		old.ResellerSettlementQuota != neu.ResellerSettlementQuota ||
		old.RetailPriceQuota != neu.RetailPriceQuota {
		return true
	}
	if old.ModelName != neu.ModelName || old.Group != neu.Group ||
		old.BillingUnit != neu.BillingUnit || old.Currency != neu.Currency {
		return true
	}
	if old.EffectiveAt != neu.EffectiveAt || old.ExpiredAt != neu.ExpiredAt {
		return true
	}
	return false
}
