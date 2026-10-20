package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// adminSecurityEventLastCleanup 只允许本进程每5分钟触发一次有界清理，避免每个请求执行大DELETE。
var adminSecurityEventLastCleanup atomic.Int64

// InitializeSecurityEvents 创建独立安全事件表，并在启动时做一次不阻塞启动的有界保留清理。
func InitializeSecurityEvents(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.AdminSecurityEvent{}); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	adminSecurityEventLastCleanup.Store(now)
	// 启动清理失败不阻止后台开放；后续记录仍会按5分钟重试。
	_ = adminSecurityEventCleanup(ctx, now)
	return nil
}

// RecordSecurityEvent 在独立事务外写入已清洗的安全事件，并按分钟聚合同组次数。
// 调用方必须在业务事务结束后调用，且不能因本函数失败改变登录或业务结果。
func RecordSecurityEvent(ctx context.Context, event model.SecurityEventInput) error {
	return adminSecurityEventRecordAt(ctx, event, time.Now().UnixMilli())
}

// adminSecurityEventRecordAt 允许测试固定分钟桶，生产入口始终使用当前毫秒时间。
func adminSecurityEventRecordAt(ctx context.Context, event model.SecurityEventInput, now int64) error {
	actor := adminSecurityEventText(event.Actor, constUtil.AdminSecurityEventActorMax)
	target := adminSecurityEventText(event.Target, constUtil.AdminSecurityEventTargetMax)
	action := strings.TrimSpace(event.Action)
	outcome := strings.ToLower(strings.TrimSpace(event.Outcome))
	reason := strings.TrimSpace(event.ReasonCode)
	requestID := strings.TrimSpace(event.RequestID)

	// 来源IP只接受可解析地址并保存规范形式，不接受原始转发头或带端口文本。
	ip := net.ParseIP(strings.TrimSpace(event.SourceIP))
	if ip == nil {
		return constUtil.ErrAdminSecurityEventInput
	}
	sourceIP := ip.String()

	if !adminSecurityEventCodeValid(action, constUtil.AdminSecurityEventActionMax) ||
		!adminSecurityEventOutcomeValid(outcome) ||
		(reason != "" && !adminSecurityEventCodeValid(reason, constUtil.AdminSecurityEventReasonMax)) ||
		(requestID != "" && !adminSecurityEventCodeValid(requestID, constUtil.AdminSecurityEventRequestIDMax)) {
		return constUtil.ErrAdminSecurityEventInput
	}

	key := adminSecurityEventKey(now/constUtil.AdminSecurityEventMinuteMillis, actor, sourceIP, action, target, outcome, reason)
	row := db.AdminSecurityEvent{
		EventKey:    key,
		FirstAt:     now,
		LastAt:      now,
		Actor:       actor,
		SourceIP:    sourceIP,
		Action:      action,
		Target:      target,
		Outcome:     outcome,
		ReasonCode:  reason,
		RequestID:   requestID,
		Occurrences: 1,
	}
	// 唯一键冲突时原子累加次数并推进最近时间，不覆盖首次时间。
	if err := db.GetIns().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_at":     gorm.Expr("CASE WHEN last_at < ? THEN ? ELSE last_at END", now, now),
			"occurrences": gorm.Expr("occurrences + 1"),
			"request_id":  gorm.Expr("CASE WHEN ? <> '' THEN ? ELSE request_id END", requestID, requestID),
		}),
	}).Create(&row).Error; err != nil {
		return err
	}
	adminSecurityEventMaybeCleanup(ctx, now)
	return nil
}

// AdminSecurityEvents 仅向当前仍为admin的已认证账号返回一页安全事件。
func AdminSecurityEvents(ctx context.Context, principal *model.AdminPrincipal, req model.AdminSecurityEventsRequest) (*model.AdminSecurityEventsResponse, error) {
	if err := adminSecurityEventRequireAdmin(ctx, principal); err != nil {
		return nil, err
	}
	d := db.GetIns().WithContext(ctx)
	query := func() *gorm.DB {
		q := d.Model(&db.AdminSecurityEvent{})
		if req.Action != "" {
			q = q.Where("action = ?", req.Action)
		}
		if req.Outcome != "" {
			q = q.Where("outcome = ?", req.Outcome)
		}
		if req.Actor != "" {
			q = q.Where("actor = ?", req.Actor)
		}
		return q
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []db.AdminSecurityEvent
	if err := query().Order("last_at DESC").Order("id DESC").Offset(int((req.Page - 1) * req.PageSize)).Limit(int(req.PageSize)).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]model.AdminSecurityEventDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.AdminSecurityEventDTO{
			ID: row.ID, FirstAt: row.FirstAt, LastAt: row.LastAt, Actor: row.Actor,
			SourceIP: row.SourceIP, Action: row.Action, Target: row.Target,
			Outcome: row.Outcome, ReasonCode: row.ReasonCode, RequestID: row.RequestID,
			Occurrences: row.Occurrences,
		})
	}
	return &model.AdminSecurityEventsResponse{Items: items, Total: total}, nil
}

// adminSecurityEventRequireAdmin 在服务端重新读取当前角色，避免旧会话继续访问新增安全日志。
func adminSecurityEventRequireAdmin(ctx context.Context, principal *model.AdminPrincipal) error {
	if principal == nil || principal.UserID < 1 {
		return constUtil.ErrAdminPermission
	}
	var user db.AdminUser
	if err := db.GetIns().WithContext(ctx).Select("id", "role", "enabled").First(&user, principal.UserID).Error; err != nil {
		return constUtil.ErrAdminPermission
	}
	if !user.Enabled || user.Role != constUtil.AdminRoleAdmin {
		return constUtil.ErrAdminPermission
	}
	return nil
}

// adminSecurityEventText 清理控制字符并按字符数截断，防止日志注入和超长字段。
func adminSecurityEventText(value string, max int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > max {
		value = string(runes[:max])
	}
	return value
}

// adminSecurityEventCodeValid 只允许固定短码字符，拒绝空格和任意文本。
func adminSecurityEventCodeValid(value string, max int) bool {
	if value == "" || len([]rune(value)) > max {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == ':' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func adminSecurityEventOutcomeValid(outcome string) bool {
	switch outcome {
	case "", constUtil.AdminSecurityEventOutcomeSuccess, constUtil.AdminSecurityEventOutcomeFailure,
		constUtil.AdminSecurityEventOutcomeDenied, constUtil.AdminSecurityEventOutcomeThrottled:
		return true
	default:
		return false
	}
}

// adminSecurityEventKey 绑定分钟桶与全部展示维度，同组事件只保留一条累计记录。
func adminSecurityEventKey(minute int64, values ...string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(strconv.FormatInt(minute, 10)))
	for _, value := range values {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(value))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// adminSecurityEventMaybeCleanup 以进程级时间窗限制清理频率，失败时不影响事件写入。
func adminSecurityEventMaybeCleanup(ctx context.Context, now int64) {
	last := adminSecurityEventLastCleanup.Load()
	if now-last < constUtil.AdminSecurityEventCleanupIntervalMS {
		return
	}
	if !adminSecurityEventLastCleanup.CompareAndSwap(last, now) {
		return
	}
	_ = adminSecurityEventCleanup(ctx, now)
}

// adminSecurityEventCleanup 每次最多删除1000条过期记录，且只操作自身表。
func adminSecurityEventCleanup(ctx context.Context, now int64) error {
	d := db.GetIns().WithContext(ctx)
	cutoff := now - constUtil.AdminSecurityEventRetentionMillis
	var ids []int64
	if err := d.Model(&db.AdminSecurityEvent{}).Where("last_at < ?", cutoff).Order("id").Limit(constUtil.AdminSecurityEventCleanupBatch).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return d.Where("id IN ?", ids).Delete(&db.AdminSecurityEvent{}).Error
}
