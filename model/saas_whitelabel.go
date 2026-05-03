package model

import (
	"errors"

	"gorm.io/gorm"
)

const (
	TenantStatusEnabled  = 1
	TenantStatusDisabled = 2

	ResellerStatusEnabled  = 1
	ResellerStatusDisabled = 2

	WalletAccountTypePlatform = "platform"
	WalletAccountTypeTenant   = "tenant"
	WalletAccountTypeReseller = "reseller"
	WalletAccountTypeUser     = "user"

	WalletLedgerTypeConsume          = "consume"
	WalletLedgerTypeRecharge         = "recharge"
	WalletLedgerTypeWithdrawFreeze   = "withdraw_freeze"
	WalletLedgerTypeWithdrawUnfreeze = "withdraw_unfreeze"
	WalletLedgerTypeWithdrawPaid     = "withdraw_paid"
	WalletLedgerTypeSettlement       = "settlement"
	WalletLedgerTypeRefund           = "refund"
	WalletLedgerTypeAdjustment       = "adjustment"

	WalletLedgerDirectionDebit  = "debit"
	WalletLedgerDirectionCredit = "credit"

	PriceRuleOwnerPlatform = "platform"
	PriceRuleOwnerTenant   = "tenant"
	PriceRuleOwnerReseller = "reseller"
	PriceRuleOwnerUser     = "user"

	RechargeOrderStatusPending = "pending"
	RechargeOrderStatusPaid    = "paid"
	RechargeOrderStatusFailed  = "failed"
	RechargeOrderStatusClosed  = "closed"

	WithdrawOrderStatusPending   = "pending"
	WithdrawOrderStatusReviewing = "reviewing"
	WithdrawOrderStatusApproved  = "approved"
	WithdrawOrderStatusRejected  = "rejected"
	WithdrawOrderStatusPaid      = "paid"
	WithdrawOrderStatusFailed    = "failed"

	ProfitTypePlatform = "platform_profit"
	ProfitTypeTenant   = "tenant_profit"
	ProfitTypeReseller = "reseller_profit"

	ProfitSettlementStatusPending = "pending"
	ProfitSettlementStatusSettled = "settled"
	ProfitSettlementStatusVoid    = "void"
)

type Tenant struct {
	Id                 int            `json:"id"`
	Code               string         `json:"code" gorm:"type:varchar(64);not null;uniqueIndex"`
	Name               string         `json:"name" gorm:"type:varchar(128);not null;index"`
	OwnerUserId        int            `json:"owner_user_id" gorm:"index;default:0"`
	Status             int            `json:"status" gorm:"type:int;default:1;index"`
	BrandName          string         `json:"brand_name" gorm:"type:varchar(128);default:''"`
	LogoUrl            string         `json:"logo_url" gorm:"type:varchar(512);default:''"`
	FaviconUrl         string         `json:"favicon_url" gorm:"type:varchar(512);default:''"`
	PrimaryColor       string         `json:"primary_color" gorm:"type:varchar(32);default:''"`
	SettlementMode     string         `json:"settlement_mode" gorm:"type:varchar(32);default:'manual'"`
	SettlementCycle    string         `json:"settlement_cycle" gorm:"type:varchar(32);default:'monthly'"`
	SettlementCurrency string         `json:"settlement_currency" gorm:"type:varchar(8);default:'USD'"`
	MinWithdrawAmount  int64          `json:"min_withdraw_amount" gorm:"type:bigint;default:0"`
	MaxResellerLevel   int            `json:"max_reseller_level" gorm:"type:int;default:2"`
	BillingConfig      string         `json:"billing_config" gorm:"type:text"`
	Remark             string         `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedAt          int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

type TenantDomain struct {
	Id          int            `json:"id"`
	TenantId    int            `json:"tenant_id" gorm:"not null;index;uniqueIndex:idx_td_tenant_domain,priority:1"`
	Domain      string         `json:"domain" gorm:"type:varchar(255);not null;uniqueIndex;uniqueIndex:idx_td_tenant_domain,priority:2"`
	IsPrimary   bool           `json:"is_primary" gorm:"default:false"`
	Status      int            `json:"status" gorm:"type:int;default:1;index"`
	VerifyToken string         `json:"verify_token" gorm:"type:varchar(128);default:''"`
	VerifiedAt  int64          `json:"verified_at" gorm:"type:bigint;default:0"`
	CreatedAt   int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

type TenantSetting struct {
	Id        int    `json:"id"`
	TenantId  int    `json:"tenant_id" gorm:"not null;index;uniqueIndex:idx_ts_key,priority:1"`
	Key       string `json:"key" gorm:"type:varchar(128);not null;uniqueIndex:idx_ts_key,priority:2"`
	Value     string `json:"value" gorm:"type:text"`
	IsSecret  bool   `json:"is_secret" gorm:"default:false"`
	Version   int    `json:"version" gorm:"type:int;default:1"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// Reseller 经销商，最多2层（level 1 or 2）
type Reseller struct {
	Id               int            `json:"id"`
	TenantId         int            `json:"tenant_id" gorm:"not null;index;uniqueIndex:idx_reseller_tenant_user,priority:1"`
	UserId           int            `json:"user_id" gorm:"not null;index;uniqueIndex:idx_reseller_tenant_user,priority:2"`
	ParentResellerId *int           `json:"parent_reseller_id" gorm:"index"`
	Level            int            `json:"level" gorm:"type:int;not null;index"`
	Status           int            `json:"status" gorm:"type:int;default:1;index"`
	Name             string         `json:"name" gorm:"type:varchar(128);default:''"`
	ContactEmail     string         `json:"contact_email" gorm:"type:varchar(128);default:'';index"`
	SettlementMode   string         `json:"settlement_mode" gorm:"type:varchar(32);default:'manual'"`
	CommissionConfig string         `json:"commission_config" gorm:"type:text"`
	Remark           string         `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedAt        int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

type WalletAccount struct {
	Id            int    `json:"id"`
	AccountType   string `json:"account_type" gorm:"type:varchar(32);not null;index;uniqueIndex:idx_wallet_owner_currency,priority:1"`
	OwnerId       int    `json:"owner_id" gorm:"not null;default:0;index;uniqueIndex:idx_wallet_owner_currency,priority:2"`
	TenantId      int    `json:"tenant_id" gorm:"default:0;index"`
	ResellerId    int    `json:"reseller_id" gorm:"default:0;index"`
	UserId        int    `json:"user_id" gorm:"default:0;index"`
	Currency      string `json:"currency" gorm:"type:varchar(8);default:'USD';uniqueIndex:idx_wallet_owner_currency,priority:3"`
	Balance       int64  `json:"balance" gorm:"type:bigint;default:0"`
	FrozenBalance int64  `json:"frozen_balance" gorm:"type:bigint;default:0"`
	Version       int64  `json:"version" gorm:"type:bigint;default:0"`
	Status        int    `json:"status" gorm:"type:int;default:1;index"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// WalletLedger 不可变流水
type WalletLedger struct {
	Id             int    `json:"id"`
	AccountId      int    `json:"account_id" gorm:"not null;index"`
	AccountType    string `json:"account_type" gorm:"type:varchar(32);not null;index"`
	OwnerId        int    `json:"owner_id" gorm:"not null;default:0;index"`
	TenantId       int    `json:"tenant_id" gorm:"default:0;index"`
	ResellerId     int    `json:"reseller_id" gorm:"default:0;index"`
	UserId         int    `json:"user_id" gorm:"default:0;index"`
	Amount         int64  `json:"amount" gorm:"type:bigint;not null"`
	BalanceBefore  int64  `json:"balance_before" gorm:"type:bigint;not null"`
	BalanceAfter   int64  `json:"balance_after" gorm:"type:bigint;not null"`
	LedgerType     string `json:"ledger_type" gorm:"type:varchar(64);not null;index"`
	Direction      string `json:"direction" gorm:"type:varchar(16);not null"`
	RefType        string `json:"ref_type" gorm:"type:varchar(64);default:'';index:idx_wl_ref,priority:1"`
	RefId          string `json:"ref_id" gorm:"type:varchar(128);default:'';index:idx_wl_ref,priority:2"`
	RequestId      string `json:"request_id" gorm:"type:varchar(64);default:'';index"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	Remark         string `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func (WalletLedger) BeforeUpdate(*gorm.DB) error { return errors.New("wallet ledger is immutable") }
func (WalletLedger) BeforeDelete(*gorm.DB) error { return errors.New("wallet ledger is immutable") }

type PriceRule struct {
	Id                      int            `json:"id"`
	OwnerType               string         `json:"owner_type" gorm:"type:varchar(32);not null;index:idx_pr_lookup,priority:1"`
	OwnerId                 int            `json:"owner_id" gorm:"not null;default:0;index:idx_pr_lookup,priority:2"`
	TenantId                int            `json:"tenant_id" gorm:"default:0;index"`
	ResellerId              int            `json:"reseller_id" gorm:"default:0;index"`
	ModelName               string         `json:"model_name" gorm:"type:varchar(128);default:'*';index:idx_pr_lookup,priority:3"`
	Group                   string         `json:"group" gorm:"type:varchar(64);default:'*';index:idx_pr_lookup,priority:4"`
	BillingUnit             string         `json:"billing_unit" gorm:"type:varchar(32);default:'quota'"`
	PlatformCostQuota       int64          `json:"platform_cost_quota" gorm:"type:bigint;default:0"`
	TenantSettlementQuota   int64          `json:"tenant_settlement_quota" gorm:"type:bigint;default:0"`
	ResellerSettlementQuota int64          `json:"reseller_settlement_quota" gorm:"type:bigint;default:0"`
	RetailPriceQuota        int64          `json:"retail_price_quota" gorm:"type:bigint;default:0"`
	Currency                string         `json:"currency" gorm:"type:varchar(8);default:'USD'"`
	Priority                int            `json:"priority" gorm:"type:int;default:0;index"`
	Status                  int            `json:"status" gorm:"type:int;default:1;index"`
	EffectiveAt             int64          `json:"effective_at" gorm:"type:bigint;default:0;index"`
	ExpiredAt               int64          `json:"expired_at" gorm:"type:bigint;default:0;index"`
	Version                 int            `json:"version" gorm:"type:int;default:1"`
	Remark                  string         `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedAt               int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt               int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt               gorm.DeletedAt `json:"-" gorm:"index"`
}

// PriceSnapshot 不可变价格快照
type PriceSnapshot struct {
	Id                      int    `json:"id"`
	RequestId               string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	TenantId                int    `json:"tenant_id" gorm:"default:0;index"`
	ResellerId              int    `json:"reseller_id" gorm:"default:0;index"`
	UserId                  int    `json:"user_id" gorm:"not null;index"`
	TokenId                 int    `json:"token_id" gorm:"default:0;index"`
	LogId                   int    `json:"log_id" gorm:"default:0;index"`
	PriceRuleIds            string `json:"price_rule_ids" gorm:"type:varchar(255);default:''"`
	ModelName               string `json:"model_name" gorm:"type:varchar(128);default:'';index"`
	Group                   string `json:"group" gorm:"type:varchar(64);default:'';index"`
	ConsumedQuota           int64  `json:"consumed_quota" gorm:"type:bigint;default:0"`
	PlatformCostSnapshot    int64  `json:"platform_cost_snapshot" gorm:"type:bigint;default:0"`
	TenantPriceSnapshot     int64  `json:"tenant_price_snapshot" gorm:"type:bigint;default:0"`
	ResellerPriceSnapshot   int64  `json:"reseller_price_snapshot" gorm:"type:bigint;default:0"`
	RetailPriceSnapshot     int64  `json:"retail_price_snapshot" gorm:"type:bigint;default:0"`
	PlatformProfit          int64  `json:"platform_profit" gorm:"type:bigint;default:0"`
	TenantProfit            int64  `json:"tenant_profit" gorm:"type:bigint;default:0"`
	ResellerProfit          int64  `json:"reseller_profit" gorm:"type:bigint;default:0"`
	Currency                string `json:"currency" gorm:"type:varchar(8);default:'USD'"`
	CreatedAt               int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func (PriceSnapshot) BeforeUpdate(*gorm.DB) error { return errors.New("price snapshot is immutable") }
func (PriceSnapshot) BeforeDelete(*gorm.DB) error { return errors.New("price snapshot is immutable") }

type RechargeOrder struct {
	Id              int    `json:"id"`
	TenantId        int    `json:"tenant_id" gorm:"default:0;index"`
	ResellerId      int    `json:"reseller_id" gorm:"default:0;index"`
	UserId          int    `json:"user_id" gorm:"not null;index"`
	Amount          int64  `json:"amount" gorm:"type:bigint;not null"`
	QuotaAmount     int64  `json:"quota_amount" gorm:"type:bigint;not null"`
	Currency        string `json:"currency" gorm:"type:varchar(8);default:'USD'"`
	PaymentProvider string `json:"payment_provider" gorm:"type:varchar(50);not null;uniqueIndex:idx_ro_provider_trade,priority:1"`
	TradeNo         string `json:"trade_no" gorm:"type:varchar(255);not null;uniqueIndex:idx_ro_provider_trade,priority:2"`
	ProviderTradeNo string `json:"provider_trade_no" gorm:"type:varchar(255);default:'';index"`
	Status          string `json:"status" gorm:"type:varchar(32);default:'pending';index"`
	PaidAt          int64  `json:"paid_at" gorm:"type:bigint;default:0"`
	ExpiredAt       int64  `json:"expired_at" gorm:"type:bigint;default:0;index"`
	IdempotencyKey  string `json:"idempotency_key" gorm:"type:varchar(128);default:'';index"`
	Metadata        string `json:"metadata" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type WithdrawOrder struct {
	Id                 int    `json:"id"`
	AccountId          int    `json:"account_id" gorm:"not null;index"`
	AccountType        string `json:"account_type" gorm:"type:varchar(32);not null;index"`
	OwnerId            int    `json:"owner_id" gorm:"not null;default:0;index"`
	TenantId           int    `json:"tenant_id" gorm:"default:0;index"`
	ResellerId         int    `json:"reseller_id" gorm:"default:0;index"`
	ApplicantUserId    int    `json:"applicant_user_id" gorm:"not null;index"`
	ReviewerUserId     int    `json:"reviewer_user_id" gorm:"default:0;index"`
	Amount             int64  `json:"amount" gorm:"type:bigint;not null"`
	FeeAmount          int64  `json:"fee_amount" gorm:"type:bigint;default:0"`
	Currency           string `json:"currency" gorm:"type:varchar(8);default:'USD'"`
	Status             string `json:"status" gorm:"type:varchar(32);default:'pending';index"`
	DestinationType    string `json:"destination_type" gorm:"type:varchar(50);default:''"`
	DestinationAccount string `json:"destination_account" gorm:"type:varchar(255);default:''"`
	TradeNo            string `json:"trade_no" gorm:"type:varchar(255);default:'';index"`
	RejectReason       string `json:"reject_reason" gorm:"type:varchar(255);default:''"`
	Version            int64  `json:"version" gorm:"type:bigint;default:0"`
	ReviewAt           int64  `json:"review_at" gorm:"type:bigint;default:0"`
	PaidAt             int64  `json:"paid_at" gorm:"type:bigint;default:0"`
	CreatedAt          int64  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt          int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// ProfitLedger append-only 利润分配流水
type ProfitLedger struct {
	Id                int    `json:"id"`
	PriceSnapshotId   int    `json:"price_snapshot_id" gorm:"not null;index;uniqueIndex:idx_pl_snapshot_type,priority:1"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);not null;index"`
	TenantId          int    `json:"tenant_id" gorm:"default:0;index"`
	ResellerId        int    `json:"reseller_id" gorm:"default:0;index"`
	BeneficiaryType   string `json:"beneficiary_type" gorm:"type:varchar(32);not null;index;uniqueIndex:idx_pl_snapshot_type,priority:2"`
	BeneficiaryId     int    `json:"beneficiary_id" gorm:"not null;default:0;index;uniqueIndex:idx_pl_snapshot_type,priority:3"`
	ProfitType        string `json:"profit_type" gorm:"type:varchar(64);not null;index"`
	Amount            int64  `json:"amount" gorm:"type:bigint;not null"`
	Currency          string `json:"currency" gorm:"type:varchar(8);default:'USD'"`
	SettlementStatus  string `json:"settlement_status" gorm:"type:varchar(32);default:'pending';index"`
	SettlementBatchId string `json:"settlement_batch_id" gorm:"type:varchar(64);default:'';index"`
	WalletLedgerId    int    `json:"wallet_ledger_id" gorm:"default:0;index"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime;index"`
	SettledAt         int64  `json:"settled_at" gorm:"type:bigint;default:0"`
}

func (ProfitLedger) BeforeDelete(*gorm.DB) error { return errors.New("profit ledger is immutable") }
