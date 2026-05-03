package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const profitSettlementBatchSize = 1000

type profitSettlementKey struct {
	BeneficiaryType string
	BeneficiaryId   int
	Currency        string
}

func RunProfitSettlement(ctx context.Context) error {
	var pending []model.ProfitLedger
	if err := model.DB.WithContext(ctx).
		Where("settlement_status = ?", model.ProfitSettlementStatusPending).
		Order("id asc").Limit(profitSettlementBatchSize).
		Find(&pending).Error; err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	ids := make([]int, 0, len(pending))
	for _, l := range pending {
		ids = append(ids, l.Id)
	}
	batchId := uuid.New().String()
	now := common.GetTimestamp()

	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ProfitLedger{}).
			Where("id IN ? AND settlement_status = ?", ids, model.ProfitSettlementStatusPending).
			Updates(map[string]any{
				"settlement_status":   model.ProfitSettlementStatusSettled,
				"settlement_batch_id": batchId,
				"settled_at":          now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		var claimed []model.ProfitLedger
		if err := tx.Where("settlement_batch_id = ?", batchId).Find(&claimed).Error; err != nil {
			return err
		}

		groups := map[profitSettlementKey]*struct {
			Amount int64
			Ids    []int
		}{}
		for _, l := range claimed {
			key := profitSettlementKey{l.BeneficiaryType, l.BeneficiaryId, normalizeWalletCurrency(l.Currency)}
			g := groups[key]
			if g == nil {
				g = &struct {
					Amount int64
					Ids    []int
				}{}
				groups[key] = g
			}
			g.Amount += l.Amount
			g.Ids = append(g.Ids, l.Id)
		}

		for key, g := range groups {
			if g.Amount <= 0 {
				continue
			}
			account, err := GetOrCreateWalletAccount(tx, key.BeneficiaryType, key.BeneficiaryId, key.Currency)
			if err != nil {
				return err
			}
			idempotencyKey := fmt.Sprintf("settlement:%s:%s:%d:%s", batchId, key.BeneficiaryType, key.BeneficiaryId, key.Currency)
			ledger, err := creditWallet(tx, account.Id, g.Amount, model.WalletLedgerTypeSettlement, "profit_settlement", batchId, "", idempotencyKey, "profit settlement")
			if err != nil {
				return err
			}
			if ledger == nil {
				continue
			}
			if err := tx.Model(&model.ProfitLedger{}).
				Where("id IN ? AND settlement_batch_id = ?", g.Ids, batchId).
				Update("wallet_ledger_id", ledger.Id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
