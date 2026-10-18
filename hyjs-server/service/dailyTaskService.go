package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type dailyTaskSnapshot struct {
	Tasks []*pb.TaskInfo    `json:"tasks"`
	Boxes []*pb.TaskBoxInfo `json:"boxes"`
}

// InitializeDailyTasks 保留既有 SQL 配置；原服目标缺失时明确采用可审计本地规则。
func (s *GameService) InitializeDailyTasks(ctx context.Context) error {
	if len(s.Tables["DailyTaskRewardBox"]) != 16 {
		return errors.New(constUtil.DailyTaskConfigError)
	}
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.DailyTaskRule{}, &db.DailyTaskBoxRule{}, &db.DailyTaskState{}); err != nil {
		return err
	}
	return d.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		rules := []db.DailyTaskRule{
			{ID: "local_copy", Type: constUtil.DailyTaskCopy, Total: 3, MinLevel: 1},
			{ID: "local_recruit", Type: constUtil.DailyTaskRecruit, Total: 3, MinLevel: 1},
			{ID: "local_equip_strengthen", Type: constUtil.DailyTaskEquipStrengthen, Total: 3, MinLevel: 1},
			{ID: "local_treasure_strengthen", Type: constUtil.DailyTaskTreasureStrengthen, Total: 1, MinLevel: 1},
			{ID: "local_training", Type: constUtil.DailyTaskTraining, Total: 1, MinLevel: 1},
			{ID: "local_combine", Type: constUtil.DailyTaskCombine, Total: 1, MinLevel: 1},
			{ID: "local_equip_refine", Type: constUtil.DailyTaskEquipRefine, Total: 1, MinLevel: 1},
		}
		for i := range rules {
			rules[i].Score = 20
			rules[i].Reward = "coin,1000"
			rules[i].Enabled = true
			rules[i].Source = "本地服日常：原服任务目标与积分未恢复；七类真实完成事件，每任务20分及1000金币；UTC+8日切，领取后加积分"
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rules).Error; err != nil {
			return err
		}
		for _, row := range s.Tables["DailyTaskRewardBox"] {
			parse := func(key string) (int32, error) { n, e := strconv.ParseInt(row[key], 10, 32); return int32(n), e }
			var rule db.DailyTaskBoxRule
			var err error
			rule.ID = row["boxId"]
			rule.Reward = row["reward"]
			rule.Enabled = true
			rule.Source = "APK TableDailyTaskRewardBox 基础奖励；级别区间采用本地左闭右开规则；概率活动奖励未开放"
			if rule.StartLevel, err = parse("startLevel"); err != nil {
				return err
			}
			if rule.EndLevel, err = parse("endLevel"); err != nil {
				return err
			}
			if rule.Score, err = parse("needScore"); err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func dailyTaskTypeSupported(kind int32) bool {
	switch kind {
	case constUtil.DailyTaskCopy, constUtil.DailyTaskRecruit, constUtil.DailyTaskEquipStrengthen, constUtil.DailyTaskTreasureStrengthen, constUtil.DailyTaskTraining, constUtil.DailyTaskCombine, constUtil.DailyTaskEquipRefine:
		return true
	}
	return false
}

// dailyTaskReward 校验原客户端每项任务只接受一条 resource,num，宝箱可接受多条基础资源。
func (s *GameService) dailyTaskReward(value string, single bool) error {
	resources, err := activityLoginResources(value)
	if err != nil || len(resources) == 0 || (single && len(resources) != 1) {
		return errors.New(constUtil.DailyTaskConfigError)
	}
	for _, r := range resources {
		if strings.HasPrefix(r.GetId(), "item_") && s.table("Item", "itemId", r.GetId()) == nil {
			return errors.New(constUtil.DailyTaskConfigError)
		}
	}
	return nil
}

// loadDailyTaskState 在角色行锁内加载日档，仅首次冻结当天 SQL 条件及奖励。
func (s *GameService) loadDailyTaskState(tx *gorm.DB, id int64, level int32, now time.Time) (*db.DailyTaskState, *dailyTaskSnapshot, error) {
	row := &db.DailyTaskState{PlayerID: id, Day: now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")}
	state := &dailyTaskSnapshot{}
	err := tx.Where("player_id = ? AND day = ?", id, row.Day).First(row).Error
	if err == nil {
		if err = json.Unmarshal([]byte(row.Snapshot), state); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	} else {
		var rules []db.DailyTaskRule
		if err := tx.Where("enabled = ? AND min_level <= ?", true, level).Order("type,id").Find(&rules).Error; err != nil {
			return nil, nil, err
		}
		for _, r := range rules {
			if r.ID == "" || r.Source == "" || r.MinLevel < 1 || r.Total < 1 || r.Score < 1 || !dailyTaskTypeSupported(r.Type) {
				return nil, nil, errors.New(constUtil.DailyTaskConfigError)
			}
			state.Tasks = append(state.Tasks, &pb.TaskInfo{TaskId: proto.String(r.ID), Type: proto.Int32(r.Type), Total: proto.Int32(r.Total), Count: proto.Int32(0), Score: proto.Int32(r.Score), Reward: proto.String(r.Reward), State: proto.Int32(constUtil.DailyTaskDoing)})
		}
		var boxes []db.DailyTaskBoxRule
		if err := tx.Where("enabled = ? AND start_level <= ? AND end_level > ?", true, level, level).Order("score,id").Find(&boxes).Error; err != nil {
			return nil, nil, err
		}
		for _, b := range boxes {
			if b.ID == "" || b.Source == "" || b.StartLevel < 1 || b.EndLevel <= b.StartLevel {
				return nil, nil, errors.New(constUtil.DailyTaskConfigError)
			}
			state.Boxes = append(state.Boxes, &pb.TaskBoxInfo{BoxId: proto.String(b.ID), Score: proto.Int32(b.Score), Rewards: proto.String(b.Reward), State: proto.Int32(constUtil.DailyTaskDoing)})
		}
	}
	// 快照结构及奖励均先验证，错误配置不能部分发奖或污染事件计数。
	if len(state.Tasks) == 0 || len(state.Boxes) != 4 {
		return nil, nil, errors.New(constUtil.DailyTaskConfigError)
	}
	seen := map[string]bool{}
	var total int64
	for _, t := range state.Tasks {
		if t == nil || t.GetTaskId() == "" || seen[t.GetTaskId()] || !dailyTaskTypeSupported(t.GetType()) || t.GetTotal() < 1 || t.GetCount() < 0 || t.GetCount() > t.GetTotal() || t.GetScore() < 1 || t.GetState() < 0 || t.GetState() > 2 {
			return nil, nil, errors.New(constUtil.DailyTaskConfigError)
		}
		seen[t.GetTaskId()] = true
		total += int64(t.GetScore())
		if total > math.MaxInt32 || (t.GetState() == constUtil.DailyTaskDoing && t.GetCount() >= t.GetTotal()) || (t.GetState() != constUtil.DailyTaskDoing && t.GetCount() < t.GetTotal()) {
			return nil, nil, errors.New(constUtil.DailyTaskConfigError)
		}
		if err := s.dailyTaskReward(t.GetReward(), true); err != nil {
			return nil, nil, err
		}
	}
	seen = map[string]bool{}
	for i, b := range state.Boxes {
		if b == nil || b.GetBoxId() == "" || seen[b.GetBoxId()] || b.GetScore() != int32((i+1)*30) || b.GetState() < 0 || b.GetState() > 2 {
			return nil, nil, errors.New(constUtil.DailyTaskConfigError)
		}
		seen[b.GetBoxId()] = true
		if b.GetState() == constUtil.DailyTaskReceived && dailyTaskScore(state) < b.GetScore() {
			return nil, nil, errors.New(constUtil.DailyTaskConfigError)
		}
		if err := s.dailyTaskReward(b.GetRewards(), false); err != nil {
			return nil, nil, err
		}
	}
	return row, state, nil
}

func saveDailyTaskState(tx *gorm.DB, row *db.DailyTaskState, state *dailyTaskSnapshot) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	row.Snapshot = string(raw)
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error
}

func dailyTaskScore(state *dailyTaskSnapshot) int32 {
	var score int32
	for _, t := range state.Tasks {
		if t.GetState() == constUtil.DailyTaskReceived {
			score += t.GetScore()
		}
	}
	return score
}

func (s *GameService) dailyTasks(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	return s.dailyTasksAt(ctx, id, request, LocalPlayNow(ctx))
}

// dailyTasksAt 在同一事务内发奖和记账，客户端重试及并发请求只能领取一次。
func (s *GameService) dailyTasksAt(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		row, state, err := s.loadDailyTaskState(tx, id, player.GetLevel(), now)
		if err != nil {
			return err
		}
		var reward string
		switch b := request.(type) {
		case *pb.CSTaskList:
			for _, box := range state.Boxes {
				if box.GetState() != constUtil.DailyTaskReceived {
					box.State = proto.Int32(constUtil.DailyTaskDoing)
					if dailyTaskScore(state) >= box.GetScore() {
						box.State = proto.Int32(constUtil.DailyTaskClaimable)
					}
				}
			}
			response = &pb.SCTaskList{TaskList: state.Tasks, BoxList: state.Boxes, Score: proto.Int32(dailyTaskScore(state))}
		case *pb.CSGetTaskReward:
			for _, t := range state.Tasks {
				if t.GetTaskId() == b.GetTaskId() && t.GetState() == constUtil.DailyTaskClaimable {
					reward = t.GetReward()
					t.State = proto.Int32(constUtil.DailyTaskReceived)
					break
				}
			}
			if reward == "" {
				return errors.New(constUtil.DailyTaskUnavailable)
			}
		case *pb.CSGetTaskBoxReward:
			for _, box := range state.Boxes {
				if box.GetBoxId() == b.GetBoxId() && box.GetState() != constUtil.DailyTaskReceived && dailyTaskScore(state) >= box.GetScore() {
					reward = box.GetRewards()
					box.State = proto.Int32(constUtil.DailyTaskReceived)
					break
				}
			}
			if reward == "" {
				return errors.New(constUtil.DailyTaskBoxUnavailable)
			}
		default:
			return errors.New(constUtil.DailyTaskConfigError)
		}
		if reward != "" {
			gain, err := s.chapterBoxGain(player, game, reward)
			if err != nil {
				return err
			}
			if _, ok := request.(*pb.CSGetTaskReward); ok {
				response = &pb.SCGetTaskReward{Get: gain}
			} else {
				response = &pb.SCGetTaskBoxReward{Get: gain}
			}
			if err := saveProfile(tx, &profile, player, game); err != nil {
				return err
			}
		}
		return saveDailyTaskState(tx, row, state)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// recordDailyTaskSave 仅依据成功事务中真实变化计数；只读查询和回执重试不增加进度。
func (s *GameService) recordDailyTaskSave(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState) error {
	return s.recordDailyTaskSaveAt(tx, row, player, game, LocalPlayNow(tx.Statement.Context))
}

func (s *GameService) recordDailyTaskSaveAt(tx *gorm.DB, row *db.Profile, player *pb.Player, game *model.GameState, now time.Time) error {
	request := tx.Statement.Context.Value(constUtil.GameRequestContextKey)
	var kind int32
	switch request.(type) {
	case *pb.CSCopyFightEnd, *pb.CSCopySweep:
		kind = constUtil.DailyTaskCopy
	case *pb.CSWarriorRecruit:
		kind = constUtil.DailyTaskRecruit
	case *pb.CSEquipStrengthen:
		kind = constUtil.DailyTaskEquipStrengthen
	case *pb.CSTreasureStrengthen:
		kind = constUtil.DailyTaskTreasureStrengthen
	case *pb.CSWarriorTraining:
		kind = constUtil.DailyTaskTraining
	case *pb.CSCombine:
		kind = constUtil.DailyTaskCombine
	case *pb.CSEquipRefine:
		kind = constUtil.DailyTaskEquipRefine
	default:
		return nil
	}
	var before model.GameState
	if row.Game == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(*row.Game), &before); err != nil {
		return err
	}
	var count int32
	switch b := request.(type) {
	case *pb.CSCopySweep:
		current := game.CopySweepReceipt
		prior := before.CopySweepReceipt
		if current != nil && current.Response != nil && (prior == nil || current.Timestamp > prior.Timestamp) {
			count = 1
		}
	case *pb.CSCopyFightEnd:
		if before.Run != nil && game.Run != nil && !before.Run.Complete && game.Run.Complete {
			count = 1
		}
	case *pb.CSWarriorRecruit:
		count = int32(max(0, len(game.Warriors)-len(before.Warriors)))
	case *pb.CSCombine:
		count = int32(max(0, len(game.Treasures)-len(before.Treasures)))
	case *pb.CSEquipStrengthen:
		for _, a := range before.Equipment {
			for _, v := range game.Equipment {
				if a.GetGuid() == b.GetGuid() && v.GetGuid() == a.GetGuid() {
					count = max(0, v.GetLevel()-a.GetLevel())
				}
			}
		}
	case *pb.CSEquipRefine:
		for _, a := range before.Equipment {
			for _, v := range game.Equipment {
				if a.GetGuid() == b.GetGuid() && v.GetGuid() == a.GetGuid() && (v.GetRefineLv() != a.GetRefineLv() || v.GetRefineExp() != a.GetRefineExp()) {
					count = 1
				}
			}
		}
	case *pb.CSTreasureStrengthen:
		// 吞噬可能只增加当前经验；按一次成功消耗操作计数。
		for _, a := range before.Treasures {
			for _, v := range game.Treasures {
				if a.GetGuid() == b.GetTargetId() && v.GetGuid() == a.GetGuid() && (v.GetStrengthLevel() != a.GetStrengthLevel() || v.GetCurrentExp() != a.GetCurrentExp()) {
					count = 1
				}
			}
		}
	case *pb.CSWarriorTraining:
		// growthOperation 的回执重试直接返回，不会调用 saveProfile；全零训练也是真实付费事件。
		if b.GetNumType() >= 1 && b.GetNumType() <= 3 {
			count = []int32{1, 5, 10}[b.GetNumType()-1]
		}
	}
	if count <= 0 {
		return nil
	}
	stateRow, state, err := s.loadDailyTaskState(tx, row.Id, player.GetLevel(), now)
	if err != nil {
		return err
	}
	for _, t := range state.Tasks {
		if t.GetType() == kind && t.GetState() == constUtil.DailyTaskDoing {
			t.Count = proto.Int32(int32(min(int64(t.GetTotal()), int64(t.GetCount())+int64(count))))
			if t.GetCount() >= t.GetTotal() {
				t.State = proto.Int32(constUtil.DailyTaskClaimable)
			}
		}
	}
	return saveDailyTaskState(tx, stateRow, state)
}
