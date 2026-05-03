package model

import (
	"gorm.io/gorm"
)

// TicketAttachment 工单附件存储（base64/二进制内联）。
//
// 安全约束（在 service 层强制，model 层只负责存取）：
//   - data 字段必须经过 service.ticket_attachment 的 MIME 嗅探 + 真实解码 + 重编码后写入；
//   - filename 必须经过 sanitize；
//   - mime 取服务端嗅探结果，不信任客户端。
//
// 三库兼容：data 用 mediumblob 标签，SQLite 实际存为 BLOB、PostgreSQL 存为 BYTEA、
// MySQL 存为 MEDIUMBLOB（最大 16MB，远超单文件 256KB 限制）。
type TicketAttachment struct {
	Id         int64  `gorm:"primaryKey" json:"id"`
	TicketId   int    `gorm:"not null;index" json:"ticket_id"`
	ReplyId    int    `gorm:"default:0;index" json:"reply_id"`
	TenantId   int    `gorm:"type:int;default:0;index" json:"tenant_id"`
	ResellerId int    `gorm:"type:int;default:0;index" json:"reseller_id"`
	UserId     int    `gorm:"not null;index" json:"user_id"`
	Filename   string `gorm:"type:varchar(255);default:''" json:"filename"`
	Mime       string `gorm:"type:varchar(64);not null" json:"mime"`
	Size       int    `gorm:"type:int;not null" json:"size"`
	Width      int    `gorm:"type:int;default:0" json:"width"`
	Height     int    `gorm:"type:int;default:0" json:"height"`
	Sha256     string `gorm:"type:char(64);not null;index" json:"sha256"`
	Data       []byte `gorm:"type:mediumblob" json:"-"`

	CreatedAt int64          `gorm:"autoCreateTime;index" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (TicketAttachment) TableName() string {
	return "ticket_attachments"
}

// 仅用于查询时只取元数据列，避免把 BLOB 拉进内存。
var attachmentMetaColumns = []string{
	"id", "ticket_id", "reply_id", "tenant_id", "reseller_id", "user_id",
	"filename", "mime", "size", "width", "height", "sha256", "created_at",
}

// GetAttachmentMeta 仅加载元数据（不含 data），用于详情/列表展示。
func GetAttachmentMeta(id int64) (*TicketAttachment, error) {
	var a TicketAttachment
	if err := DB.Select(attachmentMetaColumns).First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAttachmentForDownload 加载完整附件（含 data），仅供下载接口使用。
// 调用方必须事先做权限校验（owner / scope / not soft-deleted）。
func GetAttachmentForDownload(id int64) (*TicketAttachment, error) {
	var a TicketAttachment
	if err := DB.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAttachmentMetaByTicket 按工单加载所有未软删的附件元数据，按时间升序。
func ListAttachmentMetaByTicket(ticketId int) ([]TicketAttachment, error) {
	var list []TicketAttachment
	if err := DB.Select(attachmentMetaColumns).
		Where("ticket_id = ?", ticketId).
		Order("created_at asc").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindAttachmentBySha 在同一工单内按 sha256 查重；用于幂等/去重判断。
func FindAttachmentBySha(ticketId int, sha string) (*TicketAttachment, error) {
	var a TicketAttachment
	if err := DB.Select(attachmentMetaColumns).
		Where("ticket_id = ? AND sha256 = ?", ticketId, sha).
		First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// CountAttachmentsForTicket 统计某工单未软删的附件数量。
func CountAttachmentsForTicket(ticketId int) (int64, error) {
	var n int64
	err := DB.Model(&TicketAttachment{}).
		Where("ticket_id = ?", ticketId).
		Count(&n).Error
	return n, err
}

// CountAttachmentsForReply 统计某条回复关联的附件数量。
func CountAttachmentsForReply(replyId int) (int64, error) {
	var n int64
	err := DB.Model(&TicketAttachment{}).
		Where("reply_id = ?", replyId).
		Count(&n).Error
	return n, err
}

// SumAttachmentSizeForTicket 计算某工单未软删附件的总字节数；
// 三库通用 COALESCE 写法，空表时返回 0。
func SumAttachmentSizeForTicket(ticketId int) (int64, error) {
	var row struct {
		Total int64
	}
	err := DB.Model(&TicketAttachment{}).
		Where("ticket_id = ?", ticketId).
		Select("COALESCE(SUM(size), 0) AS total").
		Scan(&row).Error
	return row.Total, err
}

// CreateAttachmentTx 在事务内插入附件记录。
// 调用方应在同一事务内调用 IncTicketAttachmentCount 维护工单上的计数器。
func CreateAttachmentTx(tx *gorm.DB, a *TicketAttachment) error {
	if tx == nil {
		tx = DB
	}
	return tx.Create(a).Error
}

// SoftDeleteAttachment 软删除附件（保留 BLOB 用于审计）；
// 调用方需要在同一事务内 IncTicketAttachmentCount(-1)。
func SoftDeleteAttachment(tx *gorm.DB, id int64) error {
	if tx == nil {
		tx = DB
	}
	return tx.Delete(&TicketAttachment{}, id).Error
}
