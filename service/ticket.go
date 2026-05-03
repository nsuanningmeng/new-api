package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var (
	ErrTicketClosed          = errors.New("ticket is closed")
	ErrTicketNotFound        = errors.New("ticket not found")
	ErrCannotEscalateFurther = errors.New("cannot escalate further")
	ErrEscalateTooFrequent   = errors.New("ticket escalate too frequent")
	ErrEscalateLimitReached  = errors.New("ticket escalate limit reached")
	ErrAssignNotAllowed      = errors.New("ticket assign not allowed")
	ErrInvalidTargetRole     = errors.New("invalid ticket target role")
	ErrAssigneeUnavailable   = errors.New("ticket assignee unavailable")
	ErrTicketAccessDenied    = errors.New("ticket access denied")
)

type CreateTicketInput struct {
	UserId           int
	TenantId         int
	ResellerId       int
	Subject          string
	Content          string
	Priority         string
	Category         string
	DirectToPlatform bool
}

func CreateTicket(scope DataScope, input CreateTicketInput) (*model.Ticket, error) {
	userId := input.UserId
	if userId <= 0 {
		userId = scope.ActorUserId
	}
	tenantId := input.TenantId
	if tenantId == 0 {
		tenantId = scope.TenantId
	}
	resellerId := input.ResellerId
	if resellerId == 0 {
		resellerId = scope.ResellerId
	}
	priority := input.Priority
	if priority == "" {
		priority = model.TicketPriorityNormal
	}

	assigneeRole, assigneeLevel, fromRole, err := resolveInitialAssignee(scope, tenantId, resellerId, input.DirectToPlatform)
	if err != nil {
		return nil, err
	}

	ticket := &model.Ticket{
		UserId:        userId,
		TenantId:      tenantId,
		ResellerId:    resellerId,
		Status:        model.TicketStatusOpen,
		Priority:      priority,
		Subject:       input.Subject,
		Content:       input.Content,
		Category:      input.Category,
		AssigneeRole:  assigneeRole,
		AssigneeLevel: assigneeLevel,
		EscalatedFrom: fromRole,
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return model.CreateTicketTx(tx, ticket)
	}); err != nil {
		return nil, err
	}

	NotifyTicketEvent(ticket, EventTicketCreated, scope.ActorUserId)
	return ticket, nil
}

func ReplyTicket(ticketId int, scope DataScope, content string) (*model.TicketReply, error) {
	ticket, err := getTicketForActor(ticketId, scope)
	if err != nil {
		return nil, err
	}
	if ticket.Status == model.TicketStatusClosed {
		return nil, ErrTicketClosed
	}

	now := time.Now().UnixMilli()
	status := model.TicketStatusReplied
	if scope.ActorUserId == ticket.UserId {
		status = model.TicketStatusOpen
	}

	reply := &model.TicketReply{
		TicketId:  ticket.Id,
		UserId:    scope.ActorUserId,
		IsAdmin:   scope.ActorRole != ActorRoleEndUser,
		ActorRole: scope.ActorRole,
		IsSystem:  false,
		Content:   content,
	}

	// W03 修复：用 CAS 防止 close-after-read race —
	// 若另一事务在我们读到 status=open 后已把工单 close，这里的 RowsAffected=0 我们就 rollback。
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Ticket{}).
			Where("id = ? AND status <> ?", ticket.Id, model.TicketStatusClosed).
			Update("status", status)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrTicketClosed
		}
		if err := model.CreateTicketReplyTx(tx, reply); err != nil {
			return err
		}
		return model.SetLastReplyAt(tx, ticket.Id, now)
	}); err != nil {
		return nil, err
	}

	ticket.Status = status
	ticket.LastReplyAt = &now
	NotifyTicketEvent(ticket, EventTicketReplied, scope.ActorUserId)
	return reply, nil
}

func EscalateTicket(ticketId int, scope DataScope, byCustomer bool) error {
	ticket, err := getTicketForActor(ticketId, scope)
	if err != nil {
		return err
	}
	if ticket.Status == model.TicketStatusClosed {
		return ErrTicketClosed
	}
	if byCustomer {
		if scope.ActorUserId != ticket.UserId {
			return ErrTicketAccessDenied
		}
	} else if scope.ActorRole != ticket.AssigneeRole && scope.ActorRole != ActorRolePlatformAdmin {
		return ErrAssignNotAllowed
	}

	now := time.Now().UnixMilli()
	if ticket.EscalateCount >= 3 {
		return ErrEscalateLimitReached
	}
	if ticket.EscalatedAt != nil && now-*ticket.EscalatedAt < 60000 {
		return ErrEscalateTooFrequent
	}

	oldRole := ticket.AssigneeRole
	newRole, newLevel, err := nextEscalationTarget(ticket)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("Ticket escalated from %s to %s", oldRole, newRole)

	// W03 修复：用 CAS 同时校验 status<>'closed' AND escalate_count=expected，
	// 防止并发 close 或并发 escalate 突破限频。
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Ticket{}).
			Where("id = ? AND status <> ? AND escalate_count = ?", ticket.Id, model.TicketStatusClosed, ticket.EscalateCount).
			Updates(map[string]interface{}{
				"assignee_role":  newRole,
				"assignee_level": newLevel,
				"escalated_from": oldRole,
				"escalated_at":   now,
				"escalate_count": gorm.Expr("escalate_count + 1"),
				"status":         model.TicketStatusOpen,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrEscalateTooFrequent
		}
		if err := model.CreateTicketReplyTx(tx, &model.TicketReply{
			TicketId:  ticket.Id,
			UserId:    scope.ActorUserId,
			IsAdmin:   scope.ActorRole != ActorRoleEndUser,
			ActorRole: model.TicketActorSystem,
			IsSystem:  true,
			Content:   message,
		}); err != nil {
			return err
		}
		return model.SetLastReplyAt(tx, ticket.Id, now)
	}); err != nil {
		return err
	}

	ticket.AssigneeRole = newRole
	ticket.AssigneeLevel = newLevel
	ticket.EscalatedFrom = oldRole
	ticket.EscalatedAt = &now
	ticket.EscalateCount++
	ticket.Status = model.TicketStatusOpen
	ticket.LastReplyAt = &now
	NotifyTicketEvent(ticket, EventTicketEscalated, scope.ActorUserId)
	return nil
}

func AssignTicket(ticketId int, scope DataScope, targetRole string) error {
	if scope.ActorRole != ActorRolePlatformAdmin && scope.ActorRole != ActorRoleTenantAdmin {
		return ErrAssignNotAllowed
	}
	if !isValidTicketAssigneeRole(targetRole) {
		return ErrInvalidTargetRole
	}

	ticket, err := getTicketForActor(ticketId, scope)
	if err != nil {
		return err
	}
	if scope.ActorRole == ActorRoleTenantAdmin && ticket.TenantId != scope.TenantId {
		return ErrTicketAccessDenied
	}
	if len(ListAssigneeUsers(ticket, targetRole)) == 0 {
		return ErrAssigneeUnavailable
	}

	now := time.Now().UnixMilli()
	oldRole := ticket.AssigneeRole
	newLevel := ticketAssigneeLevel(targetRole)
	message := fmt.Sprintf("Reassigned from %s to %s by %s", oldRole, targetRole, scope.ActorRole)

	// W03 修复：CAS on status<>'closed'，防止并发 close 后再 assign。
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Ticket{}).
			Where("id = ? AND status <> ?", ticket.Id, model.TicketStatusClosed).
			Updates(map[string]interface{}{
				"assignee_role":  targetRole,
				"assignee_level": newLevel,
				"escalated_from": oldRole,
				"escalated_at":   now,
				"status":         model.TicketStatusOpen,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrTicketClosed
		}
		if err := model.CreateTicketReplyTx(tx, &model.TicketReply{
			TicketId:  ticket.Id,
			UserId:    scope.ActorUserId,
			IsAdmin:   true,
			ActorRole: model.TicketActorSystem,
			IsSystem:  true,
			Content:   message,
		}); err != nil {
			return err
		}
		return model.SetLastReplyAt(tx, ticket.Id, now)
	}); err != nil {
		return err
	}

	ticket.AssigneeRole = targetRole
	ticket.AssigneeLevel = newLevel
	ticket.EscalatedFrom = oldRole
	ticket.EscalatedAt = &now
	ticket.Status = model.TicketStatusOpen
	ticket.LastReplyAt = &now
	NotifyTicketEvent(ticket, EventTicketEscalated, scope.ActorUserId)
	return nil
}

func CloseTicket(ticketId int, scope DataScope) error {
	ticket, err := getTicketForActor(ticketId, scope)
	if err != nil {
		return err
	}
	if ticket.Status == model.TicketStatusClosed {
		return nil
	}

	now := time.Now().UnixMilli()
	message := fmt.Sprintf("Ticket closed by %s", scope.ActorRole)
	closedBy := scope.ActorUserId

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.CloseTicketWithUser(tx, ticket.Id, now, closedBy); err != nil {
			return err
		}
		if err := model.CreateTicketReplyTx(tx, &model.TicketReply{
			TicketId:  ticket.Id,
			UserId:    scope.ActorUserId,
			IsAdmin:   scope.ActorRole != ActorRoleEndUser,
			ActorRole: model.TicketActorSystem,
			IsSystem:  true,
			Content:   message,
		}); err != nil {
			return err
		}
		return model.SetLastReplyAt(tx, ticket.Id, now)
	}); err != nil {
		return err
	}

	ticket.Status = model.TicketStatusClosed
	ticket.ClosedAt = &now
	ticket.ClosedBy = &closedBy
	ticket.LastReplyAt = &now
	NotifyTicketEvent(ticket, EventTicketClosed, scope.ActorUserId)
	return nil
}

func ListMyTickets(scope DataScope, page, pageSize int, status string) ([]model.Ticket, int64, error) {
	tx := model.DB.Model(model.Ticket{}).Where("user_id = ?", scope.ActorUserId)
	tx = applyTicketStatusFilter(tx, status)
	return listTickets(tx, page, pageSize)
}

func ListAssigneeTickets(scope DataScope, page, pageSize int, status string) ([]model.Ticket, int64, error) {
	tx := model.DB.Model(model.Ticket{})
	tx = ApplyScope(tx, scope, "tickets")
	tx = tx.Where("assignee_role = ?", scope.ActorRole)
	tx = applyTicketStatusFilter(tx, status)
	return listTickets(tx, page, pageSize)
}

func ListDownstreamTickets(scope DataScope, page, pageSize int) ([]model.Ticket, int64, error) {
	tx := model.DB.Model(model.Ticket{})
	tx = ApplyScope(tx, scope, "tickets")
	return listTickets(tx, page, pageSize)
}

func GetTicketWithReplies(ticketId int, scope DataScope) (*model.Ticket, []model.TicketReply, error) {
	ticket, err := getTicketForActor(ticketId, scope)
	if err != nil {
		return nil, nil, err
	}
	replies, err := model.GetTicketReplies(ticket.Id)
	if err != nil {
		return nil, nil, err
	}
	return ticket, replies, nil
}

func getTicketForActor(ticketId int, scope DataScope) (*model.Ticket, error) {
	ticket, err := model.GetTicketById(ticketId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTicketNotFound
		}
		return nil, err
	}

	if ticket.UserId == scope.ActorUserId {
		return ticket, nil
	}
	if scope.ActorRole == ActorRolePlatformAdmin {
		return ticket, nil
	}
	if scope.TenantId <= 0 || ticket.TenantId != scope.TenantId {
		return nil, ErrTicketAccessDenied
	}

	switch scope.ActorRole {
	case ActorRoleTenantAdmin:
		return ticket, nil
	case ActorRoleResellerL1:
		if ticket.ResellerId == scope.ResellerId || intSliceContains(scope.DownlineIds, ticket.ResellerId) {
			return ticket, nil
		}
	case ActorRoleResellerL2:
		if ticket.ResellerId == scope.ResellerId {
			return ticket, nil
		}
	}
	return nil, ErrTicketAccessDenied
}

func resolveInitialAssignee(scope DataScope, tenantId, resellerId int, directToPlatform bool) (string, int, string, error) {
	role, level := computedInitialAssignee(scope, tenantId, resellerId)
	fromRole := ""
	if directToPlatform && role != model.TicketAssigneePlatform {
		fromRole = role
		role = model.TicketAssigneePlatform
		level = model.TicketAssigneeLevelPlatform
	}

	probe := &model.Ticket{TenantId: tenantId, ResellerId: resellerId}
	for {
		if len(ListAssigneeUsers(probe, role)) > 0 {
			return role, level, fromRole, nil
		}
		if role == model.TicketAssigneePlatform {
			return "", 0, "", ErrAssigneeUnavailable
		}
		level--
		if level < model.TicketAssigneeLevelPlatform {
			return "", 0, "", ErrAssigneeUnavailable
		}
		role = ticketAssigneeRoleForLevel(level)
	}
}

func computedInitialAssignee(scope DataScope, tenantId, resellerId int) (string, int) {
	switch scope.ActorRole {
	case ActorRoleEndUser:
		if reseller, ok := activeReseller(tenantId, resellerId); ok {
			if reseller.Level == 2 {
				return model.TicketAssigneeL2, model.TicketAssigneeLevelL2
			}
			if reseller.Level == 1 {
				return model.TicketAssigneeL1, model.TicketAssigneeLevelL1
			}
		}
		if tenantId > 0 && tenantHasAdminUser(tenantId) {
			return model.TicketAssigneeTenant, model.TicketAssigneeLevelTenant
		}
	case ActorRoleResellerL2:
		if _, ok := activeParentReseller(tenantId, resellerId); ok {
			return model.TicketAssigneeL1, model.TicketAssigneeLevelL1
		}
		if tenantId > 0 && tenantHasAdminUser(tenantId) {
			return model.TicketAssigneeTenant, model.TicketAssigneeLevelTenant
		}
	case ActorRoleResellerL1:
		if tenantId > 0 && tenantHasAdminUser(tenantId) {
			return model.TicketAssigneeTenant, model.TicketAssigneeLevelTenant
		}
	case ActorRoleTenantAdmin, ActorRolePlatformAdmin:
		return model.TicketAssigneePlatform, model.TicketAssigneeLevelPlatform
	}
	return model.TicketAssigneePlatform, model.TicketAssigneeLevelPlatform
}

func nextEscalationTarget(ticket *model.Ticket) (string, int, error) {
	seenAvailableLevel := false
	for level := ticket.AssigneeLevel - 1; level >= model.TicketAssigneeLevelPlatform; level-- {
		role := ticketAssigneeRoleForLevel(level)
		if role == "" || !ticketAssigneeLevelPossible(ticket, role) {
			continue
		}
		seenAvailableLevel = true
		if len(ListAssigneeUsers(ticket, role)) == 0 {
			continue
		}
		return role, level, nil
	}
	if seenAvailableLevel {
		return "", 0, ErrAssigneeUnavailable
	}
	return "", 0, ErrCannotEscalateFurther
}

func ticketAssigneeLevelPossible(ticket *model.Ticket, role string) bool {
	switch role {
	case model.TicketAssigneePlatform:
		return true
	case model.TicketAssigneeTenant:
		return ticket.TenantId > 0
	case model.TicketAssigneeL1, model.TicketAssigneeL2:
		return ticketResellerIdForRole(ticket, role) > 0
	default:
		return false
	}
}

func activeReseller(tenantId, resellerId int) (*model.Reseller, bool) {
	if tenantId <= 0 || resellerId <= 0 {
		return nil, false
	}
	var reseller model.Reseller
	err := model.DB.Where("tenant_id = ? AND id = ? AND status = ?", tenantId, resellerId, model.ResellerStatusEnabled).First(&reseller).Error
	return &reseller, err == nil
}

func activeParentReseller(tenantId, resellerId int) (*model.Reseller, bool) {
	reseller, ok := activeReseller(tenantId, resellerId)
	if !ok || reseller.ParentResellerId == nil {
		return nil, false
	}
	var parent model.Reseller
	err := model.DB.Where("tenant_id = ? AND id = ? AND level = ? AND status = ?", tenantId, *reseller.ParentResellerId, 1, model.ResellerStatusEnabled).First(&parent).Error
	return &parent, err == nil
}

func tenantHasAdminUser(tenantId int) bool {
	if tenantId <= 0 {
		return false
	}

	var tenant model.Tenant
	if err := model.DB.Select("owner_user_id", "status").First(&tenant, tenantId).Error; err == nil && tenant.Status == model.TenantStatusEnabled && tenant.OwnerUserId > 0 {
		var count int64
		model.DB.Model(&model.User{}).Where("id = ? AND status = ?", tenant.OwnerUserId, common.UserStatusEnabled).Count(&count)
		if count > 0 {
			return true
		}
	}

	var count int64
	model.DB.Model(&model.User{}).
		Where("tenant_id = ? AND role >= ? AND status = ?", tenantId, common.RoleAdminUser, common.UserStatusEnabled).
		Count(&count)
	return count > 0
}

func applyTicketStatusFilter(tx *gorm.DB, status string) *gorm.DB {
	if status == "" {
		return tx
	}
	return tx.Where("status = ?", status)
}

func listTickets(tx *gorm.DB, page, pageSize int) ([]model.Ticket, int64, error) {
	page, pageSize = normalizeTicketPagination(page, pageSize)

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tickets []model.Ticket
	err := tx.Order("COALESCE(last_reply_at, updated_at) DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&tickets).Error
	return tickets, total, err
}

func normalizeTicketPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func isValidTicketAssigneeRole(role string) bool {
	switch role {
	case model.TicketAssigneePlatform, model.TicketAssigneeTenant, model.TicketAssigneeL1, model.TicketAssigneeL2:
		return true
	default:
		return false
	}
}

func ticketAssigneeLevel(role string) int {
	switch role {
	case model.TicketAssigneeL2:
		return model.TicketAssigneeLevelL2
	case model.TicketAssigneeL1:
		return model.TicketAssigneeLevelL1
	case model.TicketAssigneeTenant:
		return model.TicketAssigneeLevelTenant
	default:
		return model.TicketAssigneeLevelPlatform
	}
}

func ticketAssigneeRoleForLevel(level int) string {
	switch level {
	case model.TicketAssigneeLevelL2:
		return model.TicketAssigneeL2
	case model.TicketAssigneeLevelL1:
		return model.TicketAssigneeL1
	case model.TicketAssigneeLevelTenant:
		return model.TicketAssigneeTenant
	case model.TicketAssigneeLevelPlatform:
		return model.TicketAssigneePlatform
	default:
		return ""
	}
}

func intSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
