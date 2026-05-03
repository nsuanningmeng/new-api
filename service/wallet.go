package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInsufficientBalance    = errors.New("insufficient wallet balance")
	ErrWalletConcurrentUpdate = errors.New("wallet concurrent update conflict")
)

func walletTx(tx *gorm.DB, fn func(*gorm.DB) error) error {
	if tx == nil {
		tx = model.DB
	}
	return tx.Transaction(fn)
}

func normalizeWalletCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "USD"
	}
	return currency
}

func ensureWalletAmount(amount int64) error {
	if amount <= 0 {
		return errors.New("wallet amount must be > 0")
	}
	return nil
}

func fillWalletAccountScope(tx *gorm.DB, account *model.WalletAccount) error {
	switch account.AccountType {
	case model.WalletAccountTypePlatform:
		account.OwnerId = 0
	case model.WalletAccountTypeTenant:
		account.TenantId = account.OwnerId
	case model.WalletAccountTypeReseller:
		var reseller model.Reseller
		if err := tx.Select("id", "tenant_id").First(&reseller, account.OwnerId).Error; err != nil {
			return err
		}
		account.TenantId = reseller.TenantId
		account.ResellerId = reseller.Id
	case model.WalletAccountTypeUser:
		var user model.User
		if err := tx.Select("id", "tenant_id", "reseller_id").First(&user, account.OwnerId).Error; err != nil {
			return err
		}
		account.UserId = user.Id
		account.TenantId = user.TenantId
		account.ResellerId = user.ResellerId
	default:
		return errors.New("invalid wallet account type")
	}
	return nil
}

func GetOrCreateWalletAccount(tx *gorm.DB, accountType string, ownerId int, currency string) (*model.WalletAccount, error) {
	if tx == nil {
		tx = model.DB
	}
	accountType = strings.TrimSpace(accountType)
	currency = normalizeWalletCurrency(currency)
	var account model.WalletAccount
	err := tx.Where("account_type = ? AND owner_id = ? AND currency = ?", accountType, ownerId, currency).First(&account).Error
	if err == nil {
		return &account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	account = model.WalletAccount{AccountType: accountType, OwnerId: ownerId, Currency: currency, Status: 1}
	if err := fillWalletAccountScope(tx, &account); err != nil {
		return nil, err
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		return &account, nil
	}
	if err := tx.Where("account_type = ? AND owner_id = ? AND currency = ?", accountType, ownerId, currency).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func createWalletLedger(db *gorm.DB, account model.WalletAccount, amount, balanceBefore, balanceAfter int64, ledgerType, direction, refType, refId, requestId, idempotencyKey, remark string) (*model.WalletLedger, bool, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, false, errors.New("idempotency_key required")
	}
	ledger := model.WalletLedger{
		AccountId: account.Id, AccountType: account.AccountType, OwnerId: account.OwnerId,
		TenantId: account.TenantId, ResellerId: account.ResellerId, UserId: account.UserId,
		Amount: amount, BalanceBefore: balanceBefore, BalanceAfter: balanceAfter,
		LedgerType: ledgerType, Direction: direction,
		RefType: strings.TrimSpace(refType), RefId: strings.TrimSpace(refId),
		RequestId: strings.TrimSpace(requestId), IdempotencyKey: strings.TrimSpace(idempotencyKey),
		Remark: strings.TrimSpace(remark),
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ledger)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		var existing model.WalletLedger
		if err := db.Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err != nil {
			return nil, false, err
		}
		return &existing, false, nil
	}
	return &ledger, true, nil
}

func creditWallet(db *gorm.DB, accountId int, amount int64, ledgerType, refType, refId, requestId, idempotencyKey, remark string) (*model.WalletLedger, error) {
	if err := ensureWalletAmount(amount); err != nil {
		return nil, err
	}
	var account model.WalletAccount
	if err := db.First(&account, accountId).Error; err != nil {
		return nil, err
	}
	ledger, created, err := createWalletLedger(db, account, amount, account.Balance, account.Balance+amount, ledgerType, model.WalletLedgerDirectionCredit, refType, refId, requestId, idempotencyKey, remark)
	if err != nil || !created {
		return ledger, err
	}
	result := db.Model(&model.WalletAccount{}).
		Where("id = ? AND version = ?", account.Id, account.Version).
		Updates(map[string]any{"balance": gorm.Expr("balance + ?", amount), "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrWalletConcurrentUpdate
	}
	return ledger, nil
}

func CreditWallet(tx *gorm.DB, accountId int, amount int64, ledgerType, refType, refId, requestId, idempotencyKey, remark string) error {
	return walletTx(tx, func(db *gorm.DB) error {
		_, err := creditWallet(db, accountId, amount, ledgerType, refType, refId, requestId, idempotencyKey, remark)
		return err
	})
}

func debitWallet(db *gorm.DB, accountId int, amount int64, ledgerType, refType, refId, requestId, idempotencyKey, remark string) (*model.WalletLedger, error) {
	if err := ensureWalletAmount(amount); err != nil {
		return nil, err
	}
	var account model.WalletAccount
	if err := db.First(&account, accountId).Error; err != nil {
		return nil, err
	}
	if account.Balance-account.FrozenBalance < amount {
		return nil, ErrInsufficientBalance
	}
	ledger, created, err := createWalletLedger(db, account, amount, account.Balance, account.Balance-amount, ledgerType, model.WalletLedgerDirectionDebit, refType, refId, requestId, idempotencyKey, remark)
	if err != nil || !created {
		return ledger, err
	}
	result := db.Model(&model.WalletAccount{}).
		Where("id = ? AND version = ? AND balance - frozen_balance >= ?", account.Id, account.Version, amount).
		Updates(map[string]any{"balance": gorm.Expr("balance - ?", amount), "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		var latest model.WalletAccount
		if err := db.First(&latest, account.Id).Error; err == nil && latest.Balance-latest.FrozenBalance < amount {
			return nil, ErrInsufficientBalance
		}
		return nil, ErrWalletConcurrentUpdate
	}
	return ledger, nil
}

func DebitWallet(tx *gorm.DB, accountId int, amount int64, ledgerType, refType, refId, requestId, idempotencyKey, remark string) error {
	return walletTx(tx, func(db *gorm.DB) error {
		_, err := debitWallet(db, accountId, amount, ledgerType, refType, refId, requestId, idempotencyKey, remark)
		return err
	})
}

func FreezeWallet(tx *gorm.DB, accountId int, amount int64, idempotencyKey string) error {
	return walletTx(tx, func(db *gorm.DB) error {
		if err := ensureWalletAmount(amount); err != nil {
			return err
		}
		var account model.WalletAccount
		if err := db.First(&account, accountId).Error; err != nil {
			return err
		}
		if account.Balance-account.FrozenBalance < amount {
			return ErrInsufficientBalance
		}
		_, created, err := createWalletLedger(db, account, amount, account.Balance, account.Balance, model.WalletLedgerTypeWithdrawFreeze, model.WalletLedgerDirectionDebit, "withdraw_order", "", "", idempotencyKey, "withdraw freeze")
		if err != nil || !created {
			return err
		}
		result := db.Model(&model.WalletAccount{}).
			Where("id = ? AND version = ? AND balance - frozen_balance >= ?", account.Id, account.Version, amount).
			Updates(map[string]any{"frozen_balance": gorm.Expr("frozen_balance + ?", amount), "version": gorm.Expr("version + 1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrWalletConcurrentUpdate
		}
		return nil
	})
}

func UnfreezeWallet(tx *gorm.DB, accountId int, amount int64, idempotencyKey string) error {
	return walletTx(tx, func(db *gorm.DB) error {
		if err := ensureWalletAmount(amount); err != nil {
			return err
		}
		var account model.WalletAccount
		if err := db.First(&account, accountId).Error; err != nil {
			return err
		}
		if account.FrozenBalance < amount {
			return ErrInsufficientBalance
		}
		_, created, err := createWalletLedger(db, account, amount, account.Balance, account.Balance, model.WalletLedgerTypeWithdrawUnfreeze, model.WalletLedgerDirectionCredit, "withdraw_order", "", "", idempotencyKey, "withdraw unfreeze")
		if err != nil || !created {
			return err
		}
		result := db.Model(&model.WalletAccount{}).
			Where("id = ? AND version = ? AND frozen_balance >= ?", account.Id, account.Version, amount).
			Updates(map[string]any{"frozen_balance": gorm.Expr("frozen_balance - ?", amount), "version": gorm.Expr("version + 1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrWalletConcurrentUpdate
		}
		return nil
	})
}

func PayFrozenWallet(tx *gorm.DB, accountId int, amount int64, refType, refId, requestId, idempotencyKey, remark string) error {
	return walletTx(tx, func(db *gorm.DB) error {
		if err := ensureWalletAmount(amount); err != nil {
			return err
		}
		var account model.WalletAccount
		if err := db.First(&account, accountId).Error; err != nil {
			return err
		}
		if account.Balance < amount || account.FrozenBalance < amount {
			return ErrInsufficientBalance
		}
		_, created, err := createWalletLedger(db, account, amount, account.Balance, account.Balance-amount, model.WalletLedgerTypeWithdrawPaid, model.WalletLedgerDirectionDebit, refType, refId, requestId, idempotencyKey, remark)
		if err != nil || !created {
			return err
		}
		result := db.Model(&model.WalletAccount{}).
			Where("id = ? AND version = ? AND balance >= ? AND frozen_balance >= ?", account.Id, account.Version, amount, amount).
			Updates(map[string]any{"balance": gorm.Expr("balance - ?", amount), "frozen_balance": gorm.Expr("frozen_balance - ?", amount), "version": gorm.Expr("version + 1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrWalletConcurrentUpdate
		}
		return nil
	})
}

type TopUpRechargeResult struct {
	UserId           int
	Quota            int64
	Money            float64
	PaymentMethod    string
	AlreadyCompleted bool
}

func CompleteTopUpRecharge(tradeNo, expectedProvider, actualPaymentMethod, stripeCustomerId, customerEmail string) (*TopUpRechargeResult, error) {
	if strings.TrimSpace(tradeNo) == "" {
		return nil, errors.New("trade_no is required")
	}
	result := &TopUpRechargeResult{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var topUp model.TopUp
		if err := tx.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
			return model.ErrTopUpNotFound
		}
		if expectedProvider != "" && topUp.PaymentProvider != expectedProvider {
			return model.ErrPaymentMethodMismatch
		}
		result.UserId = topUp.UserId
		result.Money = topUp.Money
		result.PaymentMethod = topUp.PaymentMethod
		if strings.TrimSpace(actualPaymentMethod) != "" {
			result.PaymentMethod = strings.TrimSpace(actualPaymentMethod)
		}
		if topUp.Status == common.TopUpStatusSuccess {
			result.AlreadyCompleted = true
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return model.ErrTopUpStatusInvalid
		}

		var quota int64
		switch topUp.PaymentProvider {
		case model.PaymentProviderStripe:
			quota = decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
		case model.PaymentProviderCreem:
			quota = topUp.Amount
		default:
			quota = decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
		}
		if quota <= 0 {
			return errors.New("invalid topup quota")
		}
		result.Quota = quota

		updates := map[string]any{"complete_time": common.GetTimestamp(), "status": common.TopUpStatusSuccess}
		if strings.TrimSpace(actualPaymentMethod) != "" {
			updates["payment_method"] = strings.TrimSpace(actualPaymentMethod)
		}
		upd := tx.Model(&model.TopUp{}).Where("id = ? AND status = ?", topUp.Id, common.TopUpStatusPending).Updates(updates)
		if upd.Error != nil {
			return upd.Error
		}
		if upd.RowsAffected == 0 {
			var current model.TopUp
			statusQuery := tx.Select("id", "status").Where("id = ?", topUp.Id)
			if !common.UsingSQLite {
				statusQuery = statusQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := statusQuery.First(&current).Error; err != nil {
				return err
			}
			if current.Status == common.TopUpStatusSuccess {
				result.AlreadyCompleted = true
				return nil
			}
			return model.ErrTopUpStatusInvalid
		}

		userUpdates := map[string]any{"quota": gorm.Expr("quota + ?", quota)}
		if strings.TrimSpace(stripeCustomerId) != "" {
			userUpdates["stripe_customer"] = strings.TrimSpace(stripeCustomerId)
		}
		if strings.TrimSpace(customerEmail) != "" {
			var user model.User
			if err := tx.Select("id", "email").First(&user, topUp.UserId).Error; err != nil {
				return err
			}
			if strings.TrimSpace(user.Email) == "" {
				userUpdates["email"] = strings.TrimSpace(customerEmail)
			}
		}
		if err := tx.Model(&model.User{}).Where("id = ?", topUp.UserId).Updates(userUpdates).Error; err != nil {
			return err
		}

		account, err := GetOrCreateWalletAccount(tx, model.WalletAccountTypeUser, topUp.UserId, "USD")
		if err != nil {
			return err
		}
		idempotencyKey := fmt.Sprintf("recharge:%s:%s", topUp.PaymentProvider, topUp.TradeNo)
		return CreditWallet(tx, account.Id, quota, model.WalletLedgerTypeRecharge, "topup", topUp.TradeNo, "", idempotencyKey, "topup recharge")
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
