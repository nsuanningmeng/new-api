package model

import (
	"gorm.io/gorm"
)

// 工单状态。
const (
	TicketStatusOpen    = "open"
	TicketStatusReplied = "replied"
	TicketStatusClosed  = "closed"
)

// 工单优先级。
const (
	TicketPriorityLow    = "low"
	TicketPriorityNormal = "normal"
	TicketPriorityHigh   = "high"
)

// 派单角色（与 service.ActorRole* 字符串保持一致；
// 在 model 层定义独立常量以避免反向依赖 service 包）。
const (
	TicketAssigneePlatform = "platform_admin"
	TicketAssigneeTenant   = "tenant_admin"
	TicketAssigneeL1       = "reseller_l1"
	TicketAssigneeL2       = "reseller_l2"
)

// 派单层级（数值越小越靠上）；用于"升级 / 转派"时比较。
const (
	TicketAssigneeLevelPlatform = 0
	TicketAssigneeLevelTenant   = 1
	TicketAssigneeLevelL1       = 2
	TicketAssigneeLevelL2       = 3
)

// TicketReply.ActorRole 默认值。
const (
	TicketActorEndUser = "end_user"
	TicketActorSystem  = "system"
)

// Ticket 工单主表。多租户/多级代理感知。
//
// 红线：
//   - 不引入金额变动；本表 0 ledger
//   - 所有查询必须经 service.ApplyScope 过滤 tenant/reseller/owner
//   - 软删除字段保留以便审计
type Ticket struct {
	Id         int    `gorm:"primaryKey" json:"id"`
	UserId     int    `gorm:"not null;index" json:"user_id"`
	TenantId   int    `gorm:"type:int;default:0;index" json:"tenant_id"`
	ResellerId int    `gorm:"type:int;default:0;index" json:"reseller_id"`
	Status     string `gorm:"type:varchar(16);default:'open';index" json:"status"`
	Priority   string `gorm:"type:varchar(16);default:'normal'" json:"priority"`
	Subject    string `gorm:"type:varchar(255)" json:"subject"`
	Content    string `gorm:"type:text" json:"content"`
	Category   string `gorm:"type:varchar(32);default:''" json:"category"`

	// 多级派单（v1 一并落地，避免后续二次 ALTER）。
	AssigneeRole  string `gorm:"type:varchar(32);default:'platform_admin';index" json:"assignee_role"`
	AssigneeLevel int    `gorm:"type:int;default:0;index" json:"assignee_level"`
	EscalatedAt   *int64 `json:"escalated_at,omitempty"`
	EscalatedFrom string `gorm:"type:varchar(32);default:''" json:"escalated_from"`
	EscalateCount int    `gorm:"type:int;default:0" json:"escalate_count"`

	// 排序辅助字段。
	LastReplyAt *int64 `gorm:"index" json:"last_reply_at,omitempty"`

	// 计数缓存：列表展示无需 join，详情仍读真实附件表。
	AttachmentCount int `gorm:"type:int;default:0" json:"attachment_count"`

	CreatedAt int64 `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime" json:"updated_at"`

	ClosedAt *int64 `json:"closed_at,omitempty"`
	ClosedBy *int   `json:"closed_by,omitempty"`

	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Ticket) TableName() string {
	return "tickets"
}

// TicketReply 工单对话流。系统消息（升级/转派/关闭）通过 IsSystem=true 写入。
type TicketReply struct {
	Id        int    `gorm:"primaryKey" json:"id"`
	TicketId  int    `gorm:"not null;index" json:"ticket_id"`
	UserId    int    `gorm:"index" json:"user_id"`
	IsAdmin   bool   `json:"is_admin"`
	ActorRole string `gorm:"type:varchar(32);default:'end_user'" json:"actor_role"`
	IsSystem  bool   `gorm:"default:false" json:"is_system"`
	Content   string `gorm:"type:text" json:"content"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

func (TicketReply) TableName() string {
	return "ticket_replies"
}

// ---------- Reads ----------

// GetTicketById 平台视角读取（无 scope）。仅可在 platform_admin 路径或事务内部使用；
// 对外暴露的 controller 必须使用 GetTicketByIdScoped 或 GetTicketByIdForOwner。
func GetTicketById(id int) (*Ticket, error) {
	var t Ticket
	if err := DB.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTicketByIdScoped 在指定 tenant 范围内读取工单。
// tenantId == 0 表示不施加 tenant 约束（仅 platform_admin 调用）。
func GetTicketByIdScoped(id, tenantId int) (*Ticket, error) {
	var t Ticket
	q := DB
	if tenantId > 0 {
		q = q.Where("tenant_id = ?", tenantId)
	}
	if err := q.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTicketByIdForOwner 仅当请求用户为工单 owner 时返回，否则 ErrRecordNotFound。
func GetTicketByIdForOwner(id, userId int) (*Ticket, error) {
	var t Ticket
	if err := DB.Where("user_id = ?", userId).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTicketReplies 按时间升序加载工单回复（含系统消息）。
func GetTicketReplies(ticketId int) ([]TicketReply, error) {
	var replies []TicketReply
	if err := DB.Where("ticket_id = ?", ticketId).
		Order("created_at asc").
		Find(&replies).Error; err != nil {
		return nil, err
	}
	return replies, nil
}

// ---------- Writes ----------

// CreateTicket 直写工单（不带事务），仅作为简化 API；
// 大多数路径应走 service 层的事务封装。
func CreateTicket(ticket *Ticket) error {
	return DB.Create(ticket).Error
}

// CreateTicketTx 在外部事务内插入工单。
func CreateTicketTx(tx *gorm.DB, ticket *Ticket) error {
	if tx == nil {
		tx = DB
	}
	return tx.Create(ticket).Error
}

// CreateTicketReply 直写回复。
func CreateTicketReply(reply *TicketReply) error {
	return DB.Create(reply).Error
}

// CreateTicketReplyTx 在外部事务内插入回复。
func CreateTicketReplyTx(tx *gorm.DB, reply *TicketReply) error {
	if tx == nil {
		tx = DB
	}
	return tx.Create(reply).Error
}

// UpdateTicketAssignee 原子更新派单字段；
// incEscalate=true 时 escalate_count += 1；调用方需保证 escalatedAtMs 单位为毫秒（与 *int64 一致）。
func UpdateTicketAssignee(tx *gorm.DB, ticketId int, role string, level int, fromRole string, escalatedAtMs int64, incEscalate bool) error {
	if tx == nil {
		tx = DB
	}
	updates := map[string]interface{}{
		"assignee_role":  role,
		"assignee_level": level,
		"escalated_from": fromRole,
		"escalated_at":   escalatedAtMs,
	}
	if incEscalate {
		updates["escalate_count"] = gorm.Expr("escalate_count + 1")
	}
	return tx.Model(&Ticket{}).Where("id = ?", ticketId).Updates(updates).Error
}

// SetTicketStatus 仅更新状态字段；不会触动派单/关闭审计字段。
func SetTicketStatus(tx *gorm.DB, ticketId int, status string) error {
	if tx == nil {
		tx = DB
	}
	return tx.Model(&Ticket{}).Where("id = ?", ticketId).Update("status", status).Error
}

// CloseTicketWithUser 关闭工单并写入审计字段；幂等：再次关闭返回 nil（行被覆盖一次）。
func CloseTicketWithUser(tx *gorm.DB, ticketId int, closedAtMs int64, closedBy int) error {
	if tx == nil {
		tx = DB
	}
	return tx.Model(&Ticket{}).Where("id = ?", ticketId).Updates(map[string]interface{}{
		"status":    TicketStatusClosed,
		"closed_at": closedAtMs,
		"closed_by": closedBy,
	}).Error
}

// SetLastReplyAt 更新最后回复时间，用于列表 ORDER BY。
func SetLastReplyAt(tx *gorm.DB, ticketId int, tsMs int64) error {
	if tx == nil {
		tx = DB
	}
	return tx.Model(&Ticket{}).Where("id = ?", ticketId).Update("last_reply_at", tsMs).Error
}

// IncTicketAttachmentCount 增量更新工单上的附件计数器，传负值表示软删后回退。
func IncTicketAttachmentCount(tx *gorm.DB, ticketId, delta int) error {
	if tx == nil {
		tx = DB
	}
	return tx.Model(&Ticket{}).Where("id = ?", ticketId).
		Update("attachment_count", gorm.Expr("attachment_count + ?", delta)).Error
}
