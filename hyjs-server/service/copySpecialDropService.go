package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
)

type copySpecialEntry struct {
	Reward        string   `json:"reward"`
	OriginalTuple [3]int64 `json:"originalTuple"`
}
type copySpecialInput struct {
	Enabled bool               `json:"enabled"`
	Cycle   int64              `json:"cycle"`
	TaskID  string             `json:"taskId"`
	Source  string             `json:"source"`
	Entries []copySpecialEntry `json:"entries"`
}

func (s *GameService) initializeCopySpecialDrops(tx *gorm.DB) error {
	// 原特殊概率语义未恢复，不再默认启用每四胜固定赠送。
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.CopySpecialRule{ID: constUtil.CopySpecialRuleID, Enabled: false, Cycle: 4, Source: constUtil.CopySpecialSource}).Error
}
func (s *GameService) copySpecialInput(tx *gorm.DB, task map[string]string) (*copySpecialInput, error) {
	if strings.TrimSpace(task["SpecialOdds"]) == "" {
		return nil, nil
	}
	if !tx.Migrator().HasTable(&db.CopySpecialRule{}) {
		return nil, nil
	}
	var cfg db.CopySpecialRule
	if e := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&cfg, "id=?", constUtil.CopySpecialRuleID).Error; e != nil {
		return nil, e
	}
	if cfg.Cycle < 1 || cfg.Cycle > 10000 || strings.TrimSpace(cfg.Source) == "" {
		return nil, errors.New(constUtil.CopySpecialConfigurationError)
	}
	input := &copySpecialInput{Enabled: cfg.Enabled, Cycle: cfg.Cycle, TaskID: task["TaskId"], Source: cfg.Source}
	if !cfg.Enabled {
		return input, nil
	}
	seen := map[string]bool{}
	for _, part := range strings.Split(task["SpecialOdds"], "|") {
		if part == "" {
			continue
		}
		pair := strings.Split(part, ":")
		if len(pair) != 2 || seen[pair[0]] {
			return nil, errors.New(constUtil.CopySpecialConfigurationError)
		}
		seen[pair[0]] = true
		supported, e := s.openServerRewardSupported(pair[0])
		if e != nil || !supported {
			return nil, errors.New(constUtil.CopySpecialConfigurationError)
		}
		values := strings.Split(pair[1], ",")
		if len(values) != 3 {
			return nil, errors.New(constUtil.CopySpecialConfigurationError)
		}
		entry := copySpecialEntry{Reward: pair[0]}
		for i, v := range values {
			n, e := strconv.ParseInt(v, 10, 32)
			if e != nil || n < 0 {
				return nil, errors.New(constUtil.CopySpecialConfigurationError)
			}
			entry.OriginalTuple[i] = n
		}
		input.Entries = append(input.Entries, entry)
	}
	return input, nil
}

// applyCopySpecialInput is called only inside the successful settlement transaction.
// A failed grant rolls back counters with the original stamina, fight and profile.
func (s *GameService) applyCopySpecialInput(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, input *copySpecialInput) (*pb.ResultObject, []*pb.BaseResource, error) {
	gain := &pb.ResultObject{}
	if input == nil || !input.Enabled {
		return gain, nil, nil
	}
	if input.TaskID == "" || input.Cycle < 1 || input.Cycle > 10000 || input.Source == "" {
		return nil, nil, errors.New(constUtil.CopySpecialConfigurationError)
	}
	raw, _ := json.Marshal(input)
	hash := p6Digest(raw)
	var resources []*pb.BaseResource
	for entryIndex, entry := range input.Entries {
		state := db.CopySpecialState{PlayerID: id, TaskID: input.TaskID, RewardID: entry.Reward, LastInputHash: hash}
		e := tx.First(&state, "player_id=? AND task_id=? AND reward_id=?", id, input.TaskID, entry.Reward).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, nil, e
		}
		if state.Credit < 0 || state.Successes < 0 || state.Spent < 0 || state.Granted < 0 || state.Credit > state.Successes || state.Spent > state.Successes || state.Credit != state.Successes-state.Spent || state.Granted > state.Successes || state.Successes == math.MaxInt64 {
			return nil, nil, errors.New(constUtil.CopySpecialConfigurationError)
		}
		state.Successes++
		state.Credit++
		state.LastInputHash = hash
		if state.Credit >= input.Cycle {
			// Preserve the caller held Run/Fight pointers; its transaction rolls back all partial grants.
			awarded, e := s.grantCopyBattleReward(tx, p, g, entry.Reward, fmt.Sprintf("special:%d", entryIndex), func(fit string) (*pb.ResultObject, error) {
				return s.openServerGain(p, g, fit)
			})
			if e != nil {
				return nil, nil, e
			}
			p1AppendGain(gain, awarded)
			state.Credit -= input.Cycle
			state.Spent += input.Cycle
			state.Granted++
			for _, item := range awarded.GetSimps() {
				resources = append(resources, &pb.BaseResource{Id: item.Id, Num: item.Num})
			}
			for _, equip := range awarded.GetEquips() {
				resources = append(resources, &pb.BaseResource{Id: equip.EquipId, Guid: equip.Guid, Num: proto.Int32(1)})
			}
			for _, warrior := range awarded.GetWars() {
				resources = append(resources, &pb.BaseResource{Id: warrior.Id, Guid: warrior.Guid, Num: proto.Int32(1)})
			}
		}
		if e = tx.Save(&state).Error; e != nil {
			return nil, nil, e
		}
	}
	return gain, resources, nil
}
func (s *GameService) rollCopySpecialDrops(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, task map[string]string) (*pb.ResultObject, error) {
	input, e := s.copySpecialInput(tx, task)
	if e != nil {
		return nil, e
	}
	gain, _, e := s.applyCopySpecialInput(tx, id, p, g, input)
	return gain, e
}
