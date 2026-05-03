package dto

import "github.com/QuantumNous/new-api/model"

type CreateTicketRequest struct {
	Subject          string `json:"subject" binding:"required,max=255"`
	Content          string `json:"content" binding:"required,max=20000"`
	Priority         string `json:"priority" binding:"omitempty,oneof=low normal high"`
	Category         string `json:"category" binding:"omitempty,max=32"`
	DirectToPlatform bool   `json:"direct_to_platform"`
}

type ReplyTicketRequest struct {
	Content string `json:"content" binding:"required,max=20000"`
}

type EscalateTicketRequest struct{}

type AssignTicketRequest struct {
	TargetRole string `json:"target_role" binding:"required,oneof=platform_admin tenant_admin reseller_l1 reseller_l2"`
}

type ListTicketsQuery struct {
	Page         int    `form:"page,default=1" binding:"min=1"`
	PageSize     int    `form:"page_size,default=20" binding:"min=1,max=100"`
	Status       string `form:"status" binding:"omitempty,oneof=open replied closed"`
	AssigneeRole string `form:"assignee_role"`
}

type TicketDTO struct {
	Id              int    `json:"id"`
	UserId          int    `json:"user_id"`
	TenantId        int    `json:"tenant_id"`
	ResellerId      int    `json:"reseller_id"`
	Status          string `json:"status"`
	Priority        string `json:"priority"`
	Subject         string `json:"subject"`
	Content         string `json:"content"`
	Category        string `json:"category"`
	AssigneeRole    string `json:"assignee_role"`
	AssigneeLevel   int    `json:"assignee_level"`
	EscalatedAt     *int64 `json:"escalated_at,omitempty"`
	EscalatedFrom   string `json:"escalated_from"`
	EscalateCount   int    `json:"escalate_count"`
	LastReplyAt     *int64 `json:"last_reply_at,omitempty"`
	AttachmentCount int    `json:"attachment_count"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	ClosedAt        *int64 `json:"closed_at,omitempty"`
	ClosedBy        *int   `json:"closed_by,omitempty"`
}

type TicketReplyDTO struct {
	Id        int    `json:"id"`
	TicketId  int    `json:"ticket_id"`
	UserId    int    `json:"user_id"`
	IsAdmin   bool   `json:"is_admin"`
	ActorRole string `json:"actor_role"`
	IsSystem  bool   `json:"is_system"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

type TicketAttachmentMetaDTO struct {
	Id        int64  `json:"id"`
	TicketId  int    `json:"ticket_id"`
	ReplyId   int    `json:"reply_id"`
	Filename  string `json:"filename"`
	Mime      string `json:"mime"`
	Size      int    `json:"size"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Sha256    string `json:"sha256"`
	CreatedAt int64  `json:"created_at"`
	Url       string `json:"url"`
}

type TicketDetailDTO struct {
	TicketDTO
	Replies     []TicketReplyDTO          `json:"replies"`
	Attachments []TicketAttachmentMetaDTO `json:"attachments"`
}

type TicketListResponse struct {
	Items    []TicketDTO `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

func ToTicketDTO(t *model.Ticket) TicketDTO {
	if t == nil {
		return TicketDTO{}
	}
	return TicketDTO{
		Id:              t.Id,
		UserId:          t.UserId,
		TenantId:        t.TenantId,
		ResellerId:      t.ResellerId,
		Status:          t.Status,
		Priority:        t.Priority,
		Subject:         t.Subject,
		Content:         t.Content,
		Category:        t.Category,
		AssigneeRole:    t.AssigneeRole,
		AssigneeLevel:   t.AssigneeLevel,
		EscalatedAt:     t.EscalatedAt,
		EscalatedFrom:   t.EscalatedFrom,
		EscalateCount:   t.EscalateCount,
		LastReplyAt:     t.LastReplyAt,
		AttachmentCount: t.AttachmentCount,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		ClosedAt:        t.ClosedAt,
		ClosedBy:        t.ClosedBy,
	}
}

func ToTicketReplyDTO(r *model.TicketReply) TicketReplyDTO {
	if r == nil {
		return TicketReplyDTO{}
	}
	return TicketReplyDTO{
		Id:        r.Id,
		TicketId:  r.TicketId,
		UserId:    r.UserId,
		IsAdmin:   r.IsAdmin,
		ActorRole: r.ActorRole,
		IsSystem:  r.IsSystem,
		Content:   r.Content,
		CreatedAt: r.CreatedAt,
	}
}

func ToTicketDetailDTO(t *model.Ticket, replies []model.TicketReply, attachments []TicketAttachmentMetaDTO) TicketDetailDTO {
	replyDTOs := make([]TicketReplyDTO, 0, len(replies))
	for i := range replies {
		replyDTOs = append(replyDTOs, ToTicketReplyDTO(&replies[i]))
	}
	return TicketDetailDTO{
		TicketDTO:   ToTicketDTO(t),
		Replies:     replyDTOs,
		Attachments: attachments,
	}
}
