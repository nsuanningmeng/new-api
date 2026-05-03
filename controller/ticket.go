package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ---------- User-side handlers (owner of the ticket) ----------

func ListMyTickets(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	query := dto.ListTicketsQuery{}
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}
	normalizeTicketQuery(&query)

	tickets, total, err := service.ListMyTickets(scope, query.Page, query.PageSize, query.Status)
	if err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": ticketListResponse(tickets, total, query)})
}

func CreateTicket(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	req := dto.CreateTicketRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	ticket, err := service.CreateTicket(scope, service.CreateTicketInput{
		UserId:           scope.ActorUserId,
		TenantId:         scope.TenantId,
		ResellerId:       scope.ResellerId,
		Subject:          req.Subject,
		Content:          req.Content,
		Priority:         req.Priority,
		Category:         req.Category,
		DirectToPlatform: req.DirectToPlatform,
	})
	if err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": toTicketDTO(ticket)})
}

func GetMyTicket(c *gin.Context) {
	getTicketDetail(c)
}

func ReplyMyTicket(c *gin.Context) {
	replyTicket(c)
}

func CloseMyTicket(c *gin.Context) {
	closeTicket(c)
}

func EscalateMyTicket(c *gin.Context) {
	escalateTicket(c, true)
}

// ---------- Admin/Assignee-side handlers ----------

func ListAssigneeTickets(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	query := dto.ListTicketsQuery{}
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}
	normalizeTicketQuery(&query)

	tickets, total, err := service.ListAssigneeTickets(scope, query.Page, query.PageSize, query.Status)
	if err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": ticketListResponse(tickets, total, query)})
}

func ListDownstreamTickets(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	query := dto.ListTicketsQuery{}
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}
	normalizeTicketQuery(&query)

	tickets, total, err := service.ListDownstreamTickets(scope, query.Page, query.PageSize)
	if err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": ticketListResponse(tickets, total, query)})
}

func GetTicketAdmin(c *gin.Context) {
	getTicketDetail(c)
}

func ReplyTicketAdmin(c *gin.Context) {
	replyTicket(c)
}

func CloseTicketAdmin(c *gin.Context) {
	closeTicket(c)
}

func EscalateTicketAdmin(c *gin.Context) {
	escalateTicket(c, false)
}

func AssignTicketAdmin(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}

	req := dto.AssignTicketRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	if err := service.AssignTicket(ticketId, scope, req.TargetRole); err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// ---------- Attachment handlers ----------

func UploadAttachment(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}

	if err := c.Request.ParseMultipartForm(2 * 1024 * 1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid multipart form"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "missing file"})
		return
	}
	defer file.Close()

	replyId, _ := strconv.Atoi(c.PostForm("reply_id"))
	attachment, err := service.UploadTicketAttachment(service.UploadAttachmentInput{
		TicketId: ticketId,
		ReplyId:  replyId,
		Scope:    scope,
		Filename: header.Filename,
		Header:   header,
		File:     file,
	})
	if err != nil {
		respondTicketError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": ticketAttachmentDTO(attachment)})
}

func GetAttachment(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}
	attachmentId, ok := parseInt64Param(c, "aid")
	if !ok {
		return
	}

	attachment, err := service.DownloadTicketAttachment(attachmentId, scope)
	if err != nil {
		respondTicketError(c, err)
		return
	}
	if attachment.TicketId != ticketId {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	// 强制 attachment 下载头 + nosniff + sandbox CSP，禁止浏览器在线渲染恶意内容
	c.Header("Content-Disposition", "attachment; filename=\""+safeAttachmentDownloadName(attachment.Filename)+"\"")
	c.Header("Content-Type", attachment.Mime)
	c.Header("Content-Length", strconv.Itoa(attachment.Size))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cache-Control", "private, no-cache")
	c.Data(http.StatusOK, attachment.Mime, attachment.Data)
}

func DeleteAttachment(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}
	attachmentId, ok := parseInt64Param(c, "aid")
	if !ok {
		return
	}

	// I01 修复：先校验 attachment 确实属于这个 ticketId（路由一致性 + 防越权）。
	if err := service.DeleteTicketAttachmentInTicket(ticketId, attachmentId, scope); err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// ---------- Internal helpers ----------

func getTicketDetail(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}

	ticket, replies, err := service.GetTicketWithReplies(ticketId, scope)
	if err != nil {
		respondTicketError(c, err)
		return
	}

	attachments, err := service.ListTicketAttachmentDTOs(ticket.Id)
	if err != nil {
		respondTicketError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": toTicketDetailDTO(ticket, replies, attachments)})
}

func replyTicket(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}

	req := dto.ReplyTicketRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	reply, err := service.ReplyTicket(ticketId, scope, req.Content)
	if err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": toTicketReplyDTO(reply)})
}

func closeTicket(c *gin.Context) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}

	if err := service.CloseTicket(ticketId, scope); err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func escalateTicket(c *gin.Context, byCustomer bool) {
	scope, err := service.BuildDataScope(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
		return
	}

	ticketId, ok := parseIntParam(c, "id")
	if !ok {
		return
	}

	if err := service.EscalateTicket(ticketId, scope, byCustomer); err != nil {
		respondTicketError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func ticketStatusFromError(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrTicketNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrTicketAccessDenied):
		return http.StatusForbidden, "access denied"
	case errors.Is(err, service.ErrTicketClosed):
		return http.StatusBadRequest, "ticket closed"
	case errors.Is(err, service.ErrCannotEscalateFurther):
		return http.StatusBadRequest, "cannot escalate further"
	case errors.Is(err, service.ErrEscalateTooFrequent):
		return http.StatusTooManyRequests, "too frequent"
	case errors.Is(err, service.ErrEscalateLimitReached):
		return http.StatusBadRequest, "limit reached"
	case errors.Is(err, service.ErrAssignNotAllowed):
		return http.StatusForbidden, "assign not allowed"
	case errors.Is(err, service.ErrInvalidTargetRole):
		return http.StatusBadRequest, "invalid role"
	case errors.Is(err, service.ErrAssigneeUnavailable):
		return http.StatusBadRequest, "assignee unavailable"
	case errors.Is(err, service.ErrAttachmentTooLarge):
		return http.StatusRequestEntityTooLarge, "too large"
	case errors.Is(err, service.ErrAttachmentMimeNotAllowed):
		return http.StatusUnsupportedMediaType, "mime not allowed"
	case errors.Is(err, service.ErrAttachmentDecodeFailed):
		return http.StatusBadRequest, "decode failed"
	case errors.Is(err, service.ErrAttachmentTooComplex):
		return http.StatusRequestEntityTooLarge, "image too complex"
	case errors.Is(err, service.ErrAttachmentLimitReached):
		return http.StatusBadRequest, "attachment limit"
	case errors.Is(err, service.ErrAttachmentTotalSizeExceeded):
		return http.StatusBadRequest, "total size exceeded"
	case errors.Is(err, service.ErrAttachmentClosed):
		return http.StatusBadRequest, "ticket closed"
	case errors.Is(err, service.ErrAttachmentForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrAttachmentDuplicate):
		return http.StatusConflict, "duplicate"
	case errors.Is(err, service.ErrAttachmentTooManyPixels):
		return http.StatusBadRequest, "too many pixels"
	default:
		common.SysError("ticket controller error: " + err.Error())
		return http.StatusInternalServerError, "internal error"
	}
}

func respondTicketError(c *gin.Context, err error) {
	status, message := ticketStatusFromError(err)
	c.JSON(status, gin.H{"success": false, "message": message})
}

func parseIntParam(c *gin.Context, name string) (int, bool) {
	value, err := strconv.Atoi(c.Param(name))
	if err != nil || value <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid parameter"})
		return 0, false
	}
	return value, true
}

func parseInt64Param(c *gin.Context, name string) (int64, bool) {
	value, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || value <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid parameter"})
		return 0, false
	}
	return value, true
}

func normalizeTicketQuery(query *dto.ListTicketsQuery) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
}

// ticketListResponse 用 codex correction 版本：直接接收 []model.Ticket，
// 避免 modelTicketLike interface 抽象（service 返回的就是 []model.Ticket）。
func ticketListResponse(tickets []model.Ticket, total int64, query dto.ListTicketsQuery) dto.TicketListResponse {
	items := make([]dto.TicketDTO, 0, len(tickets))
	for i := range tickets {
		items = append(items, toTicketDTO(&tickets[i]))
	}
	return dto.TicketListResponse{
		Items:    items,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}
}

func ticketAttachmentDTO(a *model.TicketAttachment) dto.TicketAttachmentMetaDTO {
	if a == nil {
		return dto.TicketAttachmentMetaDTO{}
	}
	return dto.TicketAttachmentMetaDTO{
		Id:        a.Id,
		TicketId:  a.TicketId,
		ReplyId:   a.ReplyId,
		Filename:  a.Filename,
		Mime:      a.Mime,
		Size:      a.Size,
		Width:     a.Width,
		Height:    a.Height,
		Sha256:    a.Sha256,
		CreatedAt: a.CreatedAt,
		Url:       fmt.Sprintf("/api/ticket/%d/attachment/%d", a.TicketId, a.Id),
	}
}

// toTicketDTO 等三个转换函数原本在 dto 包内，因为 dto ↔ model 会导致 import cycle，
// 移到这里（controller 已同时 import 两边）。
func toTicketDTO(t *model.Ticket) dto.TicketDTO {
	if t == nil {
		return dto.TicketDTO{}
	}
	return dto.TicketDTO{
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

func toTicketReplyDTO(r *model.TicketReply) dto.TicketReplyDTO {
	if r == nil {
		return dto.TicketReplyDTO{}
	}
	return dto.TicketReplyDTO{
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

func toTicketDetailDTO(t *model.Ticket, replies []model.TicketReply, attachments []dto.TicketAttachmentMetaDTO) dto.TicketDetailDTO {
	replyDTOs := make([]dto.TicketReplyDTO, 0, len(replies))
	for i := range replies {
		replyDTOs = append(replyDTOs, toTicketReplyDTO(&replies[i]))
	}
	return dto.TicketDetailDTO{
		TicketDTO:   toTicketDTO(t),
		Replies:     replyDTOs,
		Attachments: attachments,
	}
}

func safeAttachmentDownloadName(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, "\"", "_")
	filename = strings.ReplaceAll(filename, "\r", "_")
	filename = strings.ReplaceAll(filename, "\n", "_")
	if strings.TrimSpace(filename) == "" {
		return "attachment"
	}
	return filename
}
