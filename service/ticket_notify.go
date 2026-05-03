package service

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type TicketNotifyEvent string

const (
	EventTicketCreated   TicketNotifyEvent = "ticket_created"
	EventTicketReplied   TicketNotifyEvent = "ticket_replied"
	EventTicketEscalated TicketNotifyEvent = "ticket_escalated"
	EventTicketClosed    TicketNotifyEvent = "ticket_closed"
)

// NotifyTicketEvent fires the appropriate notification matrix asynchronously.
// Failures are logged but never propagate back to the business transaction.
func NotifyTicketEvent(t *model.Ticket, ev TicketNotifyEvent, actorUserId int) {
	if t == nil {
		return
	}
	ticket := *t
	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysError(fmt.Sprintf("panic in ticket notification: %v\n%s", r, debug.Stack()))
			}
		}()
		dispatchTicketNotification(&ticket, ev, actorUserId)
	}()
}

// ListAssigneeUsers exposes the per-role lookup so service/ticket.go can validate
// that a target role has at least one active user before assigning to it.
func ListAssigneeUsers(t *model.Ticket, role string) []model.User {
	return listAssigneeUsers(t, role)
}

func listPlatformAdmins() []model.User {
	var users []model.User
	if err := model.DB.Select("id", "email", "setting", "role", "status").
		Where("role >= ? AND status = ?", common.RoleRootUser, common.UserStatusEnabled).
		Find(&users).Error; err != nil {
		common.SysError(fmt.Sprintf("failed to list platform admins for ticket notification: %s", err.Error()))
		return nil
	}
	return users
}

func listTenantAdmins(tenantId int) []model.User {
	if tenantId <= 0 {
		return nil
	}

	var users []model.User
	seen := map[int]struct{}{}

	var tenant model.Tenant
	if err := model.DB.Select("id", "owner_user_id", "status").First(&tenant, tenantId).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError(fmt.Sprintf("failed to load tenant %d for ticket notification: %s", tenantId, err.Error()))
		}
		return nil
	}
	if tenant.Status != model.TenantStatusEnabled {
		return nil
	}

	if tenant.OwnerUserId > 0 {
		var owner model.User
		if err := model.DB.Select("id", "email", "setting", "role", "status").
			Where("id = ? AND status = ?", tenant.OwnerUserId, common.UserStatusEnabled).
			First(&owner).Error; err == nil {
			users = append(users, owner)
			seen[owner.Id] = struct{}{}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError(fmt.Sprintf("failed to load tenant owner %d for ticket notification: %s", tenant.OwnerUserId, err.Error()))
		}
	}

	var admins []model.User
	if err := model.DB.Select("id", "email", "setting", "role", "status").
		Where("tenant_id = ? AND role >= ? AND status = ?", tenantId, common.RoleAdminUser, common.UserStatusEnabled).
		Find(&admins).Error; err != nil {
		common.SysError(fmt.Sprintf("failed to list tenant admins for ticket notification: %s", err.Error()))
		return users
	}
	for _, admin := range admins {
		if _, ok := seen[admin.Id]; ok {
			continue
		}
		users = append(users, admin)
		seen[admin.Id] = struct{}{}
	}
	return users
}

func listResellerOwner(tenantId, resellerId int) []model.User {
	if tenantId <= 0 || resellerId <= 0 {
		return nil
	}

	var reseller model.Reseller
	if err := model.DB.Select("id", "tenant_id", "user_id", "status").
		Where("tenant_id = ? AND id = ? AND status = ?", tenantId, resellerId, model.ResellerStatusEnabled).
		First(&reseller).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError(fmt.Sprintf("failed to load reseller %d for ticket notification: %s", resellerId, err.Error()))
		}
		return nil
	}

	var user model.User
	if err := model.DB.Select("id", "email", "setting", "role", "status").
		Where("id = ? AND status = ?", reseller.UserId, common.UserStatusEnabled).
		First(&user).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError(fmt.Sprintf("failed to load reseller owner %d for ticket notification: %s", reseller.UserId, err.Error()))
		}
		return nil
	}
	return []model.User{user}
}

func listAssigneeUsers(t *model.Ticket, role string) []model.User {
	if t == nil {
		return nil
	}

	var users []model.User
	switch role {
	case model.TicketAssigneePlatform:
		users = listPlatformAdmins()
	case model.TicketAssigneeTenant:
		users = listTenantAdmins(t.TenantId)
	case model.TicketAssigneeL1, model.TicketAssigneeL2:
		resellerId := ticketResellerIdForRole(t, role)
		users = listResellerOwner(t.TenantId, resellerId)
	default:
		return nil
	}
	return capTicketUsers(users, "assignee")
}

func dispatchTicketNotification(t *model.Ticket, ev TicketNotifyEvent, actorUserId int) {
	switch ev {
	case EventTicketCreated:
		sendTicketNotifications(listAssigneeUsers(t, t.AssigneeRole), buildNotify(t, ev, "工单已创建"))

		ownerNotify := buildNotify(t, ev, "您已提交工单")
		ownerNotify.Title = fmt.Sprintf("[Ticket #%d] 您已提交 - %s", t.Id, truncateTicketText(t.Subject, 40))
		sendTicketNotifications(ticketOwnerUsers(t), ownerNotify)
	case EventTicketReplied:
		if actorUserId == t.UserId {
			sendTicketNotifications(listAssigneeUsers(t, t.AssigneeRole), buildNotify(t, ev, "工单有新回复"))
			return
		}
		sendTicketNotifications(ticketOwnerUsers(t), buildNotify(t, ev, "工单有新回复"))
	case EventTicketEscalated:
		recipients := listAssigneeUsers(t, t.AssigneeRole)
		recipients = append(recipients, listAssigneeUsers(t, t.EscalatedFrom)...)
		recipients = append(recipients, ticketOwnerUsers(t)...)
		sendTicketNotifications(recipients, buildNotify(t, ev, fmt.Sprintf("工单 #%d %s -> %s", t.Id, t.EscalatedFrom, t.AssigneeRole)))
	case EventTicketClosed:
		recipients := ticketOwnerUsers(t)
		recipients = append(recipients, listAssigneeUsers(t, t.AssigneeRole)...)
		sendTicketNotifications(recipients, buildNotify(t, ev, "工单已关闭"))
	}
}

func buildNotify(t *model.Ticket, ev TicketNotifyEvent, content string) dto.Notify {
	phrase := ticketEventPhrase(ev)
	title := fmt.Sprintf("[Ticket #%d] %s - %s", t.Id, phrase, truncateTicketText(t.Subject, 40))
	body := fmt.Sprintf(
		"工单号: %d\n主题: %s\n状态: %s\n优先级: %s\n当前处理: %s\n事件: %s\n内容: %s\n查看: /ticket/%d",
		t.Id,
		t.Subject,
		t.Status,
		t.Priority,
		t.AssigneeRole,
		phrase,
		truncateTicketText(content, 200),
		t.Id,
	)
	return dto.NewNotify(ticketNotifyType(ev), title, body, nil)
}

func ticketNotifyType(ev TicketNotifyEvent) string {
	switch ev {
	case EventTicketReplied:
		return dto.NotifyTypeTicketReply
	case EventTicketEscalated:
		return dto.NotifyTypeTicketEscalated
	case EventTicketClosed:
		return dto.NotifyTypeTicketClosed
	default:
		return dto.NotifyTypeTicketCreated
	}
}

func ticketEventPhrase(ev TicketNotifyEvent) string {
	switch ev {
	case EventTicketReplied:
		return "新回复"
	case EventTicketEscalated:
		return "已升级"
	case EventTicketClosed:
		return "已关闭"
	default:
		return "已创建"
	}
}

func sendTicketNotifications(users []model.User, notification dto.Notify) {
	recipients := capTicketUsers(dedupTicketUsers(users), "recipient")
	for _, user := range recipients {
		if user.Id <= 0 {
			continue
		}
		if err := NotifyUser(user.Id, user.Email, user.GetSetting(), notification); err != nil {
			common.SysLog(fmt.Sprintf("failed to notify ticket user %d: %s", user.Id, err.Error()))
		}
	}
}

func ticketOwnerUsers(t *model.Ticket) []model.User {
	if t == nil || t.UserId <= 0 {
		return nil
	}
	var user model.User
	if err := model.DB.Select("id", "email", "setting", "role", "status").
		Where("id = ? AND status = ?", t.UserId, common.UserStatusEnabled).
		First(&user).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError(fmt.Sprintf("failed to load ticket owner %d for notification: %s", t.UserId, err.Error()))
		}
		return nil
	}
	return []model.User{user}
}

func ticketResellerIdForRole(t *model.Ticket, role string) int {
	if t == nil || t.TenantId <= 0 || t.ResellerId <= 0 {
		return 0
	}

	var reseller model.Reseller
	if err := model.DB.Select("id", "tenant_id", "parent_reseller_id", "level", "status").
		Where("tenant_id = ? AND id = ? AND status = ?", t.TenantId, t.ResellerId, model.ResellerStatusEnabled).
		First(&reseller).Error; err != nil {
		return 0
	}

	switch role {
	case model.TicketAssigneeL2:
		if reseller.Level == 2 {
			return reseller.Id
		}
	case model.TicketAssigneeL1:
		if reseller.Level == 1 {
			return reseller.Id
		}
		if reseller.Level == 2 && reseller.ParentResellerId != nil {
			return *reseller.ParentResellerId
		}
	}
	return 0
}

func dedupTicketUsers(users []model.User) []model.User {
	seen := map[int]struct{}{}
	out := make([]model.User, 0, len(users))
	for _, user := range users {
		if user.Id <= 0 {
			continue
		}
		if _, ok := seen[user.Id]; ok {
			continue
		}
		out = append(out, user)
		seen[user.Id] = struct{}{}
	}
	return out
}

func capTicketUsers(users []model.User, label string) []model.User {
	if len(users) <= 10 {
		return users
	}
	common.SysLog(fmt.Sprintf("ticket notification %s users capped at 10 from %d", label, len(users)))
	return users[:10]
}

func truncateTicketText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
