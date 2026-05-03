package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createWithdrawRequest struct {
	Amount             int64  `json:"amount"`
	Currency           string `json:"currency"`
	DestinationType    string `json:"destination_type"`
	DestinationAccount string `json:"destination_account"`
}

type reviewWithdrawRequest struct {
	Action       string `json:"action"`
	RejectReason string `json:"reject_reason"`
	TradeNo      string `json:"trade_no"`
}

func GetTenantWallet(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	currency := c.DefaultQuery("currency", "USD")
	account, err := service.GetOrCreateWalletAccount(model.DB, model.WalletAccountTypeTenant, scope.TenantId, currency)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, account)
}

func GetTenantWalletLedger(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	currency := c.DefaultQuery("currency", "USD")
	account, err := service.GetOrCreateWalletAccount(model.DB, model.WalletAccountTypeTenant, scope.TenantId, currency)
	if err != nil {
		common.ApiError(c, err)
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
	var ledgers []model.WalletLedger
	q := model.DB.Model(&model.WalletLedger{}).Where("account_id = ?", account.Id)
	if lt := strings.TrimSpace(c.Query("ledger_type")); lt != "" {
		q = q.Where("ledger_type = ?", lt)
	}
	q.Count(&total)
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&ledgers).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"data": ledgers, "total": total})
}

func CreateTenantWithdraw(c *gin.Context) {
	scope, ok := getTenantAdminScope(c)
	if !ok {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	var req createWithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid params")
		return
	}
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.Amount <= 0 {
		common.ApiErrorMsg(c, "amount must be > 0")
		return
	}
	req.DestinationType = strings.TrimSpace(req.DestinationType)
	req.DestinationAccount = strings.TrimSpace(req.DestinationAccount)
	if req.DestinationType == "" || req.DestinationAccount == "" {
		common.ApiErrorMsg(c, "destination is required")
		return
	}

	var order model.WithdrawOrder
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var tenant model.Tenant
		if err := tx.Select("id", "min_withdraw_amount").First(&tenant, scope.TenantId).Error; err != nil {
			return err
		}
		if tenant.MinWithdrawAmount > 0 && req.Amount < tenant.MinWithdrawAmount {
			return fmt.Errorf("amount must be >= %d", tenant.MinWithdrawAmount)
		}
		account, err := service.GetOrCreateWalletAccount(tx, model.WalletAccountTypeTenant, scope.TenantId, req.Currency)
		if err != nil {
			return err
		}
		order = model.WithdrawOrder{
			AccountId: account.Id, AccountType: account.AccountType, OwnerId: account.OwnerId,
			TenantId: account.TenantId, ApplicantUserId: c.GetInt("id"),
			Amount: req.Amount, Currency: req.Currency, Status: model.WithdrawOrderStatusPending,
			DestinationType: req.DestinationType, DestinationAccount: req.DestinationAccount,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		return service.FreezeWallet(tx, account.Id, req.Amount, fmt.Sprintf("withdraw:%d:freeze", order.Id))
	})
	if err != nil {
		if errors.Is(err, service.ErrInsufficientBalance) {
			common.ApiErrorMsg(c, "insufficient balance")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, order)
}

func GetPlatformWithdrawOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := model.DB.Model(&model.WithdrawOrder{})
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		q = q.Where("status = ?", s)
	}
	if tid, _ := strconv.Atoi(c.Query("tenant_id")); tid > 0 {
		q = q.Where("tenant_id = ?", tid)
	}
	var total int64
	q.Count(&total)
	var orders []model.WithdrawOrder
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&orders).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"data": orders, "total": total})
}

func ReviewPlatformWithdraw(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	var req reviewWithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid params")
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	switch req.Action {
	case "reviewing", "approve", "reject":
	default:
		common.ApiErrorMsg(c, "invalid action: must be reviewing/approve/reject")
		return
	}

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var order model.WithdrawOrder
		if err := tx.First(&order, id).Error; err != nil {
			return err
		}
		reviewerId := c.GetInt("id")
		now := common.GetTimestamp()

		if req.Action == "reviewing" {
			if order.Status != model.WithdrawOrderStatusPending {
				return errors.New("order must be pending")
			}
			r := tx.Model(&model.WithdrawOrder{}).
				Where("id = ? AND status = ? AND version = ?", order.Id, model.WithdrawOrderStatusPending, order.Version).
				Updates(map[string]any{"status": model.WithdrawOrderStatusReviewing, "reviewer_user_id": reviewerId, "review_at": now, "version": gorm.Expr("version + 1")})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected == 0 {
				return service.ErrWalletConcurrentUpdate
			}
			return nil
		}

		validStatuses := []string{model.WithdrawOrderStatusPending, model.WithdrawOrderStatusReviewing}
		if order.Status != model.WithdrawOrderStatusPending && order.Status != model.WithdrawOrderStatusReviewing {
			return errors.New("order status invalid")
		}

		if req.Action == "reject" {
			r := tx.Model(&model.WithdrawOrder{}).
				Where("id = ? AND status IN ? AND version = ?", order.Id, validStatuses, order.Version).
				Updates(map[string]any{"status": model.WithdrawOrderStatusRejected, "reviewer_user_id": reviewerId, "reject_reason": strings.TrimSpace(req.RejectReason), "review_at": now, "version": gorm.Expr("version + 1")})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected == 0 {
				return service.ErrWalletConcurrentUpdate
			}
			return service.UnfreezeWallet(tx, order.AccountId, order.Amount, fmt.Sprintf("withdraw:%d:unfreeze", order.Id))
		}

		// approve
		r := tx.Model(&model.WithdrawOrder{}).
			Where("id = ? AND status IN ? AND version = ?", order.Id, validStatuses, order.Version).
			Updates(map[string]any{"status": model.WithdrawOrderStatusPaid, "reviewer_user_id": reviewerId, "trade_no": strings.TrimSpace(req.TradeNo), "review_at": now, "paid_at": now, "version": gorm.Expr("version + 1")})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return service.ErrWalletConcurrentUpdate
		}
		return service.PayFrozenWallet(tx, order.AccountId, order.Amount, "withdraw_order", strconv.Itoa(order.Id), "", fmt.Sprintf("withdraw:%d:paid", order.Id), "withdraw approved")
	})
	if err != nil {
		if errors.Is(err, service.ErrInsufficientBalance) {
			common.ApiErrorMsg(c, "insufficient balance")
			return
		}
		if errors.Is(err, service.ErrWalletConcurrentUpdate) {
			common.ApiErrorMsg(c, "concurrent update conflict, please retry")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
