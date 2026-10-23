package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminArenaPrizeResourceAllowed follows the actual original prize consumer.
func AdminArenaPrizeResourceAllowed(id string) bool {
	if id == "coin" || id == "popularity" {
		return true
	}
	for _, allowed := range strings.Split(constUtil.P11ArenaPrizeOriginalItems, ",") {
		if id == allowed {
			return true
		}
	}
	return false
}

func adminArenaPrizePoolDTO(policy db.P11ArenaPrizePolicy) (model.AdminArenaPrizePoolSettings, error) {
	var pool []p11ArenaPrizeOption
	if json.Unmarshal([]byte(policy.PoolJSON), &pool) != nil {
		return model.AdminArenaPrizePoolSettings{}, constUtil.ErrAdminConfiguration
	}
	out := model.AdminArenaPrizePoolSettings{Enabled: policy.Enabled, StartAt: policy.StartAt, EndAt: policy.EndAt, MinLevel: policy.MinLevel, DailyWins: policy.DailyWins, WindowWins: policy.WindowWins, Items: []model.AdminArenaPrizePoolItem{}}
	for _, item := range pool {
		out.Items = append(out.Items, model.AdminArenaPrizePoolItem{ResourceID: item.ID, Amount: int64(item.Num), Weight: item.Weight})
	}
	return out, nil
}

func adminReadArenaPrize(tx *gorm.DB, nowMonth string) (*model.AdminArenaPrizeResponse, *db.P11ArenaPrizePolicy, error) {
	tx = tx.Session(&gorm.Session{})
	var policy db.P11ArenaPrizePolicy
	if err := tx.First(&policy, constUtil.P11ArenaPrizePolicyID).Error; err != nil {
		return nil, nil, err
	}
	minimum, maximum, err := p11ArenaPolicyRegularCoinRange(&policy)
	if err != nil {
		return nil, nil, constUtil.ErrAdminConfiguration
	}
	pool, err := adminArenaPrizePoolDTO(policy)
	if err != nil {
		return nil, nil, err
	}
	var frozen int64
	if err = tx.Session(&gorm.Session{}).Model(&db.P11ArenaPrizeOffer{}).Where("month=?", nowMonth).Count(&frozen).Error; err != nil {
		return nil, nil, err
	}
	allowed := append([]string{"coin", "popularity"}, strings.Split(constUtil.P11ArenaPrizeOriginalItems, ",")...)
	out := &model.AdminArenaPrizeResponse{Version: adminVersion(policy), RegularCoin: model.AdminArenaCoinRange{Min: minimum, Max: maximum}, OperatorPool: pool, OperatorPoolFrozen: frozen > 0, Notes: constUtil.AdminArenaPrizeNotes, AllowedResourceIDs: allowed}
	if tx.Migrator().HasTable(&db.P11ArenaDailyPolicy{}) {
		var daily db.P11ArenaDailyPolicy
		if err := tx.Session(&gorm.Session{}).First(&daily, constUtil.P11ArenaDailyPolicyID).Error; err != nil {
			return nil, nil, err
		}
		out.DailyRank = &model.AdminArenaDailyRankStatus{Enabled: daily.Enabled && p11ArenaDailyPolicyValid(&daily), CutoffSecond: daily.CutoffSecond, MinChallenges: daily.MinChallenges}
	}
	return out, &policy, nil
}

// Admin requests do not carry an authenticated character calendar context.
// Read the same persistent server clock directly; saves share-lock it before
// prize rows so a date adjustment cannot change the freeze month mid-save.
func (s *GameService) adminArenaPrizeNow(tx *gorm.DB, lock bool) (time.Time, error) {
	if !s.ServerClockReady {
		return LocalPlayNow(tx.Statement.Context), nil
	}
	query := tx.Session(&gorm.Session{})
	if lock {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	var clock db.ServerGameClock
	if err := query.First(&clock, "server_id=?", s.serverGameClockID()).Error; err != nil {
		return time.Time{}, err
	}
	if !serverGameClockValid(&clock) {
		return time.Time{}, constUtil.ErrAdminConfiguration
	}
	return time.Now().AddDate(0, 0, int(clock.OffsetDays)), nil
}

// AdminArenaPrizes reads typed persistent fields without changing game awards.
func (s *GameService) AdminArenaPrizes(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminArenaPrizeResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	now, err := s.adminArenaPrizeNow(tx, false)
	if err != nil {
		return nil, err
	}
	month, _ := p11ArenaPrizeMonth(now)
	out, _, err := adminReadArenaPrize(tx, month)
	return out, err
}

// AdminSaveArenaPrizes shares the prize policy row lock with actual settlement.
// Only future fights consume the new range; accepted draw snapshots stay intact.
func (s *GameService) AdminSaveArenaPrizes(ctx context.Context, actor *model.AdminPrincipal, req model.AdminArenaPrizeSaveRequest) (*model.AdminArenaPrizeResponse, error) {
	var out *model.AdminArenaPrizeResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		now, err := s.adminArenaPrizeNow(tx, true)
		if err != nil {
			return err
		}
		month, _ := p11ArenaPrizeMonth(now)
		current, policy, err := adminReadArenaPrize(tx.Clauses(clause.Locking{Strength: "UPDATE"}), month)
		if err != nil {
			return err
		}
		if current.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		changedPool := req.OperatorPool != nil && !reflect.DeepEqual(*req.OperatorPool, current.OperatorPool)
		if changedPool {
			if current.OperatorPoolFrozen {
				return fmt.Errorf("%w: 本月运营奖池已冻结，常规金币范围仍可单独保存", constUtil.ErrAdminConfigConflict)
			}
			pool := req.OperatorPool
			entries := []p11ArenaPrizeOption{}
			for _, item := range pool.Items {
				entries = append(entries, p11ArenaPrizeOption{ID: item.ResourceID, Num: int32(item.Amount), Weight: item.Weight})
			}
			raw, err := json.Marshal(entries)
			if err != nil {
				return err
			}
			policy.Enabled, policy.StartAt, policy.EndAt = pool.Enabled, pool.StartAt, pool.EndAt
			policy.MinLevel, policy.DailyWins, policy.WindowWins = pool.MinLevel, pool.DailyWins, pool.WindowWins
			policy.Version = "admin-arena-" + uuid.NewString()
			policy.Source = constUtil.P11ArenaPrizeSource
			policy.PoolJSON = string(raw)
			check := *policy
			if !pool.Enabled && pool.StartAt == 0 && pool.EndAt == 0 {
				policy.PublishedAt = 0
				check.PublishedAt, check.StartAt, check.EndAt = 1, 1, 2
			} else {
				policy.PublishedAt = now.UnixMilli()
				check.PublishedAt = policy.PublishedAt
				if policy.StartAt < policy.PublishedAt {
					return fmt.Errorf("%w: 运营奖池开始时间须晚于保存时间", constUtil.ErrAdminInput)
				}
			}
			if _, err = p11ArenaPrizePool(&check); err != nil {
				return fmt.Errorf("%w: %s", constUtil.ErrAdminInput, err)
			}
		}
		policy.RegularCoinMin, policy.RegularCoinMax = req.RegularCoin.Min, req.RegularCoin.Max
		if err = tx.Save(policy).Error; err != nil {
			return err
		}
		detail := fmt.Sprintf("regularCoin=%d..%d -> %d..%d; operatorPoolChanged=%t; before=%s; accepted receipts retained", current.RegularCoin.Min, current.RegularCoin.Max, req.RegularCoin.Min, req.RegularCoin.Max, changedPool, current.Version)
		if err = adminAudit(tx, actor, constUtil.AdminArenaPrizeAuditAction, constUtil.AdminArenaPrizeAuditTarget, detail); err != nil {
			return err
		}
		out, _, err = adminReadArenaPrize(tx, month)
		return err
	})
	return out, err
}
