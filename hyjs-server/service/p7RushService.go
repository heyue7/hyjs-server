package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// LoadP7Tables 只加载已恢复的精英首领原表，不为缺失的普通楼层补造敌人。
func (s *GameService) LoadP7Tables(root string) error {
	for _, name := range []string{"RushEliteBoss", "BuffInfExtend"} {
		if err := s.loadP7Table(root, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) loadP7Table(root, name string) error {
	f, err := os.Open(filepath.Join(root, "Table"+name+".tsv"))
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}
	if len(rows) < 5 || name == "RushEliteBoss" && len(rows) < 6 {
		return errors.New("精英首领原表缺失")
	}
	var entries []map[string]string
	for _, values := range rows[5:] {
		if len(values) == 0 || values[0] == "" {
			continue
		}
		entry := map[string]string{}
		for i, v := range values {
			if i < len(rows[1]) {
				entry[rows[1][i]] = v
			}
		}
		entries = append(entries, entry)
	}
	s.Tables[name] = entries
	return nil
}

func (s *GameService) InitializeP7(ctx context.Context) error {
	if !s.P1Ready || len(s.Tables["RushEliteBoss"]) == 0 {
		return errors.New("P7需要P1及精英首领原表")
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P7RushPolicy{}, &db.P7RushStageRule{}, &db.P7RushState{}, &db.P7RushProgress{}, &db.P7RushFight{}, &db.P7RushReceipt{}, &db.P7RushScopedPolicy{}, &db.P7RushScopedCalculation{}, &db.P7RushManualPolicy{}, &db.P7RushManualCalculation{}); err != nil {
		return err
	}
	if err := s.InitializeP7MazeTrader(ctx); err != nil {
		return err
	}
	if err := s.InitializeP7SectionRewards(ctx); err != nil {
		return err
	}
	if err := s.InitializeP7RushAddition(ctx); err != nil {
		return err
	}
	if err := s.InitializeP7RushContinuation(ctx); err != nil {
		return err
	}
	if err := s.InitializeP7ReRush(ctx); err != nil {
		return err
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := initializeP7ManualRush(tx); err != nil {
			return err
		}
		if err := initializeP7ScopedRush(tx); err != nil {
			return err
		}
		policy := db.P7RushPolicy{ID: constUtil.P7RushPolicyID, Enabled: false, EliteBuyCost: 50, TrialResetCost: 50, Source: constUtil.P7RushPolicySource}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, row := range s.Tables["RushEliteBoss"] {
			id := row["EliteTaskId"]
			if id == "" || seen[id] || row["ConditionTaskId"] == "" {
				return errors.New("精英首领原条件重复或缺失")
			}
			seen[id] = true
			for _, reward := range []string{row["FirstReward"], row["Reward"]} {
				if ok, err := s.openServerRewardSupported(reward); err != nil || !ok {
					return errors.New("精英首领原奖励无效")
				}
			}
			level, err := p7Integer(s.table("GameConfigure", "StaticName", "RushLimitLevel")["NameValue"], 1, math.MaxInt32)
			if err != nil {
				return err
			}
			cfg := db.P7RushStageRule{ID: "elite:" + id, Kind: constUtil.P7RushElite, TaskID: id, ConditionTaskID: row["ConditionTaskId"], MinLevel: level, MaxRound: 40, FirstReward: row["FirstReward"], Reward: row["Reward"], Source: "原TableRushEliteBoss奖励/ConditionTaskId；IconName为界面图标，不能当作已恢复Boss战斗阵容。原敌人/场景/胜利条件待恢复，默认关闭。"}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
				return err
			}
		}
		return s.seedP7StarActivity(tx, time.Now(), false)
	})
}

func p7Integer(raw string, low, high int32) (int32, error) {
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < int64(low) || n > int64(high) {
		return 0, errors.New("试炼原数值配置无效")
	}
	return int32(n), nil
}

// p7AuthorityAvailable 同时要求原生整场证据、能力hash和P6发布开关，SQL单独启用无法越过门禁。
func p7AuthorityAvailable(tx *gorm.DB) (bool, error) {
	parity := false
	for _, capability := range P6Capabilities() {
		if capability.Name == "native-full-battle-parity" {
			parity = capability.Implemented && capability.NativeCompared
		}
		if capability.Implemented && !capability.NativeCompared {
			return false, nil
		}
	}
	if !parity {
		return false, nil
	}
	var cfg db.P6CombatRule
	if err := tx.First(&cfg, 1).Error; err != nil {
		return false, err
	}
	return cfg.AuthoritativeEnabled && cfg.NativeCapabilityHash == P6CapabilityHash() && cfg.ClientHash == constUtil.P6OriginalClientHash, nil
}

func (s *GameService) p7Policy(tx *gorm.DB) (db.P7RushPolicy, error) {
	var cfg db.P7RushPolicy
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, constUtil.P7RushPolicyID).Error
	if err == nil && (cfg.EliteBuyCost < 1 || cfg.TrialResetCost < 1 || cfg.EliteBuyCost > math.MaxInt32/100 || cfg.TrialResetCost > math.MaxInt32/100) {
		err = errors.New("试炼运营价格无效")
	}
	return cfg, err
}

func p7State(tx *gorm.DB, id int64, now time.Time) (*db.P7RushState, error) {
	state := &db.P7RushState{PlayerID: id, RunID: 1, Floor: 1, Day: p1Day(now)}
	err := tx.Where("player_id=?", id).First(state).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if state.RunID < 1 || state.Floor < 1 || state.TotalStars < 0 || state.RecordStars < state.TotalStars || state.HighFloor < 0 || state.FreeResetUsed < 0 || state.PaidResetUsed < 0 || state.EliteUsed < 0 || state.EliteBought < 0 {
		return nil, errors.New("试炼存档进度无效")
	}
	if err = refreshP7RushCalendar(tx, state, now); err != nil {
		return nil, err
	}
	return state, saveP7RushCalendar(tx, state, now)
}

func (s *GameService) p7VIP(p *pb.Player, key string) (int32, error) {
	return p7Integer(s.table("VipPerks", "vip", strconv.Itoa(int(p.GetVip())))[key], 0, 10000)
}

func (s *GameService) p7Stages(tx *gorm.DB, kind int32) ([]db.P7RushStageRule, error) {
	var rows []db.P7RushStageRule
	err := tx.Where("kind=?", kind).Order("task_id,id").Find(&rows).Error
	return rows, err
}

func (s *GameService) validateP7Stage(cfg db.P7RushStageRule) (*pb.BattleMonster, error) {
	if cfg.ID == "" || cfg.TaskID == "" || cfg.MinLevel < 1 || cfg.Scene == "" || cfg.MaxRound < 1 || cfg.MaxRound > 40 || cfg.WinKey != 0 || cfg.WinValue != 0 || cfg.Kind != 1 && cfg.Kind != 2 || cfg.Kind == 1 && (cfg.Floor < 1 || cfg.Star < 1 || cfg.Star > 3) || cfg.Kind == 2 && cfg.Star != 0 || cfg.Source == "" {
		return nil, errors.New(constUtil.P7RushContentMissing)
	}
	monster := &pb.BattleMonster{}
	// 原客户端构造敌方时直接读取实例怒气；拒绝缺少实例的配置，避免发出必然崩溃的战斗包。
	if proto.Unmarshal(cfg.Enemies, monster) != nil || monster.Inst == nil || len(monster.GetFormation()) < 1 {
		return nil, errors.New(constUtil.P7RushContentMissing)
	}
	for _, reward := range []string{cfg.Reward, cfg.FirstReward} {
		if reward != "" {
			if ok, err := s.openServerRewardSupported(reward); err != nil || !ok {
				return nil, errors.New("试炼奖励尚未恢复")
			}
		}
	}
	return monster, nil
}

func p7HasPassed(tx *gorm.DB, id int64, task string) (bool, error) {
	if task == "" {
		return true, nil
	}
	var count int64
	err := tx.Model(&db.P7RushProgress{}).Where("player_id=? AND kind=? AND task_id=?", id, constUtil.P7RushTrial, task).Count(&count).Error
	return count > 0, err
}

// p7StagePrerequisite 保留原任务号前置，并将原精英数字任务号接到真实连续楼层记录。
// 兼容分支只读取已接受的冻结战斗，不写入03等虚构通关记录或补发奖励。
func (s *GameService) p7StagePrerequisite(tx *gorm.DB, id int64, stage db.P7RushStageRule) (bool, error) {
	passed, err := p7HasPassed(tx, id, stage.ConditionTaskID)
	if err != nil || stage.Kind != constUtil.P7RushElite || passed && stage.Source != constUtil.P7ReRushSource {
		return passed, err
	}
	original := s.table("RushEliteBoss", "EliteTaskId", stage.TaskID)
	if original == nil || original["ConditionTaskId"] != stage.ConditionTaskID {
		return false, nil
	}
	floor, err := p7Integer(stage.ConditionTaskID, 1, math.MaxInt32)
	if err != nil {
		return false, nil
	}
	var state db.P7RushState
	if err = tx.First(&state, "player_id=?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if state.HighFloor < floor {
		return false, nil
	}
	if stage.Source == constUtil.P7ReRushSource {
		wins, err := p7ReRushRunWins(tx, &state, 1, floor)
		if err != nil {
			return false, err
		}
		if len(wins) == int(floor) {
			return true, nil
		}
	}
	var fights []db.P7RushFight
	if err = tx.Where("player_id=? AND kind=? AND status=? AND diagnostic=? AND run_id > 0", id, constUtil.P7RushTrial, constUtil.P7RushSettled, false).Find(&fights).Error; err != nil {
		return false, err
	}
	completed := map[int64]map[int32]bool{}
	for i := range fights {
		fight := &fights[i]
		var rule db.P7RushStageRule
		if p6Digest(fight.Rule) != fight.RuleHash || json.Unmarshal(fight.Rule, &rule) != nil || rule.Kind != constUtil.P7RushTrial || rule.TaskID != fight.TaskID || rule.Floor < 1 || rule.Floor > floor || rule.Star < 1 || rule.Star > 3 {
			continue
		}
		if rule.Source != constUtil.P7SectionStageSource && rule.Source != constUtil.P7RushContinuationSource && rule.Source != constUtil.P7ReRushSource {
			continue
		}
		end := &pb.CSRushFightEnd{}
		result := &pb.SCRushFightEnd{}
		if proto.Unmarshal(fight.EndRequest, end) != nil || proto.Unmarshal(fight.Result, result) != nil || end.GetFightId() != fight.ID || end.GetResult() != 1 || result.GetResult() != 1 || result.GetStar() != rule.Star {
			continue
		}
		// 复用冻结来源校验，单独插入进度、战斗结果或诊断实例均不能满足此桥。
		outcome, scoped, readErr := p7ReadScopedRush(tx, fight, end)
		if readErr != nil {
			return false, readErr
		}
		if !scoped || outcome == nil || !outcome.Authoritative || outcome.Winner != 0 {
			continue
		}
		if completed[fight.RunID] == nil {
			completed[fight.RunID] = map[int32]bool{}
		}
		completed[fight.RunID][rule.Floor] = true
	}
	// 同一自然轮次必须含1至目标层的完整链，其他轮的散落高层不拼接授权。
	for _, run := range completed {
		if len(run) != int(floor) {
			continue
		}
		return true, nil
	}
	return false, nil
}

// P7RushQuery 返回原精英目录与真实VIP预算；普通楼层缺失时保持显式门禁。
func (s *GameService) P7RushQuery(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var result proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		cfg, err := s.p7Policy(tx)
		if err != nil {
			return err
		}
		state, err := p7State(tx, id, now)
		if err != nil {
			return err
		}
		authority, err := p7AuthorityAvailable(tx)
		if err != nil {
			return err
		}
		switch request.(type) {
		case *pb.CSGetRushEliteBossInfo:
			free, err := s.p7VIP(p, "numPerks24")
			if err != nil {
				return err
			}
			paid, err := s.p7VIP(p, "numPerks25")
			if err != nil {
				return err
			}
			if state.EliteBought > paid || state.EliteUsed > free+state.EliteBought {
				return errors.New("精英日次数存档无效")
			}
			response := &pb.SCGetRushEliteBossInfo{LeftTime: proto.Int32(free + state.EliteBought - state.EliteUsed), TotalTime: proto.Int32(free + state.EliteBought), DiamondLeftTime: proto.Int32(paid - state.EliteBought), DiamondLeftTimeCost: proto.Int32(cfg.EliteBuyCost)}
			stages, err := s.p7Stages(tx, constUtil.P7RushElite)
			if err != nil {
				return err
			}
			for _, stage := range stages {
				first, err := p1RewardResources(stage.FirstReward)
				if err != nil {
					return err
				}
				general, err := p1RewardResources(stage.Reward)
				if err != nil {
					return err
				}
				passed, err := s.p7StagePrerequisite(tx, id, stage)
				if err != nil {
					return err
				}
				open := int32(0)
				scoped := false
				if cfg.Enabled && !authority && stage.Enabled && passed && p.GetLevel() >= stage.MinLevel {
					scoped, err = s.p7ScopedStageAvailable(ctx, tx, p, g, stage, now)
					if err != nil {
						return err
					}
				}
				if cfg.Enabled && (authority || scoped) && stage.Enabled && passed && p.GetLevel() >= stage.MinLevel {
					if _, err = s.validateP7Stage(stage); err == nil {
						open = 1
					}
				}
				var wins int64
				if err = tx.Model(&db.P7RushProgress{}).Where("player_id=? AND kind=? AND task_id=?", id, constUtil.P7RushElite, stage.TaskID).Count(&wins).Error; err != nil {
					return err
				}
				response.BossInfoList = append(response.BossInfoList, &pb.RushEliteBossInfo{EliteTaskId: proto.String(stage.TaskID), IsOpen: proto.Int32(open), FirstRewardList: first, GeneralRewardList: general, IsSuccess: proto.Int32(int32(min(wins, 1)))})
			}
			result = response
		case *pb.CSGetRushInfo, *pb.CSRushCurrentInfo, *pb.CSRushGetTopSweepInfo:
			_, pendingStage, pending, e := s.p7PendingRushContinuation(tx, state)
			if e != nil {
				return e
			}
			if pending {
				switch request.(type) {
				case *pb.CSGetRushInfo:
					result, e = s.p7PendingRushContinuationInfo(tx, p, state, cfg, now, pendingStage.Source)
				case *pb.CSRushCurrentInfo:
					projected, pe := p7RushContinuationTaskProjection(*pendingStage)
					if pe != nil {
						return pe
					}
					result = &pb.SCRushCurrentInfo{TaskInfoList: []*pb.RushTaskInfo{projected}}
				case *pb.CSRushGetTopSweepInfo:
					result = &pb.SCRushGetTopSweepInfo{TopFloor: proto.Int32(state.HighFloor)}
				}
				return e
			}
			stages, err := s.p7Stages(tx, constUtil.P7RushTrial)
			if err != nil {
				return err
			}
			var current []db.P7RushStageRule
			legacyFirst, legacyErr := s.p7ReRushLegacyFirstSection(tx, state)
			if legacyErr != nil {
				return legacyErr
			}
			for _, stage := range stages {
				if legacyFirst {
					allowed, err := s.p7ReRushLegacyStage(tx, state, stage)
					if err != nil {
						return err
					}
					stage.Enabled = allowed
				}
				if stage.Enabled && stage.Floor == state.Floor {
					if _, err := s.validateP7Stage(stage); err != nil {
						return err
					}
					allowed, e := s.p7RushContinuationStageAvailable(tx, state, stage)
					if e != nil {
						return e
					}
					if !allowed {
						continue
					}
					current = append(current, stage)
				}
			}
			if len(current) == 0 {
				reEnabled, reErr := p7ReRushEnabled(tx)
				if reErr != nil {
					return reErr
				}
				if reEnabled && state.Floor > constUtil.P7ReRushLastFloor {
					switch request.(type) {
					case *pb.CSGetRushInfo:
						result, reErr = s.p7ReRushCompletedInfo(tx, p, state, cfg, now)
					case *pb.CSRushCurrentInfo:
						result = &pb.SCRushCurrentInfo{}
					case *pb.CSRushGetTopSweepInfo:
						result = &pb.SCRushGetTopSweepInfo{TopFloor: proto.Int32(state.HighFloor)}
					}
					return reErr
				}
				if _, ok := request.(*pb.CSGetRushInfo); ok {
					earned, handled, e := s.p7EarnedSectionOnlyInfo(tx, p, state, cfg, now)
					if e != nil {
						return e
					}
					if handled {
						result = earned
						return nil
					}
					earned, handled, e = s.p7EarnedRushAdditionOnlyInfo(tx, p, state, cfg, now)
					if e != nil {
						return e
					}
					if handled {
						result = earned
						return nil
					}
				}
				return errors.New(constUtil.P7RushContentMissing)
			}
			floorScene := current[0].Scene
			if current[0].Source == constUtil.P7ReRushSource {
				floorScene = p7ReRushNavigation(state.Floor)
			}
			if !authority && cfg.Enabled {
				scopedEnabled, err := p7ScopedRushAvailable(tx)
				if err != nil {
					return err
				}
				if scopedEnabled {
					available, err := s.p7ScopedStageAvailable(ctx, tx, p, g, current[0], now)
					if err != nil {
						return err
					}
					if !available {
						return errors.New(constUtil.P7RushGateClosed)
					}
					// Original RoomRush navigation and actual battle scenery have
					// separate fields/default assets. Keep stage.Scene for Start.
					if current[0].Source != constUtil.P7ReRushSource {
						floorScene = constUtil.P7OriginalScopedFloorScene
					}
				}
			}
			free, err := s.p7VIP(p, "numPerk2")
			if err != nil {
				return err
			}
			paid, err := s.p7VIP(p, "numPerk23")
			if err != nil {
				return err
			}
			if _, ok := request.(*pb.CSGetRushInfo); ok {
				mustReset, mazeValid, err := s.p7MazeTraderFlags(tx, state, now)
				if err != nil {
					return err
				}
				rewardReady, e := s.p7SectionRewardFlag(tx, state, now)
				if e != nil {
					return e
				}
				additionReady, e := s.p7RushAdditionFlag(tx, state, now)
				if e != nil {
					return e
				}
				if current[0].Source == constUtil.P7ReRushSource {
					rewardRe, additionRe, reErr := p7ReRushSectionFlags(tx, state)
					e = reErr
					rewardReady, additionReady = max(rewardReady, rewardRe), max(additionReady, additionRe)
					if e != nil {
						return e
					}
					valid, err := p7ReRushTraderValid(tx, state)
					if err != nil {
						return err
					}
					if valid {
						mazeValid = 1
					}
				}
				result = &pb.SCGetRushInfo{IsAdditionNeedGet: proto.Int32(additionReady), IsRewardNeedGet: proto.Int32(rewardReady), IsMustReset: proto.Int32(mustReset), IsMazeTraderValid: proto.Int32(mazeValid), CurFloor: proto.Int32(state.Floor), CurTotalStar: proto.Int32(state.TotalStars), ResetLeftTime: proto.Int32(max(0, free-state.FreeResetUsed)), DiamondRestLeftTime: proto.Int32(max(0, paid-state.PaidResetUsed)), DiamondRestCostNum: proto.Int32(cfg.TrialResetCost), HighRecordFloor: proto.Int32(state.HighFloor), RecordTotalStar: proto.Int32(state.RecordStars), CurFloorSceneName: proto.String(floorScene), WinKey: proto.Int32(current[0].WinKey), WinValue: proto.Int32(current[0].WinValue)}
			} else if _, ok := request.(*pb.CSRushGetTopSweepInfo); ok {
				result = &pb.SCRushGetTopSweepInfo{TopFloor: proto.Int32(state.HighFloor)}
			} else {
				response := &pb.SCRushCurrentInfo{}
				for _, stage := range current {
					if stage.Source == constUtil.P7ReRushSource {
						response.TaskInfoList = append(response.TaskInfoList, p7ReRushTaskProjection(stage))
						continue
					}
					if stage.Source == constUtil.P7RushContinuationSource {
						projected, err := p7RushContinuationTaskProjection(stage)
						if err != nil {
							return err
						}
						response.TaskInfoList = append(response.TaskInfoList, projected)
						continue
					}
					if stage.Source == constUtil.P7SectionStageSource {
						projected, err := p7SectionTaskProjection(stage)
						if err != nil {
							return err
						}
						response.TaskInfoList = append(response.TaskInfoList, projected)
						continue
					}
					response.TaskInfoList = append(response.TaskInfoList, &pb.RushTaskInfo{TaskId: proto.String(stage.TaskID), Star: proto.Int32(stage.Star), FightScore: proto.Float64(stage.RecommendedPower), IsPassed: proto.Bool(false)})
				}
				result = response
			}
		default:
			return errors.New("试炼查询尚未恢复")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}

func p7ReadReceipt(tx *gorm.DB, id int64, domain string, stamp int64, request, response proto.Message) (bool, error) {
	if stamp <= 0 {
		return false, errors.New("试炼请求时间戳无效")
	}
	var row db.P7RushReceipt
	err := tx.Where("player_id=? AND domain=? AND timestamp=?", id, domain, stamp).First(&row).Error
	raw, e := proto.Marshal(request)
	if e != nil {
		return false, e
	}
	if err == nil {
		if !bytes.Equal(raw, row.Request) {
			return false, errors.New("试炼回执内容冲突")
		}
		return true, proto.Unmarshal(row.Response, response)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	var newer int64
	if err = tx.Model(&db.P7RushReceipt{}).Where("player_id=? AND domain=? AND timestamp>?", id, domain, stamp).Count(&newer).Error; err != nil {
		return false, err
	}
	if newer > 0 {
		return false, errors.New("试炼请求已过期")
	}
	return false, nil
}

func p7SaveReceipt(tx *gorm.DB, id int64, domain string, stamp int64, request, response proto.Message) error {
	raw, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	reply, err := proto.Marshal(response)
	if err != nil {
		return err
	}
	return tx.Create(&db.P7RushReceipt{PlayerID: id, Domain: domain, Timestamp: stamp, Request: raw, Response: reply}).Error
}

// PrepareP7DiagnosticFight 仅供隔离验证冻结/整场计算，诊断实例不占用业务次数且永久不能领奖。
func (s *GameService) PrepareP7DiagnosticFight(ctx context.Context, id int64, request *pb.CSRushFightStart, stamp int64) (*pb.SCRushFightStart, error) {
	return s.p7Start(ctx, id, request, stamp, true)
}

func (s *GameService) p7Start(ctx context.Context, id int64, request *pb.CSRushFightStart, stamp int64, diagnostic bool) (*pb.SCRushFightStart, error) {
	result := &pb.SCRushFightStart{}
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		domain := "start"
		if diagnostic {
			domain = "diagnostic"
		}
		if found, err := p7ReadReceipt(tx, id, domain, stamp, request, result); err != nil || found {
			if err != nil || !found || diagnostic {
				return err
			}
			var accepted db.P7RushFight
			if err = tx.First(&accepted, "id=? AND player_id=?", result.GetFightId(), id).Error; err != nil {
				return err
			}
			_, _, err = p7ReadScopedRush(tx, &accepted, nil)
			return err
		}
		if !diagnostic {
			// 恢复已创建的本地后继局时先校验冻结权限，当前开关只控制新建。
			var existing db.P7RushState
			e := tx.First(&existing, "player_id=?", id).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if e == nil && existing.ActiveFight != "" {
				pendingStart, pendingStage, owned, pe := s.p7PendingRushContinuation(tx, &existing)
				if pe != nil {
					return pe
				}
				if owned && request.GetRushType() == constUtil.P7RushTrial && request.GetTaskId() == pendingStage.TaskID && request.GetStar() == pendingStage.Star {
					result = pendingStart
					return p7SaveReceipt(tx, id, domain, stamp, request, result)
				}
			}
		}
		policy, err := s.p7Policy(tx)
		if err != nil {
			return err
		}
		useScoped := false
		if !diagnostic {
			available, err := p7AuthorityAvailable(tx)
			if err != nil {
				return err
			}
			if !available {
				useScoped, err = p7ScopedRushAvailable(tx)
				if err != nil {
					return err
				}
			}
			if !policy.Enabled || !(available || useScoped) {
				return errors.New(constUtil.P7RushGateClosed)
			}
		}
		state, err := p7State(tx, id, time.Now())
		if err != nil {
			return err
		}
		if !diagnostic && state.ActiveFight != "" {
			return errors.New("请先完成当前试炼战斗")
		}
		var stages []db.P7RushStageRule
		if err = tx.Where("kind=? AND task_id=? AND star=?", request.GetRushType(), request.GetTaskId(), request.GetStar()).Find(&stages).Error; err != nil {
			return err
		}
		if len(stages) != 1 {
			return errors.New(constUtil.P7RushContentMissing)
		}
		cfg := stages[0]
		if !cfg.Enabled {
			legacy, err := s.p7ReRushLegacyStage(tx, state, cfg)
			if err != nil {
				return err
			}
			if !legacy {
				return errors.New(constUtil.P7RushContentMissing)
			}
			cfg.Enabled = true
		}
		if !diagnostic {
			allowed, e := s.p7RushContinuationStageAvailable(tx, state, cfg)
			if e != nil {
				return e
			}
			if !allowed {
				return errors.New("本地试炼后继目录未开放或本轮来源不足")
			}
		}
		monster, err := s.validateP7Stage(cfg)
		if err != nil {
			return err
		}
		if p.GetLevel() < cfg.MinLevel {
			return errors.New("试炼等级条件未达成")
		}
		if !diagnostic {
			if cfg.Kind == constUtil.P7RushTrial {
				failed, err := p7TrialMustReset(tx, state)
				if err != nil {
					return err
				}
				if failed {
					return errors.New("本轮试炼失败，请先重置")
				}
			}
			if cfg.Kind == constUtil.P7RushTrial && cfg.Floor != state.Floor {
				return errors.New("试炼楼层顺序不匹配")
			}
			if cfg.Kind == constUtil.P7RushTrial && cfg.Source == constUtil.P7ReRushSource {
				legacy, err := s.p7ReRushLegacyFirstSection(tx, state)
				if err != nil {
					return err
				}
				if legacy {
					return errors.New("请先完成本轮已冻结的原试炼首组")
				}
				reward, addition, err := p7ReRushSectionFlags(tx, state)
				if err != nil {
					return err
				}
				oldReward, err := s.p7SectionRewardFlag(tx, state, LocalPlayNow(ctx))
				if err != nil {
					return err
				}
				oldAddition, err := s.p7RushAdditionFlag(tx, state, LocalPlayNow(ctx))
				if err != nil {
					return err
				}
				reward, addition = max(reward, oldReward), max(addition, oldAddition)
				if reward != 0 || addition != 0 {
					return errors.New("请先领取本段试炼奖励并选择加成")
				}
			}
			passed, err := s.p7StagePrerequisite(tx, id, cfg)
			if err != nil {
				return err
			}
			if !passed {
				return errors.New("试炼前置任务未通过")
			}
			if cfg.Kind == constUtil.P7RushElite {
				free, err := s.p7VIP(p, "numPerks24")
				if err != nil {
					return err
				}
				if state.EliteUsed >= free+state.EliteBought {
					return errors.New("精英挑战次数不足")
				}
			}
		}
		if g.Run != nil && !g.Run.Complete {
			return errors.New("请先离开主线副本")
		}
		if err = s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
			return err
		}
		if useScoped {
			// Include the current membership-bound Prayer/Tech fold in the same paid transaction.
			if err = s.PrepareP5GuildPrayerProperties(ctx, tx, p, g, time.Now()); err != nil {
				return err
			}
		}
		attacker, err := s.battlePlayer(p, g)
		if err != nil {
			return err
		}
		if !diagnostic && (cfg.Kind == constUtil.P7RushTrial || cfg.Source == constUtil.P7ReRushSource) {
			if cfg.Source == constUtil.P7ReRushSource {
				attacker, err = s.p7ApplyReRushBonus(tx, state, attacker)
			} else {
				attacker, err = s.p7ApplyRushAddition(tx, state, attacker)
			}
			if err != nil {
				return err
			}
		}
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return err
		}
		fightID := "p7-" + hex.EncodeToString(random[:])
		seed := int64(0)
		for _, b := range random[:6] {
			seed = seed<<8 | int64(b)
		}
		// Natural047/025的独立原机范围使用非负int32种子；新局保留
		// 新鲜随机性并遵守该输入域，其他既有族保持原48位种子。
		if len(attacker.Formation) == 2 && attacker.Formation[0].GetId() == "hero_047" && attacker.Formation[1].GetId() == "hero_025" {
			seed &= math.MaxInt32
		}
		// 原试炼回调 SetData(start,true) 为9，精英 SetData(start,false) 为10。
		battleType := constUtil.P7RushTrialBattleType
		if cfg.Kind == constUtil.P7RushElite {
			battleType = constUtil.P7RushEliteBattleType
		}
		input := model.FrozenBattleInput{Attacker: attacker, Monster: monster, Seed: seed, MaxRound: cfg.MaxRound, BattleType: battleType}
		replayID, err := s.CaptureBattleReplay(ctx, tx, id, constUtil.P7RushReplaySource, fightID, input)
		if err != nil {
			return err
		}
		result = &pb.SCRushFightStart{FightId: proto.String(fightID), RandSeed: proto.Int64(seed), BattlePlayer: attacker, BattleMonsterList: []*pb.BattleMonster{monster}, FightScene: proto.String(cfg.Scene), MaxRound: proto.Int32(cfg.MaxRound), WinKey: proto.Int32(cfg.WinKey), WinValue: proto.Int32(cfg.WinValue), RushType: proto.Int32(cfg.Kind)}
		var outcome *model.CombatOutcome
		if useScoped {
			outcome, err = s.p7ComputeScopedRush(ctx, tx, id, replayID, result, time.Now())
		} else {
			// Existing diagnostic/global behavior retains its original driver and gate.
			outcome, err = s.ComputeBattleReplay(ctx, tx, id, constUtil.P7RushReplaySource, fightID, nil)
		}
		if err != nil {
			return err
		}
		if !diagnostic && !outcome.Authoritative {
			return errors.New(constUtil.P7RushGateClosed)
		}
		start, err := proto.Marshal(result)
		if err != nil {
			return err
		}
		rule, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		fight := db.P7RushFight{ID: fightID, PlayerID: id, Kind: cfg.Kind, TaskID: cfg.TaskID, Status: constUtil.P7RushComputed, Diagnostic: diagnostic, Day: state.Day, RunID: state.RunID, Rule: rule, RuleHash: p6Digest(rule), Start: start, StartHash: p6Digest(start), ReplayID: replayID}
		if err = tx.Create(&fight).Error; err != nil {
			return err
		}
		if !diagnostic {
			if err = s.p7CaptureSectionOffer(tx, &fight); err != nil {
				return err
			}
			if err = s.p7CaptureRushAdditionOffer(tx, &fight); err != nil {
				return err
			}
			state.ActiveFight = fightID
			if cfg.Kind == constUtil.P7RushElite {
				state.EliteUsed++
			}
			if err = saveP7RushCalendar(tx, state, time.Now()); err != nil {
				return err
			}
		}
		return p7SaveReceipt(tx, id, domain, stamp, request, result)
	})
	return result, err
}

// ComputeP7Diagnostic 从原始冻结记录重演已计算战斗，返回真实胜败并保留不可领奖身份。
func (s *GameService) ComputeP7Diagnostic(ctx context.Context, id int64, fightID string, operate *pb.BattleOperate) (*model.CombatOutcome, error) {
	var result *model.CombatOutcome
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		var fight db.P7RushFight
		if err := tx.Where("id=? AND player_id=? AND diagnostic=?", fightID, id, true).First(&fight).Error; err != nil {
			return err
		}
		if p6Digest(fight.Rule) != fight.RuleHash || p6Digest(fight.Start) != fight.StartHash {
			return errors.New("试炼冻结记录损坏")
		}
		var err error
		result, err = s.ComputeBattleReplay(ctx, tx, id, constUtil.P7RushReplaySource, fightID, operate)
		return err
	})
	return result, err
}

func (s *GameService) p7End(ctx context.Context, id int64, request *pb.CSRushFightEnd) ([]proto.Message, error) {
	var result *pb.SCRushFightEnd
	var synchronizedPlayer *pb.Player
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var fight db.P7RushFight
		if err := tx.Where("id=? AND player_id=?", request.GetFightId(), id).First(&fight).Error; err != nil {
			return err
		}
		raw, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		if fight.Status == constUtil.P7RushSettled {
			if !bytes.Equal(raw, fight.EndRequest) {
				return errors.New("试炼结算回执内容冲突")
			}
			result = &pb.SCRushFightEnd{}
			if err = proto.Unmarshal(fight.Result, result); err != nil {
				return err
			}
			_, scoped, err := p7ReadScopedRush(tx, &fight, request)
			if err != nil {
				return err
			}
			if scoped {
				if err = s.p7ValidateScopedAward(tx, id, g, result); err != nil {
					return err
				}
				synchronizedPlayer = proto.Clone(p).(*pb.Player)
				return nil
			}
			return nil
		}
		if fight.Diagnostic {
			return errors.New(constUtil.P7RushGateClosed)
		}
		scopedOutcome, scoped, err := p7ReadScopedRush(tx, &fight, request)
		if err != nil {
			return err
		}
		if !scoped {
			available, err := p7AuthorityAvailable(tx)
			if err != nil {
				return err
			}
			if !available {
				return errors.New(constUtil.P7RushGateClosed)
			}
		}
		if p6Digest(fight.Rule) != fight.RuleHash || p6Digest(fight.Start) != fight.StartHash {
			return errors.New("试炼冻结记录损坏")
		}
		var cfg db.P7RushStageRule
		if json.Unmarshal(fight.Rule, &cfg) != nil {
			return errors.New("试炼任务快照损坏")
		}
		if request.GetOptRecord() == nil {
			return errors.New("试炼结算缺少原操作记录")
		}
		outcome := scopedOutcome
		if !scoped {
			outcome, err = s.ComputeBattleReplay(ctx, tx, id, constUtil.P7RushReplaySource, fight.ID, request.GetOptRecord())
		}
		if err != nil {
			return err
		}
		if !outcome.Authoritative {
			return errors.New(constUtil.P7RushGateClosed)
		}
		won := int32(0)
		if outcome.Winner == 0 {
			won = 1
		}
		if request.GetResult() != won {
			return errors.New("客户端试炼结果与冻结计算不符")
		}
		state, err := p7State(tx, id, time.Now())
		if err != nil {
			return err
		}
		if state.ActiveFight != fight.ID || state.RunID != fight.RunID {
			return errors.New("试炼实例已结束或被重置")
		}
		gain := &pb.ResultObject{}
		star := int32(0)
		if won == 1 {
			var progress db.P7RushProgress
			err = tx.Where("player_id=? AND kind=? AND task_id=?", id, cfg.Kind, cfg.TaskID).First(&progress).Error
			first := errors.Is(err, gorm.ErrRecordNotFound)
			if err != nil && !first {
				return err
			}
			reward := cfg.Reward
			if first && cfg.FirstReward != "" {
				if reward != "" {
					reward += ";"
				}
				reward += cfg.FirstReward
			}
			if reward != "" {
				gain, err = s.GrantFixedReward(p, g, reward)
				if err != nil {
					return err
				}
			}
			if cfg.Kind == constUtil.P7RushTrial {
				star = cfg.Star
				if state.TotalStars > math.MaxInt32-star {
					return errors.New("试炼星数溢出")
				}
				state.TotalStars += star
				state.RecordStars = max(state.RecordStars, state.TotalStars)
				state.HighFloor = max(state.HighFloor, cfg.Floor)
				state.Floor = cfg.Floor + 1
				if err = s.recordP7AcceptedStarActivity(tx, &fight, cfg, state); err != nil {
					return err
				}
			}
			if first {
				progress = db.P7RushProgress{PlayerID: id, Kind: cfg.Kind, TaskID: cfg.TaskID, Star: star, FirstFight: fight.ID}
				if err = tx.Create(&progress).Error; err != nil {
					return err
				}
			}
		}
		state.ActiveFight = ""
		if err = saveP7RushCalendar(tx, state, time.Now()); err != nil {
			return err
		}
		result = &pb.SCRushFightEnd{Result: proto.Int32(won), Star: proto.Int32(star), ResultObj: gain}
		fight.Result, err = proto.Marshal(result)
		if err != nil {
			return err
		}
		fight.EndRequest = raw
		fight.Status = constUtil.P7RushSettled
		if scoped {
			synchronizedPlayer = proto.Clone(p).(*pb.Player)
		}
		if err = tx.Save(&fight).Error; err != nil {
			return err
		}
		return s.p7FreezeMazeTrader(tx, &fight, time.Now())
	})
	if err != nil {
		return nil, err
	}
	if synchronizedPlayer != nil {
		return []proto.Message{result, &pb.SCPlayer{Player: synchronizedPlayer}}, nil
	}
	return []proto.Message{result}, nil
}

// P7RushMutation 保留已实现的原次数/重置原子路径；权威与内容门禁关闭时不留下部分消费。
func (s *GameService) P7RushMutation(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, error) {
	if result, handled, err := s.p7ReRushMutation(ctx, id, request, stamp); handled || err != nil {
		return result, err
	}
	switch request.(type) {
	case *pb.CSRushAddition, *pb.CSRushAdditionView, *pb.CSRushGetSectionAddition:
		return s.P7RushAddition(ctx, id, request, stamp, time.Now())
	case *pb.CSRushRewardPreview, *pb.CSRushGetSectionReward:
		return s.P7SectionReward(ctx, id, request, stamp, time.Now())
	case *pb.CSRushMazeTrader, *pb.CSRushMazeTraderBuy:
		return s.P7MazeTrader(ctx, id, request, stamp, time.Now())
	}
	if b, ok := request.(*pb.CSRushFightStart); ok {
		r, e := s.p7Start(ctx, id, b, stamp, false)
		if e != nil {
			return nil, e
		}
		return []proto.Message{r}, nil
	}
	if b, ok := request.(*pb.CSRushFightEnd); ok {
		return s.p7End(ctx, id, b)
	}
	var result proto.Message
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var domain string
		switch request.(type) {
		case *pb.CSRushEliteBossBuyTime:
			domain = "elite-buy"
			result = &pb.SCRushEliteBossBuyTime{}
		case *pb.CSRushResetChallengeCount:
			domain = "trial-reset"
			result = &pb.SCRushResetChallengeCount{}
		default:
			return errors.New("试炼扫荡/加成/迷宫商人尚未恢复原规则")
		}
		if found, err := p7ReadReceipt(tx, id, domain, stamp, request, result); err != nil || found {
			return err
		}
		policy, err := s.p7Policy(tx)
		if err != nil {
			return err
		}
		available, err := p7AuthorityAvailable(tx)
		if err != nil {
			return err
		}
		state, err := p7State(tx, id, time.Now())
		if err != nil {
			return err
		}
		if state.ActiveFight != "" {
			return errors.New("当前试炼战斗结束前不能购买或重置")
		}
		failed, err := p7TrialMustReset(tx, state)
		if err != nil {
			return err
		}
		completedSection := false
		if domain == "trial-reset" && !available && !failed {
			completedSection, err = s.p7CompletedLocalSectionResetAllowed(tx, state)
			if err != nil {
				return err
			}
		}
		reEnabled, err := p7ReRushEnabled(tx)
		if err != nil {
			return err
		}
		if !policy.Enabled || (!available && !reEnabled && !(domain == "trial-reset" && (failed || completedSection))) {
			return errors.New(constUtil.P7RushGateClosed)
		}
		var cost []*pb.BaseResource
		if domain == "elite-buy" {
			paid, err := s.p7VIP(p, "numPerks25")
			if err != nil {
				return err
			}
			if state.EliteBought >= paid {
				return errors.New("今日精英付费次数已用完")
			}
			cost = []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(policy.EliteBuyCost)}}
			state.EliteBought++
		} else {
			if state.Floor <= 1 && state.TotalStars == 0 && !failed {
				return errors.New("尚无可重置的试炼进度")
			}
			free, err := s.p7VIP(p, "numPerk2")
			if err != nil {
				return err
			}
			paid, err := s.p7VIP(p, "numPerk23")
			if err != nil {
				return err
			}
			if state.FreeResetUsed < free {
				state.FreeResetUsed++
			} else if state.PaidResetUsed < paid {
				state.PaidResetUsed++
				cost = []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(policy.TrialResetCost)}}
			} else {
				return errors.New("今日试炼重置次数已用完")
			}
			if state.RunID == math.MaxInt64 {
				return errors.New("试炼轮次溢出")
			}
			state.RunID++
			state.Floor = 1
			state.TotalStars = 0
		}
		if len(cost) > 0 {
			if err := deductPoolCost(p, g, cost[0]); err != nil {
				return err
			}
			if err := s.RecordP1Cost(tx, id, fmt.Sprintf("p7:%s:%d", domain, stamp), cost, LocalPlayNow(ctx)); err != nil {
				return err
			}
		}
		if domain == "elite-buy" {
			result = &pb.SCRushEliteBossBuyTime{Cost: cost}
		} else {
			result = &pb.SCRushResetChallengeCount{Cost: cost}
		}
		if err = saveP7RushCalendar(tx, state, time.Now()); err != nil {
			return err
		}
		return p7SaveReceipt(tx, id, domain, stamp, request, result)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}

func (s *GameService) seedP7StarActivity(tx *gorm.DB, now time.Time, enabled bool) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	cfg := db.PeriodInstance{ID: "aincradStars_p4_" + start.Format("200601"), Enabled: enabled, Title: "试炼星数", Kind: constUtil.P7RushStarEvent, Mode: "single", Goal: 3, Rewards: "point,100", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.AddDate(0, 0, 7).UnixMilli(), Source: "本地首档3星/100积分；只接已通过P6完整原生对照的服务端权威试炼结算，记录本轮真实累计星数峰值。诊断计算、客户端result、查询和伪造SQL开关不产生星数。原活动档由P15继续恢复。"}
	if err := s.validateP1Period(cfg); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
		return err
	}
	if enabled {
		return tx.Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Update("enabled", true).Error
	}
	return nil
}

// DispatchP7 只接恢复中的Rush协议，未恢复扫荡/商人/加成明确拒绝，防止空成功回包。
func (s *GameService) DispatchP7(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSGetRushEliteBossInfo, *pb.CSGetRushInfo, *pb.CSRushCurrentInfo, *pb.CSRushGetTopSweepInfo:
		r, e := s.P7RushQuery(ctx, id, request, time.Now())
		return r, true, e
	case *pb.CSRushFightStart, *pb.CSRushFightEnd, *pb.CSRushEliteBossBuyTime, *pb.CSRushResetChallengeCount, *pb.CSRushAddition, *pb.CSRushAdditionView, *pb.CSRush3Hit, *pb.CSRushToTop, *pb.CSRushMazeTrader, *pb.CSRushMazeTraderBuy, *pb.CSRushRewardPreview, *pb.CSRushGetSectionReward, *pb.CSRushGetSectionAddition:
		r, e := s.P7RushMutation(ctx, id, request, stamp)
		return r, true, e
	}
	return nil, false, nil
}
