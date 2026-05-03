package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const saasPriceBaseQuota int64 = 1000000

func SettleSaaSProfit(tx *gorm.DB, relayInfo *relaycommon.RelayInfo, actualQuota int64) error {
	if relayInfo == nil || relayInfo.SaaSPriceQuote == nil {
		return nil
	}
	if actualQuota < 0 {
		return errors.New("actual quota must be >= 0")
	}
	if relayInfo.RequestId == "" {
		return errors.New("request_id required for SaaS settlement")
	}
	if err := ValidatePriceMonotonic(relayInfo.SaaSPriceQuote); err != nil {
		return err
	}
	if tx == nil {
		tx = model.DB
	}

	return tx.Transaction(func(db *gorm.DB) error {
		var existing model.PriceSnapshot
		if err := db.Where("request_id = ?", relayInfo.RequestId).First(&existing).Error; err == nil {
			fillRelaySaaSSettlement(relayInfo, existing)
			return nil // idempotent
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		q := relayInfo.SaaSPriceQuote
		platformCost := quotaByPrice(actualQuota, q.PlatformCostQuota)
		tenantSettlement := quotaByPrice(actualQuota, q.TenantSettlementQuota)
		resellerSettlement := quotaByPrice(actualQuota, q.ResellerSettlementQuota)
		retailPrice := quotaByPrice(actualQuota, q.RetailPriceQuota)

		platformProfit := tenantSettlement - platformCost
		tenantProfit := resellerSettlement - tenantSettlement
		resellerProfit := retailPrice - resellerSettlement
		if q.ResellerId == 0 {
			resellerSettlement = retailPrice
			resellerProfit = 0
		}

		snapshot := model.PriceSnapshot{
			RequestId:               relayInfo.RequestId,
			TenantId:                q.TenantId,
			ResellerId:              q.ResellerId,
			UserId:                  relayInfo.UserId,
			TokenId:                 relayInfo.TokenId,
			PriceRuleIds:            priceRuleIdsToString(q.PriceRuleIds),
			ModelName:               relayInfo.OriginModelName,
			Group:                   relayInfo.UsingGroup,
			ConsumedQuota:           actualQuota,
			PlatformCostSnapshot:    platformCost,
			TenantPriceSnapshot:     tenantSettlement,
			ResellerPriceSnapshot:   resellerSettlement,
			RetailPriceSnapshot:     retailPrice,
			PlatformProfit:          platformProfit,
			TenantProfit:            tenantProfit,
			ResellerProfit:          resellerProfit,
			Currency:                q.Currency,
			RuleId:                  q.RuleId,
			RuleVersion:             q.RuleVersion,
		}
		if snapshot.Currency == "" {
			snapshot.Currency = "USD"
		}

		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&snapshot)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		ledgers := []model.ProfitLedger{
			{PriceSnapshotId: snapshot.Id, RequestId: relayInfo.RequestId, TenantId: q.TenantId, ResellerId: q.ResellerId, BeneficiaryType: model.WalletAccountTypePlatform, BeneficiaryId: 0, ProfitType: model.ProfitTypePlatform, Amount: platformProfit, Currency: snapshot.Currency, SettlementStatus: model.ProfitSettlementStatusPending},
			{PriceSnapshotId: snapshot.Id, RequestId: relayInfo.RequestId, TenantId: q.TenantId, ResellerId: q.ResellerId, BeneficiaryType: model.WalletAccountTypeTenant, BeneficiaryId: q.TenantId, ProfitType: model.ProfitTypeTenant, Amount: tenantProfit, Currency: snapshot.Currency, SettlementStatus: model.ProfitSettlementStatusPending},
			{PriceSnapshotId: snapshot.Id, RequestId: relayInfo.RequestId, TenantId: q.TenantId, ResellerId: q.ResellerId, BeneficiaryType: model.WalletAccountTypeReseller, BeneficiaryId: q.ResellerId, ProfitType: model.ProfitTypeReseller, Amount: resellerProfit, Currency: snapshot.Currency, SettlementStatus: model.ProfitSettlementStatusPending},
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ledgers).Error; err != nil {
			return err
		}

		fillRelaySaaSSettlement(relayInfo, snapshot)

		return db.Model(&model.Log{}).Where("request_id = ?", relayInfo.RequestId).Updates(map[string]any{
			"tenant_id": snapshot.TenantId, "reseller_id": snapshot.ResellerId,
			"price_snapshot_id": snapshot.Id,
			"platform_cost_snapshot": snapshot.PlatformCostSnapshot, "tenant_price_snapshot": snapshot.TenantPriceSnapshot,
			"reseller_price_snapshot": snapshot.ResellerPriceSnapshot, "retail_price_snapshot": snapshot.RetailPriceSnapshot,
			"platform_profit": snapshot.PlatformProfit, "tenant_profit": snapshot.TenantProfit, "reseller_profit": snapshot.ResellerProfit,
		}).Error
	})
}

func ApplySaaSLogFields(params *model.RecordConsumeLogParams, relayInfo *relaycommon.RelayInfo) {
	if params == nil || relayInfo == nil || relayInfo.SaaSPriceQuote == nil {
		return
	}
	params.TenantId = relayInfo.SaaSPriceQuote.TenantId
	params.ResellerId = relayInfo.SaaSPriceQuote.ResellerId
	params.PriceSnapshotId = relayInfo.SaaSPriceSnapshotId
	params.PlatformCostSnapshot = relayInfo.SaaSPlatformCostSnapshot
	params.TenantPriceSnapshot = relayInfo.SaaSTenantPriceSnapshot
	params.ResellerPriceSnapshot = relayInfo.SaaSResellerPriceSnapshot
	params.RetailPriceSnapshot = relayInfo.SaaSRetailPriceSnapshot
	params.PlatformProfit = relayInfo.SaaSPlatformProfit
	params.TenantProfit = relayInfo.SaaSTenantProfit
	params.ResellerProfit = relayInfo.SaaSResellerProfit
}

func quotaByPrice(actualQuota, priceQuota int64) int64 {
	return actualQuota * priceQuota / saasPriceBaseQuota
}

func priceRuleIdsToString(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func fillRelaySaaSSettlement(relayInfo *relaycommon.RelayInfo, s model.PriceSnapshot) {
	relayInfo.SaaSPriceSnapshotId = s.Id
	relayInfo.SaaSPlatformCostSnapshot = s.PlatformCostSnapshot
	relayInfo.SaaSTenantPriceSnapshot = s.TenantPriceSnapshot
	relayInfo.SaaSResellerPriceSnapshot = s.ResellerPriceSnapshot
	relayInfo.SaaSRetailPriceSnapshot = s.RetailPriceSnapshot
	relayInfo.SaaSPlatformProfit = s.PlatformProfit
	relayInfo.SaaSTenantProfit = s.TenantProfit
	relayInfo.SaaSResellerProfit = s.ResellerProfit
}

// suppress unused import warning
var _ = common.Marshal
