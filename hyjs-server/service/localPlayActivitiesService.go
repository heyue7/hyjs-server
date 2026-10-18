package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

var errLocalPlayActivitiesDisabled = errors.New("试玩活动工具暂未开放")
var errLocalPlayActivitiesInvalid = errors.New("试玩活动请求或新期存档无效")

type localPlayOpenServerSnapshot struct {
	Rule  db.OpenServerActivityRule   `json:"rule"`
	Tasks []db.OpenServerActivityTask `json:"tasks"`
}

func (s *GameService) InitializeLocalPlayActivities(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.LocalPlayActivityBinding{}, &db.LocalPlayActivityCycle{}, &db.LocalPlayActivityOperation{}, &db.LocalPlayOpenServerState{}, &db.LocalPlayOpenServerCounter{}, &db.LocalPlayOpenServerReceipt{}); err != nil {
		return err
	}
	return nil
}

func (s *GameService) localPlayActivitiesAuthorize(ctx context.Context, row *db.Profile) error {
	if !s.LocalPlayToolsEnabled || !LocalPlayContextBound(ctx) {
		return errLocalPlayActivitiesDisabled
	}
	if !localPlayAPIAuthorized(ctx, row) {
		return errLocalPlayActivitiesInvalid
	}
	return nil
}

func localPlayActivitiesPolicy(tx *gorm.DB) error {
	enabled, err := localPlayPolicyEnabled(tx)
	if err != nil {
		return err
	}
	if !enabled {
		return errLocalPlayActivitiesDisabled
	}
	return nil
}

func localPlayActivityOwner(tx *gorm.DB) int64 { return GameCalendarOwner(tx.Statement.Context) }

func localPlayBoundCycle(tx *gorm.DB, id int64, activityID string) (*db.LocalPlayActivityCycle, error) {
	if !LocalPlayContextBound(tx.Statement.Context) {
		return nil, nil
	}
	if id <= 1 || localPlayActivityOwner(tx) != id || !tx.Migrator().HasTable(&db.LocalPlayActivityBinding{}) {
		return nil, nil
	}
	var binding db.LocalPlayActivityBinding
	if err := tx.First(&binding, "player_id=? AND activity_id=?", id, activityID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var cycle db.LocalPlayActivityCycle
	if err := tx.First(&cycle, "id=?", binding.CycleID).Error; err != nil {
		return nil, err
	}
	if binding.Revision < 1 || cycle.PlayerID != id || cycle.ActivityID != activityID || cycle.Source != constUtil.LocalPlayActivitiesVersion || cycle.StartAt <= 0 || cycle.EndAt <= cycle.StartAt || cycle.RewardEndAt < cycle.EndAt {
		return nil, errLocalPlayActivitiesInvalid
	}
	return &cycle, nil
}

func localPlayActivityViews(tx *gorm.DB, id int64, now time.Time) (*model.LocalPlayActivitiesView, error) {
	view := &model.LocalPlayActivitiesView{Activities: []model.LocalPlayActivityView{}, Results: []model.LocalPlayActivityResult{}, TestOnly: true}
	for _, def := range localPlayActivityDefinitions {
		item := model.LocalPlayActivityView{ID: def.ID, Name: def.Name, Status: "unsupported", Description: def.Description}
		if def.Kind != "" {
			item.Status, item.CanOpen, item.CanRefresh = "closed", true, true
			sourceReady := localPlayActivitySourceReady(tx, def)
			if !sourceReady {
				item.CanOpen, item.CanRefresh = false, false
				item.Description += "；待后台配置并启用"
			}
			cycle, err := localPlayBoundCycle(tx, id, def.ID)
			if err != nil {
				return nil, err
			}
			if cycle != nil && sourceReady {
				item.StartsAt, item.EndsAt = cycle.StartAt, cycle.EndAt
				if now.UnixMilli() >= cycle.StartAt && now.UnixMilli() < cycle.RewardEndAt {
					item.Status = "open"
				}
			}
		}
		view.Activities = append(view.Activities, item)
	}
	return view, nil
}

func (s *GameService) LocalPlayActivitiesSnapshot(ctx context.Context, id int64) (*model.LocalPlayActivitiesView, error) {
	var result *model.LocalPlayActivitiesView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, _ *pb.Player, _ *model.GameState) error {
		if err := s.localPlayActivitiesAuthorize(ctx, row); err != nil {
			return err
		}
		if err := localPlayActivitiesPolicy(tx); err != nil {
			return err
		}
		clock := db.LocalPlayClockState{PlayerID: id}
		if err := tx.First(&clock, "player_id=?", id).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if clock.OffsetDays < -3650 || clock.OffsetDays > 3650 || clock.Revision < 0 {
			return errLocalPlayActivitiesInvalid
		}
		ctx = context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, clock.OffsetDays)
		tx = tx.WithContext(context.WithValue(ctx, constUtil.LocalPlayOwnerKey, id))
		var err error
		result, err = localPlayActivityViews(tx, id, LocalPlayNow(ctx))
		return err
	})
	return result, err
}

func (s *GameService) LocalPlayActivitiesApply(ctx context.Context, id int64, action, activityID, requestID string) (*model.LocalPlayActivitiesView, error) {
	if (action != "open" && action != "refresh") || !validLocalWalletKey(requestID) {
		return nil, errLocalPlayActivitiesInvalid
	}
	if _, err := uuid.Parse(requestID); err != nil {
		return nil, errLocalPlayActivitiesInvalid
	}
	found := activityID == "all"
	for _, def := range localPlayActivityDefinitions {
		found = found || def.ID == activityID
	}
	if !found {
		return nil, errLocalPlayActivitiesInvalid
	}
	var result *model.LocalPlayActivitiesView
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, _ *pb.Player, g *model.GameState) error {
		if err := s.localPlayActivitiesAuthorize(ctx, row); err != nil {
			return err
		}
		if g.Run != nil {
			return errors.New("请先正常结束副本再刷新试玩活动")
		}
		if err := localPlayActivitiesPolicy(tx); err != nil {
			return err
		}
		clock := db.LocalPlayClockState{PlayerID: id}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&clock).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&clock, "player_id=?", id).Error; err != nil {
			return err
		}
		if clock.OffsetDays < -3650 || clock.OffsetDays > 3650 || clock.Revision < 0 {
			return errLocalPlayActivitiesInvalid
		}
		ctx = context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, clock.OffsetDays)
		tx = tx.WithContext(context.WithValue(ctx, constUtil.LocalPlayOwnerKey, id))
		var old db.LocalPlayActivityOperation
		err := tx.First(&old, "player_id=? AND request_id=?", id, requestID).Error
		if err == nil {
			if old.Action != action || old.ActivityID != activityID {
				return errors.New("试玩活动操作编号冲突")
			}
			result = &model.LocalPlayActivitiesView{}
			if json.Unmarshal(old.Response, result) != nil || !result.TestOnly {
				return errLocalPlayActivitiesInvalid
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := LocalPlayNow(ctx)
		outcomes := []model.LocalPlayActivityResult{}
		for _, def := range localPlayActivityDefinitions {
			if activityID != "all" && def.ID != activityID {
				continue
			}
			if def.Kind == "" {
				outcomes = append(outcomes, model.LocalPlayActivityResult{ID: def.ID, Status: "unsupported", Message: def.Description})
				continue
			}
			if !localPlayActivitySourceReady(tx, def) {
				outcomes = append(outcomes, model.LocalPlayActivityResult{ID: def.ID, Status: "closed", Message: "待后台配置并启用，本次未开放"})
				continue
			}
			var binding db.LocalPlayActivityBinding
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&binding, "player_id=? AND activity_id=?", id, def.ID).Error
			if err == nil && action == "open" {
				current, e := localPlayBoundCycle(tx, id, def.ID)
				if e != nil {
					return e
				}
				if current != nil && now.UnixMilli() >= current.StartAt && now.UnixMilli() < current.RewardEndAt {
					outcomes = append(outcomes, model.LocalPlayActivityResult{ID: def.ID, Status: "open", Message: "保留当前独立活动期"})
					continue
				}
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if binding.Revision == math.MaxInt64 {
				return errLocalPlayActivitiesInvalid
			}
			digest := sha256.Sum256([]byte(row.Uid + "/" + def.ID + "/" + requestID))
			cycleID := "lp-" + hex.EncodeToString(digest[:13])
			if err := tx.SavePoint("local_play_activity_create").Error; err != nil {
				return err
			}
			cycle, err := s.localPlayCreateCycle(tx, id, def, cycleID, now)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errLocalPlayActivitySource) {
					if e := tx.RollbackTo("local_play_activity_create").Error; e != nil {
						return e
					}
					outcomes = append(outcomes, model.LocalPlayActivityResult{ID: def.ID, Status: "closed", Message: "待后台配置并启用，本次未开放"})
					continue
				}
				return err
			}
			if err := tx.Create(cycle).Error; err != nil {
				return err
			}
			if err := s.localPlayRetireWelfare(tx, id, binding, cycle, now); err != nil {
				return err
			}
			binding.PlayerID, binding.ActivityID, binding.CycleID = id, def.ID, cycleID
			binding.Revision++
			if err := tx.Save(&binding).Error; err != nil {
				return err
			}
			outcomes = append(outcomes, model.LocalPlayActivityResult{ID: def.ID, Status: "open", Message: "已建立独立新期，旧领取和回执保留"})
		}
		result, err = localPlayActivityViews(tx, id, now)
		if err != nil {
			return err
		}
		result.Results = outcomes
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.Create(&db.LocalPlayActivityOperation{PlayerID: id, RequestID: requestID, Action: action, ActivityID: activityID, Response: raw, CreatedAt: now.UnixMilli()}).Error
	})
	return result, err
}

func (s *GameService) localPlayCreateCycle(tx *gorm.DB, id int64, def localPlayActivityDefinition, cycleID string, now time.Time) (*db.LocalPlayActivityCycle, error) {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	cycle := &db.LocalPlayActivityCycle{ID: cycleID, PlayerID: id, ActivityID: def.ID, CreatedAt: now.UnixMilli(), StartAt: start.UnixMilli(), Source: constUtil.LocalPlayActivitiesVersion}
	cycle.RealCreatedAt = time.Now().UnixMilli()
	if tx.Migrator().HasTable(&db.EventFact{}) {
		if err := tx.Model(&db.EventFact{}).Select("COALESCE(MAX(id),0)").Where("player_id=?", id).Scan(&cycle.EventAfterID).Error; err != nil {
			return nil, err
		}
	}
	switch def.Kind {
	case "open-server":
		var snapshot localPlayOpenServerSnapshot
		if err := tx.Where("id=?", constUtil.OpenServerActivityID).First(&snapshot.Rule).Error; err != nil {
			return nil, err
		}
		if err := tx.Order("id").Find(&snapshot.Tasks).Error; err != nil {
			return nil, err
		}
		if len(snapshot.Tasks) != 249 {
			return nil, errors.New("试玩开服原249任务缺失")
		}
		for i := range snapshot.Tasks {
			snapshot.Tasks[i].UsedStock = 0
			if err := validateOpenServerTask(snapshot.Tasks[i]); err != nil {
				return nil, err
			}
		}
		cycle.EndAt, cycle.RewardEndAt = start.AddDate(0, 0, 14).UnixMilli(), start.AddDate(0, 0, 15).UnixMilli()
		snapshot.Rule.ID, snapshot.Rule.Enabled = cycleID, true
		snapshot.Rule.StartAt, snapshot.Rule.EndAt, snapshot.Rule.RewardEndAt = cycle.StartAt, cycle.EndAt, cycle.RewardEndAt
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		cycle.Snapshot = raw
	case "turntable":
		// Clone the configured original red/nonred pool; never overwrite global inventory.
		baseTX := tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.LocalPlayOwnerKey, int64(0)))
		baseID := s.p2TurntablePoolID(baseTX)
		if baseID != constUtil.P2RedWheelPool {
			// 用户截图普通盘用于新个人期，旧已冻结期和独立红武盘保持原账本。
			baseID = constUtil.TurntableScreenshotPool
		}
		pool, rewards, rules, err := s.turntableConfigForPool(tx, baseID)
		if err != nil {
			return nil, err
		}
		if pool.EndAt <= pool.StartAt || pool.RewardEndAt < pool.EndAt {
			return nil, errLocalPlayActivitiesInvalid
		}
		pool.ID, pool.Enabled, pool.Name = cycleID, true, "试玩幸运转盘"
		cycle.EndAt, cycle.RewardEndAt = cycle.StartAt+pool.EndAt-pool.StartAt, cycle.StartAt+pool.RewardEndAt-pool.StartAt
		pool.StartAt, pool.EndAt, pool.RewardEndAt = cycle.StartAt, cycle.EndAt, cycle.RewardEndAt
		if err := tx.Create(pool).Error; err != nil {
			return nil, err
		}
		for i := range rewards {
			rewards[i].PoolID = cycleID
			if err := tx.Create(&rewards[i]).Error; err != nil {
				return nil, err
			}
		}
		rules.PoolID = cycleID
		if err := tx.Create(rules).Error; err != nil {
			return nil, err
		}
		if err := tx.Create(&db.TurntableGlobal{PoolID: cycleID, DiamondPool: pool.InitialDiamondPool, UsedStock: []byte("{}")}).Error; err != nil {
			return nil, err
		}
		raw, err := json.Marshal(pool)
		if err != nil {
			return nil, err
		}
		cycle.Snapshot = raw
	default:
		return s.localPlayCreateScopedCycle(tx, id, def, cycle, now)
	}
	return cycle, nil
}

// Called at the beginning of the original p2TurntablePoolID; non-demo/background calls retain the original pool.
func localPlayTurntablePoolID(tx *gorm.DB) string {
	id := localPlayActivityOwner(tx)
	cycle, err := localPlayBoundCycle(tx, id, "BigTurnTable")
	if err != nil {
		return "invalid-local-play-pool"
	}
	if cycle != nil {
		return cycle.ID
	}
	return ""
}
