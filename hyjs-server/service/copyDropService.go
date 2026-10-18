package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type copyDropInput struct {
	Enabled bool              `json:"enabled"`
	Mode    string            `json:"mode"`
	Source  string            `json:"source"`
	Entries []copyDropEntry   `json:"entries"`
	First   *copyDropInput    `json:"first,omitempty"`
	Special *copySpecialInput `json:"special,omitempty"`
}
type copyDropEntry struct {
	ID      string     `json:"id"`
	Options [][2]int64 `json:"options"`
}

// InitializeCopyDrops 创建可审计的相对权重兼容规则；原私服算法仍未恢复。
func (s *GameService) InitializeCopyDrops(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.CopyDropRule{}, &db.CopyDropSnapshot{}, &db.CopySpecialRule{}, &db.CopySpecialState{}); err != nil {
		return err
	}
	rule := db.CopyDropRule{ID: constUtil.CopyDropRuleID, Enabled: true, Mode: constUtil.CopyDropRelativeWeights, Source: "本地兼容：原DropOdds每条配置独立按数量/相对权重抽一次，包含0数量；未知原服抽样/保底算法，SpecialOdds三元保持未恢复；首次掉落使用独立first冻结规则"}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error; err != nil {
		return err
	}
	if e := s.initializeCopyFirstDrops(tx); e != nil {
		return e
	}
	return s.initializeCopySpecialDrops(tx)
}

func (s *GameService) copyDropInput(tx *gorm.DB, content string) (*copyDropInput, error) {
	return s.copyDropInputForRule(tx, content, constUtil.CopyDropRuleID)
}

func (s *GameService) copyDropInputForRule(tx *gorm.DB, content, ruleID string) (*copyDropInput, error) {
	input := &copyDropInput{}
	// 旧隔离夹具和升级前战斗没有该表时，保持已有确定首通行为。
	if !tx.Migrator().HasTable(&db.CopyDropRule{}) {
		return input, nil
	}
	var rule db.CopyDropRule
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&rule, "id = ?", ruleID).Error; err != nil {
		return nil, err
	}
	input.Enabled, input.Mode, input.Source = rule.Enabled, rule.Mode, rule.Source
	if rule.Mode != constUtil.CopyDropRelativeWeights || strings.TrimSpace(rule.Source) == "" {
		return nil, errors.New(constUtil.CopyDropConfigurationError)
	}
	if !rule.Enabled {
		return input, nil
	}
	if len(content) > 16384 {
		return nil, errors.New(constUtil.CopyDropConfigurationError)
	}
	for _, part := range strings.Split(content, "|") {
		if part == "" {
			continue
		}
		pair := strings.Split(part, ":")
		if len(pair) != 2 || pair[0] == "" {
			return nil, errors.New(constUtil.CopyDropConfigurationError)
		}
		entry := copyDropEntry{ID: pair[0]}
		// 即使0数量随机命中，也先验证全部对象，损坏配置不会偶尔成功。
		supported, err := s.openServerRewardSupported(entry.ID + ",1")
		if err != nil || !supported {
			return nil, errors.New(constUtil.CopyDropConfigurationError)
		}
		var total int64
		for _, option := range strings.Split(pair[1], ";") {
			values := strings.Split(option, ",")
			if len(values) != 2 {
				return nil, errors.New(constUtil.CopyDropConfigurationError)
			}
			amount, e1 := strconv.ParseInt(values[0], 10, 32)
			weight, e2 := strconv.ParseInt(values[1], 10, 32)
			if e1 != nil || e2 != nil || amount < 0 || weight <= 0 || total > math.MaxInt64-weight {
				return nil, errors.New(constUtil.CopyDropConfigurationError)
			}
			total += weight
			entry.Options = append(entry.Options, [2]int64{amount, weight})
		}
		if total <= 0 {
			return nil, errors.New(constUtil.CopyDropConfigurationError)
		}
		input.Entries = append(input.Entries, entry)
	}
	return input, nil
}

// captureCopyDrops 与原开始帧同事务保存配置，旧实例不从重启后的表重新抽配置。
func (s *GameService) captureCopyDrops(tx *gorm.DB, id int64, fightID string, task map[string]string) error {
	if !tx.Migrator().HasTable(&db.CopyDropSnapshot{}) {
		return nil
	}
	input, err := s.copyDropInput(tx, task["DropOdds"])
	if err != nil {
		return err
	}
	input.First, err = s.copyFirstDropInput(tx, task["FirstDropOdds"])
	if err != nil {
		return err
	}
	input.Special, err = s.copySpecialInput(tx, task)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return tx.Create(&db.CopyDropSnapshot{PlayerID: id, FightID: fightID, TaskID: task["TaskId"], Input: raw, InputHash: p6Digest(raw)}).Error
}

func (s *GameService) applyCopyDropInput(player *pb.Player, game *model.GameState, input *copyDropInput) (*pb.ResultObject, []*pb.BaseResource, error) {
	return s.applyCopyDropInputWithGrant(player, game, input, func(content string) (*pb.ResultObject, error) {
		return s.openServerGain(player, game, content)
	})
}

func (s *GameService) applyCopyDropInputWithGrant(player *pb.Player, game *model.GameState, input *copyDropInput, grant func(string) (*pb.ResultObject, error)) (*pb.ResultObject, []*pb.BaseResource, error) {
	gain := &pb.ResultObject{}
	if input == nil || !input.Enabled {
		return gain, nil, nil
	}
	if input.Mode != constUtil.CopyDropRelativeWeights || strings.TrimSpace(input.Source) == "" {
		return nil, nil, errors.New(constUtil.CopyDropConfigurationError)
	}
	var content []string
	for _, entry := range input.Entries {
		amount, err := growthPick(entry.Options)
		if err != nil {
			return nil, nil, err
		}
		if amount < 0 || amount > math.MaxInt32 {
			return nil, nil, errors.New(constUtil.CopyDropConfigurationError)
		}
		if amount > 0 {
			content = append(content, fmt.Sprintf("%s,%d", entry.ID, amount))
		}
	}
	if len(content) == 0 {
		return gain, nil, nil
	}
	// 调用方持有Profile锁；直接实际构造实例，避免克隆Game导致Run指针脱离存档。
	var err error
	gain, err = grant(strings.Join(content, ";"))
	if err != nil {
		return nil, nil, err
	}
	var resources []*pb.BaseResource
	for _, item := range gain.GetSimps() {
		resources = append(resources, &pb.BaseResource{Id: item.Id, Num: item.Num})
	}
	for _, warrior := range gain.GetWars() {
		resources = append(resources, &pb.BaseResource{Id: warrior.Id, Guid: warrior.Guid, Num: proto.Int32(1)})
	}
	return gain, resources, nil
}

// settleCopyDrops 仅在未结算胜利分支执行；Profile锁与原Fight回执保证并发一次入库。
func (s *GameService) settleCopyDrops(tx *gorm.DB, id int64, player *pb.Player, game *model.GameState, fight *model.CopyFight) (*pb.ResultObject, []*pb.BaseResource, error) {
	if !tx.Migrator().HasTable(&db.CopyDropSnapshot{}) {
		return &pb.ResultObject{}, nil, nil
	}
	var saved db.CopyDropSnapshot
	err := tx.First(&saved, "player_id=? AND fight_id=?", id, fight.Start.GetFightId()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.ResultObject{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var input copyDropInput
	if saved.TaskID != fight.Task || saved.InputHash != p6Digest(saved.Input) || json.Unmarshal(saved.Input, &input) != nil {
		return nil, nil, errors.New(constUtil.CopyDropConfigurationError)
	}
	gain, resources, err := s.applyCopyDropInputWithGrant(player, game, &input, func(content string) (*pb.ResultObject, error) {
		return s.grantCopyBattleReward(tx, player, game, content, "ordinary", func(fit string) (*pb.ResultObject, error) {
			return s.openServerGain(player, game, fit)
		})
	})
	if err != nil {
		return nil, nil, err
	}
	if input.Special != nil && input.Special.TaskID != saved.TaskID {
		return nil, nil, errors.New(constUtil.CopySpecialConfigurationError)
	}
	special, extra, err := s.applyCopySpecialInput(tx, id, player, game, input.Special)
	if err != nil {
		return nil, nil, err
	}
	p1AppendGain(gain, special)
	resources = append(resources, extra...)
	return gain, resources, nil
}

// rollCopyDrops 将扫荡的普通掉落纳入同包扣费、次数与完整奖励回执。
func (s *GameService) rollCopyDrops(tx *gorm.DB, player *pb.Player, game *model.GameState, content string) (*pb.ResultObject, []*pb.BaseResource, error) {
	input, err := s.copyDropInput(tx, content)
	if err != nil {
		return nil, nil, err
	}
	return s.applyCopyDropInput(player, game, input)
}
