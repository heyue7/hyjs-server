package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeAdminOperations 仅创建运营结构，空公告及活动不自动投放奖励。
func InitializeAdminOperations(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminAudit{}, &db.AdminAnnouncement{}, &db.AdminGrantReceipt{})
}
func adminJSON(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
func adminVersion(v any) string       { return adminDigest(string(adminJSON(v))) }
func adminOperationUser(tx *gorm.DB, p *model.AdminPrincipal, write bool) error {
	if p == nil || p.UserID < 1 {
		return constUtil.ErrAdminPermission
	}
	var u db.AdminUser
	if err := tx.First(&u, p.UserID).Error; err != nil {
		return constUtil.ErrAdminPermission
	}
	if !u.Enabled || !AdminRoleValid(u.Role) || (write && u.Role == constUtil.AdminRoleViewer) {
		return constUtil.ErrAdminPermission
	}
	return nil
}
func adminAudit(tx *gorm.DB, p *model.AdminPrincipal, action, target, detail string) error {
	return tx.Create(&db.AdminAudit{Time: time.Now().UnixMilli(), Actor: p.Username, Action: action, Target: target, Detail: detail}).Error
}

// AdminCatalogue 读取已加载的原目录；没有创建或替换游戏原物品。
func (s *GameService) AdminCatalogue(q, category string, page, size int64) model.AdminItemsResponse {
	return s.AdminCatalogueFiltered(model.AdminItemsRequest{Q: q, Category: category, Page: page, PageSize: size})
}

// AdminPlayers 返回有限玩家索引，不含账号凭据或完整存档。
func AdminPlayers(ctx context.Context, q string) ([]map[string]any, error) {
	var rows []db.Profile
	query := db.GetIns().WithContext(ctx).Select("id", "server_id", "player")
	if id, err := strconv.ParseInt(q, 10, 64); err == nil && id > 0 {
		query = query.Where("id=?", id)
	}
	if err := query.Order("id").Limit(500).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		empty := "{}"
		p, _, err := decodeProfile(&db.Profile{Id: r.Id, Player: r.Player, Game: &empty})
		if err != nil {
			continue
		}
		if q != "" && strconv.FormatInt(r.Id, 10) != q && !strings.Contains(p.GetName(), q) {
			continue
		}
		out = append(out, map[string]any{"id": r.Id, "name": p.GetName(), "level": p.GetLevel()})
		if len(out) == 50 {
			break
		}
	}
	return out, nil
}

// AdminOverview 汇总真实运行计数，避免读取玩家私密字段。
func AdminOverview(ctx context.Context, itemCount int64) (map[string]any, error) {
	d := db.GetIns().WithContext(ctx)
	counts := map[string]int64{"items": itemCount}
	for key, table := range map[string]string{"players": "profiles", "activities": "p4_activity_registry", "announcements": "admin_announcements"} {
		var n int64
		if err := d.Table(table).Count(&n).Error; err != nil {
			return nil, err
		}
		counts[key] = n
	}
	var audit []db.AdminAudit
	if err := d.Order("id DESC").Limit(8).Find(&audit).Error; err != nil {
		return nil, err
	}
	if audit == nil {
		audit = []db.AdminAudit{}
	}
	return map[string]any{"server": map[string]string{"version": "HYJS Go", "status": "running"}, "counts": counts, "recentAudit": audit}, nil
}
func adminActivityProjection(domain string, v any, rewards []db.GameplayPoolReward) model.AdminActivityDTO {
	a := model.AdminActivityDTO{Type: domain, Configured: true}
	switch r := v.(type) {
	case db.GameplayPool:
		a.ID = "pool:" + r.ID
		a.Name = r.Name
		a.Enabled = r.Enabled
		a.Source = r.Source
		a.Config = adminJSON(map[string]any{"pool": r, "rewards": rewards})
	case db.P4CommerceRule:
		a.ID = "commerce:" + r.ID
		a.Name = r.Kind + " · " + r.ID
		a.Enabled = r.Enabled
		a.Source = r.Source
		m := map[string]any{}
		_ = json.Unmarshal(adminJSON(r), &m)
		var rules any
		_ = json.Unmarshal(r.Rules, &rules)
		m["rules"] = rules
		a.Config = adminJSON(m)
	case db.P4WelfareRule:
		a.ID = "welfare:" + r.ID
		a.Name = r.Kind + " · " + r.ID
		a.Enabled = r.Enabled
		a.Source = r.Source
		m := map[string]any{}
		_ = json.Unmarshal(adminJSON(r), &m)
		var rules any
		_ = json.Unmarshal(r.Rules, &rules)
		m["rules"] = rules
		a.Config = adminJSON(m)
	case db.PeriodInstance:
		a.ID = "period:" + r.ID
		a.Name = r.Title
		a.Enabled = r.Enabled
		a.Source = r.Source
		a.Config = adminJSON(r)
	case db.P4ActivityRegistry:
		a.ID = "registry:" + r.Name
		a.Name = r.Name
		a.Source = r.Evidence
		a.Configured = false
		a.Config = adminJSON(map[string]string{"protocols": r.Protocols, "status": r.Status})
	}
	a.Version = adminVersion(a.Config)
	return a
}

// AdminActivities 展示真实运营实例及未配置的原活动登记。
func AdminActivities(ctx context.Context) ([]model.AdminActivityDTO, error) {
	d := db.GetIns().WithContext(ctx)
	out := []model.AdminActivityDTO{}
	var pools []db.GameplayPool
	var commerce []db.P4CommerceRule
	var welfare []db.P4WelfareRule
	var periods []db.PeriodInstance
	var registry []db.P4ActivityRegistry
	if err := d.Order("id").Find(&pools).Error; err != nil {
		return nil, err
	}
	for _, r := range pools {
		var rewards []db.GameplayPoolReward
		if err := d.Where("pool_id=?", r.ID).Order("id").Find(&rewards).Error; err != nil {
			return nil, err
		}
		if rewards == nil {
			rewards = []db.GameplayPoolReward{}
		}
		out = append(out, adminActivityProjection("pool", r, rewards))
	}
	if err := d.Order("id").Find(&commerce).Error; err != nil {
		return nil, err
	}
	for _, r := range commerce {
		out = append(out, adminActivityProjection("commerce", r, nil))
	}
	if err := d.Order("id").Find(&welfare).Error; err != nil {
		return nil, err
	}
	for _, r := range welfare {
		out = append(out, adminActivityProjection("welfare", r, nil))
	}
	if err := d.Order("id").Find(&periods).Error; err != nil {
		return nil, err
	}
	for _, r := range periods {
		out = append(out, adminActivityProjection("period", r, nil))
	}
	if err := d.Order("id").Find(&registry).Error; err != nil {
		return nil, err
	}
	for _, r := range registry {
		out = append(out, adminActivityProjection("registry", r, nil))
	}
	return out, nil
}
func adminDecodeActivityRule(raw json.RawMessage, out any) error {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return constUtil.ErrAdminInput
	}
	rules := m["rules"]
	delete(m, "rules")
	if json.Unmarshal(adminJSON(m), out) != nil {
		return constUtil.ErrAdminInput
	}
	switch r := out.(type) {
	case *db.P4CommerceRule:
		r.Rules = rules
	case *db.P4WelfareRule:
		r.Rules = rules
	}
	return nil
}

// AdminUpdateActivity 使用版本与事务锁提交运营字段；保留已冻结玩家和全局历史。
func (s *GameService) AdminUpdateActivity(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminActivityUpdateRequest) (*model.AdminActivityDTO, error) {
	var saved model.AdminActivityDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		domain, key, ok := strings.Cut(id, ":")
		if !ok {
			return constUtil.ErrAdminInput
		}
		switch domain {
		case "pool":
			var old db.GameplayPool
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id=?", key).Error; err != nil {
				return err
			}
			var prior []db.GameplayPoolReward
			if err := tx.Where("pool_id=?", key).Order("id").Find(&prior).Error; err != nil {
				return err
			}
			current := adminActivityProjection(domain, old, prior)
			if current.Version != req.Version {
				return constUtil.ErrAdminConfigConflict
			}
			var cfg struct {
				Pool    db.GameplayPool         `json:"pool"`
				Rewards []db.GameplayPoolReward `json:"rewards"`
			}
			if json.Unmarshal(req.Config, &cfg) != nil || cfg.Pool.ID != key || cfg.Pool.Name == "" || len(cfg.Pool.Source) == 0 || len(cfg.Pool.Source) > 255 || cfg.Pool.MinLevel < 1 || (len(cfg.Rewards) == 0 && req.Enabled) || len(cfg.Rewards) > 200 {
				return constUtil.ErrAdminInput
			}
			cfg.Pool.Enabled = req.Enabled
			seen := map[int32]bool{}
			var total int64
			for i, r := range cfg.Rewards {
				if seen[r.ID] || r.ID < 0 || r.Amount < 1 || r.Weight < 1 || r.Weight > 1000000000 || r.Stock < 0 {
					return constUtil.ErrAdminInput
				}
				seen[r.ID] = true
				total += r.Weight
				valid, err := s.openServerRewardSupported(fmt.Sprintf("%s,%d", r.ResourceID, r.Amount))
				if strings.HasPrefix(r.ResourceID, "pet_") {
					valid = s.turntableResource(r.ResourceID)
					err = nil
				}
				if err != nil || !valid {
					return constUtil.ErrAdminInput
				}
				cfg.Rewards[i].PoolID = key
			}
			if total > 100000000000 {
				return constUtil.ErrAdminInput
			}
			if cfg.Pool.StartAt < 0 || cfg.Pool.EndAt <= cfg.Pool.StartAt || cfg.Pool.RewardEndAt < cfg.Pool.EndAt || cfg.Pool.JackpotPerMillion < 0 || cfg.Pool.JackpotPerMillion > 1000000 || cfg.Pool.PoolContributionBps < 0 || cfg.Pool.PoolContributionBps > 10000 || cfg.Pool.FreePerDay < -1 || cfg.Pool.FreeSeconds < 1 || cfg.Pool.GuaranteeEvery < 0 || cfg.Pool.CoinPerDraw < 0 {
				return constUtil.ErrAdminInput
			}
			for _, cost := range []string{cfg.Pool.SingleCost, cfg.Pool.TenCost, cfg.Pool.SingleTicket, cfg.Pool.TenTicket} {
				if cost != "" {
					if _, err := p3Resources(cost); err != nil {
						return constUtil.ErrAdminInput
					}
				}
			}
			// Existing private/frozen snapshots retain their rules; only future snapshots read this source.
			if err := tx.Save(&cfg.Pool).Error; err != nil {
				return err
			}
			if err := tx.Where("pool_id=?", key).Delete(&db.GameplayPoolReward{}).Error; err != nil {
				return err
			}
			if len(cfg.Rewards) > 0 {
				if err := tx.Create(&cfg.Rewards).Error; err != nil {
					return err
				}
			}
			if len(cfg.Rewards) == 0 {
				saved = adminActivityProjection(domain, cfg.Pool, cfg.Rewards)
				break
			}
			if !req.Enabled {
				if err := tx.Model(&db.GameplayPool{}).Where("id=?", key).Update("enabled", true).Error; err != nil {
					return err
				}
			}
			var n int64
			if err := tx.Model(&db.TurntableRules{}).Where("pool_id=?", key).Count(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				if _, _, _, err := s.turntableConfigForPool(tx, key); err != nil {
					return fmt.Errorf("%w: %s", constUtil.ErrAdminInput, err.Error())
				}
			} else {
				if _, _, err := s.loadGameplayPool(tx, key); err != nil {
					return fmt.Errorf("%w: %s", constUtil.ErrAdminInput, err.Error())
				}
			}
			if !req.Enabled {
				if err := tx.Model(&db.GameplayPool{}).Where("id=?", key).Update("enabled", false).Error; err != nil {
					return err
				}
			}
			saved = adminActivityProjection(domain, cfg.Pool, cfg.Rewards)
		case "commerce":
			var old db.P4CommerceRule
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id=?", key).Error; err != nil {
				return err
			}
			current := adminActivityProjection(domain, old, nil)
			if current.Version != req.Version {
				return constUtil.ErrAdminConfigConflict
			}
			var cfg db.P4CommerceRule
			if adminDecodeActivityRule(req.Config, &cfg) != nil || cfg.ID != key || cfg.Kind != old.Kind || cfg.ShopType != old.ShopType {
				return constUtil.ErrAdminInput
			}
			cfg.Enabled = req.Enabled
			if req.Enabled {
				if _, err := s.validateP4Commerce(cfg); err != nil {
					return adminActivityValidationError(err)
				}
			}
			old.Enabled = req.Enabled
			if len(old.Frozen) > 0 && adminActivityProjection(domain, old, nil).Version != adminActivityProjection(domain, cfg, nil).Version {
				return constUtil.ErrAdminConfigConflict
			}
			cfg.Frozen = old.Frozen
			if err := tx.Save(&cfg).Error; err != nil {
				return err
			}
			saved = adminActivityProjection(domain, cfg, nil)
		case "welfare":
			var old db.P4WelfareRule
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id=?", key).Error; err != nil {
				return err
			}
			current := adminActivityProjection(domain, old, nil)
			if current.Version != req.Version {
				return constUtil.ErrAdminConfigConflict
			}
			var cfg db.P4WelfareRule
			if adminDecodeActivityRule(req.Config, &cfg) != nil || cfg.ID != key || cfg.Kind != old.Kind {
				return constUtil.ErrAdminInput
			}
			cfg.Enabled = req.Enabled
			if req.Enabled {
				if _, err := s.validateP4Welfare(cfg); err != nil {
					return adminActivityValidationError(err)
				}
			}
			old.Enabled = req.Enabled
			if len(old.Frozen) > 0 && adminActivityProjection(domain, old, nil).Version != adminActivityProjection(domain, cfg, nil).Version {
				return constUtil.ErrAdminConfigConflict
			}
			cfg.Frozen = old.Frozen
			if err := tx.Save(&cfg).Error; err != nil {
				return err
			}
			saved = adminActivityProjection(domain, cfg, nil)
		case "period":
			var old db.PeriodInstance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id=?", key).Error; err != nil {
				return err
			}
			current := adminActivityProjection(domain, old, nil)
			if current.Version != req.Version {
				return constUtil.ErrAdminConfigConflict
			}
			var cfg db.PeriodInstance
			if json.Unmarshal(req.Config, &cfg) != nil || cfg.ID != key || cfg.Kind != old.Kind || cfg.Mode != old.Mode {
				return constUtil.ErrAdminInput
			}
			cfg.Enabled = req.Enabled
			if err := s.validateP1Period(cfg); err != nil {
				return adminActivityValidationError(err)
			}
			old.Enabled = req.Enabled
			if len(old.Frozen) > 0 && adminActivityProjection(domain, old, nil).Version != adminActivityProjection(domain, cfg, nil).Version {
				return constUtil.ErrAdminConfigConflict
			}
			cfg.Frozen = old.Frozen
			if err := tx.Save(&cfg).Error; err != nil {
				return err
			}
			saved = adminActivityProjection(domain, cfg, nil)
		default:
			return constUtil.ErrAdminInput
		}
		return adminAudit(tx, p, "activity.update", id, "before="+req.Version+" after="+saved.Version)
	})
	return &saved, err
}

// AdminAnnouncements 读取真实公告，版本摘要用于避免互相覆盖。
func AdminAnnouncements(ctx context.Context, public bool) ([]db.AdminAnnouncement, error) {
	q := db.GetIns().WithContext(ctx)
	if public {
		now := time.Now().UnixMilli()
		q = q.Where("enabled=? AND start_at<=? AND (end_at=0 OR end_at>?)", true, now, now)
	}
	var rows []db.AdminAnnouncement
	if err := q.Order("id DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []db.AdminAnnouncement{}
	}
	for i := range rows {
		rows[i].Version = adminVersion(rows[i])
	}
	return rows, nil
}

// AdminSaveAnnouncement 保存纯文本及真实发布窗口，审计与公告同事务。
func AdminSaveAnnouncement(ctx context.Context, p *model.AdminPrincipal, id int64, req model.AdminAnnouncementRequest) (*db.AdminAnnouncement, error) {
	var saved db.AdminAnnouncement
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if id > 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&saved, id).Error; err != nil {
				return err
			}
			if adminVersion(saved) != req.Version {
				return constUtil.ErrAdminConfigConflict
			}
		}
		saved.Title = req.Title
		saved.Content = req.Content
		saved.Enabled = req.Enabled
		saved.StartAt = req.StartAt
		saved.EndAt = req.EndAt
		saved.UpdatedAtMillis = time.Now().UnixMilli()
		if err := tx.Save(&saved).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "announcement.save", strconv.FormatInt(saved.ID, 10), saved.Title)
	})
	saved.Version = adminVersion(saved)
	return &saved, err
}

// AdminGrant 将幂等附件投递到原邮件和领奖中心，提交时不直接改玩家库存。
func (s *GameService) AdminGrant(ctx context.Context, p *model.AdminPrincipal, req model.AdminGrantRequest) (*model.AdminGrantResponse, error) {
	var result model.AdminGrantResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if _, err := uuid.Parse(req.RequestID); err != nil {
			return constUtil.ErrAdminInput
		}
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "server_id").First(&profile, req.PlayerID).Error; err != nil {
			return err
		}
		if err := AdminCheckProfileScope(ctx, profile); err != nil {
			return err
		}
		hash := adminVersion(req)
		var prior db.AdminGrantReceipt
		err := tx.First(&prior, "request_id=?", req.RequestID).Error
		if err == nil {
			if prior.ActorID != p.UserID || prior.BodyHash != hash {
				return constUtil.ErrAdminConfigConflict
			}
			result = model.AdminGrantResponse{DeliveryID: prior.DeliveryID, RequestID: req.RequestID}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		supported, supportErr := s.openServerRewardSupported(fmt.Sprintf("%s,%d", req.ResourceID, req.Amount))
		if supportErr != nil || !supported {
			return constUtil.ErrAdminInput
		}
		delivery, err := s.EnqueueRewardNotice(tx, req.PlayerID, "admin-grant:"+req.RequestID, "运营发放", req.Reason, fmt.Sprintf("%s,%d", req.ResourceID, req.Amount), 0)
		if err != nil {
			return err
		}
		if err = tx.Create(&db.AdminGrantReceipt{RequestID: req.RequestID, ActorID: p.UserID, BodyHash: hash, DeliveryID: delivery.ID}).Error; err != nil {
			return err
		}
		result = model.AdminGrantResponse{DeliveryID: delivery.ID, RequestID: req.RequestID}
		return adminAudit(tx, p, "resource.grant", strconv.FormatInt(req.PlayerID, 10), fmt.Sprintf("%s x%d; request=%s; %s", req.ResourceID, req.Amount, req.RequestID, req.Reason))
	})
	return &result, err
}

// AdminAuditPage 查询真实已提交变更，不回传密码或会话口令。
func AdminAuditPage(ctx context.Context, page, size int64) (map[string]any, error) {
	d := db.GetIns().WithContext(ctx)
	var total int64
	var rows []db.AdminAudit
	if err := d.Model(&db.AdminAudit{}).Count(&total).Error; err != nil {
		return nil, err
	}
	if err := d.Order("id DESC").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []db.AdminAudit{}
	}
	return map[string]any{"items": rows, "total": total}, nil
}
