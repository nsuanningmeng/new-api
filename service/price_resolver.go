package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const priceRuleStatusEnabled = 1

func ResolveSaaSPriceQuote(tenantId, resellerId, userId int, modelName, group string) (*relaycommon.SaaSPriceQuote, error) {
	modelName = normalizePriceMatchValue(modelName)
	group = normalizePriceMatchValue(group)

	targets := []struct {
		ownerType string
		ownerId   int
		enabled   bool
	}{
		{model.PriceRuleOwnerUser, userId, userId > 0},
		{model.PriceRuleOwnerReseller, resellerId, resellerId > 0},
		{model.PriceRuleOwnerTenant, tenantId, tenantId > 0},
		{model.PriceRuleOwnerPlatform, 0, true},
	}

	for _, t := range targets {
		if !t.enabled {
			continue
		}
		rule, ok, err := findBestPriceRule(t.ownerType, t.ownerId, modelName, group)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		quote := &relaycommon.SaaSPriceQuote{
			TenantId:                tenantId,
			ResellerId:              resellerId,
			PriceRuleIds:            []int{rule.Id},
			PlatformCostQuota:       rule.PlatformCostQuota,
			TenantSettlementQuota:   rule.TenantSettlementQuota,
			ResellerSettlementQuota: rule.ResellerSettlementQuota,
			RetailPriceQuota:        rule.RetailPriceQuota,
			Currency:                strings.TrimSpace(rule.Currency),
		}
		if quote.Currency == "" {
			quote.Currency = "USD"
		}
		if resellerId == 0 {
			quote.ResellerSettlementQuota = quote.RetailPriceQuota
		}
		if err := ValidatePriceMonotonic(quote); err != nil {
			return nil, fmt.Errorf("price rule %d is invalid: %w", rule.Id, err)
		}
		return quote, nil
	}
	return nil, nil
}

func ValidatePriceMonotonic(quote *relaycommon.SaaSPriceQuote) error {
	if quote == nil {
		return errors.New("price quote is nil")
	}
	if quote.PlatformCostQuota < 0 || quote.TenantSettlementQuota < 0 ||
		quote.ResellerSettlementQuota < 0 || quote.RetailPriceQuota < 0 {
		return errors.New("price quota must be >= 0")
	}
	if quote.TenantSettlementQuota < quote.PlatformCostQuota {
		return errors.New("tenant_settlement_quota must be >= platform_cost_quota")
	}
	if quote.ResellerSettlementQuota < quote.TenantSettlementQuota {
		return errors.New("reseller_settlement_quota must be >= tenant_settlement_quota")
	}
	if quote.RetailPriceQuota < quote.ResellerSettlementQuota {
		return errors.New("retail_price_quota must be >= reseller_settlement_quota")
	}
	return nil
}

func findBestPriceRule(ownerType string, ownerId int, modelName, group string) (*model.PriceRule, bool, error) {
	now := time.Now().Unix()
	var rules []model.PriceRule
	err := model.DB.
		Where("owner_type = ? AND owner_id = ? AND status = ? AND effective_at <= ? AND (expired_at = ? OR expired_at > ?)",
			ownerType, ownerId, priceRuleStatusEnabled, now, int64(0), now).
		Where("model_name IN ? AND "+model.PriceRuleGroupCol()+" IN ?", matchValues(modelName), matchValues(group)).
		Find(&rules).Error
	if err != nil {
		return nil, false, err
	}
	if len(rules) == 0 {
		return nil, false, nil
	}
	sort.SliceStable(rules, func(i, j int) bool {
		im := priceMatchScore(rules[i].ModelName, modelName)
		jm := priceMatchScore(rules[j].ModelName, modelName)
		if im != jm {
			return im > jm
		}
		ig := priceMatchScore(rules[i].Group, group)
		jg := priceMatchScore(rules[j].Group, group)
		if ig != jg {
			return ig > jg
		}
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].Id > rules[j].Id
	})
	return &rules[0], true, nil
}

func normalizePriceMatchValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "*"
	}
	return v
}

func matchValues(v string) []string {
	if v == "*" {
		return []string{"*"}
	}
	return []string{v, "*"}
}

func priceMatchScore(ruleVal, reqVal string) int {
	if strings.TrimSpace(ruleVal) == reqVal {
		return 2
	}
	if strings.TrimSpace(ruleVal) == "*" {
		return 1
	}
	return 0
}
