package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"math"
	"math/big"
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

// InitEquipmentMutations 首次创建显式本地返还策略，重启保留 SQL 运营修改。
func (s *GameService) InitEquipmentMutations(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := tx.AutoMigrate(&db.EquipmentRecycleRule{}, &db.EquipmentMutationReceipt{}); err != nil {
			return err
		}
		rule := db.EquipmentRecycleRule{ID: 1, Enabled: true, ProgressionRefund: constUtil.EquipmentRefundBaseOnly, Source: "APK TableEquipDecompose.retRes; local base_only: original progression refunds unknown"}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error
	})
}

// equipmentMutation 锁定正式 MySQL 存档，在同一事务内保存扣费、奖励和重试回执。
func (s *GameService) equipmentMutation(ctx context.Context, id int64, opcode int32, stamp int64, request, response proto.Message, mutate func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	if stamp <= 0 {
		return errors.New("装备请求缺少有效时间戳")
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		var receipt db.EquipmentMutationReceipt
		err := tx.Where("player_id = ? AND opcode = ?", id, opcode).Take(&receipt).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && stamp <= receipt.Timestamp {
			if stamp < receipt.Timestamp {
				return errors.New("装备请求已过期")
			}
			prior := request.ProtoReflect().Type().New().Interface()
			if proto.Unmarshal(receipt.Request, prior) != nil || !proto.Equal(prior, request) {
				return errors.New("装备请求与已有回执不匹配")
			}
			_, game, err := decodeProfile(&profile)
			if err != nil {
				return err
			}
			if err := proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			return s.validateP5GrowthReceipt(game, response)
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if err = lockProfilePeriods(tx); err != nil {
			return err
		}
		if err = mutate(tx, player, game); err != nil {
			return err
		}
		if err = saveProfile(tx, &profile, player, game); err != nil {
			return err
		}
		rawRequest, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		rawResponse, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		receipt = db.EquipmentMutationReceipt{PlayerID: id, Opcode: opcode, Timestamp: stamp, Request: rawRequest, Response: rawResponse}
		return tx.Save(&receipt).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// equipmentDecompose 每批最多四件，按原资质表发放威名和金币，拒绝任何阵容挂载。
func (s *GameService) equipmentDecompose(ctx context.Context, id int64, request *pb.CSEquipDecompose, stamp int64) ([]proto.Message, error) {
	result := &pb.SCEquipDecompose{}
	err := s.equipmentMutation(ctx, id, constUtil.EquipmentDecomposeOpcode, stamp, request, result, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		var rule db.EquipmentRecycleRule
		if err := tx.Take(&rule, 1).Error; err != nil {
			return err
		}
		if !rule.Enabled || (rule.ProgressionRefund != constUtil.EquipmentRefundBaseOnly && rule.ProgressionRefund != constUtil.EquipmentRefundRejectProgressed) {
			return errors.New("装备分解本地返还策略无效或未启用")
		}
		gain, costs, err := s.equipmentDecomposeApply(player, game, request, rule.ProgressionRefund)
		if err != nil {
			return err
		}
		result.Get, result.Cost = gain, costs
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) equipmentDecomposeApply(player *pb.Player, game *model.GameState, request *pb.CSEquipDecompose, policy string) (*pb.ResultObject, []*pb.BaseResource, error) {
	if len(request.GetGuid()) < 1 || len(request.GetGuid()) > constUtil.EquipmentDecomposeMaxBatch {
		return nil, nil, errors.New("装备分解必须选择1至4件")
	}
	selected := map[string]bool{}
	amounts := map[string]int64{}
	var costs []*pb.BaseResource
	for _, guid := range request.GetGuid() {
		if selected[guid] {
			return nil, nil, errors.New("装备分解实例重复")
		}
		selected[guid] = true
		equip, err := s.equipmentRecord(game, guid)
		if err != nil {
			return nil, nil, err
		}
		for _, slot := range game.Formation {
			for _, thing := range slot.GetThings() {
				if thing.GetGuid() == guid {
					return nil, nil, errors.New("已穿戴装备不能分解")
				}
			}
		}
		if err = equipmentBasicState(equip); err != nil {
			return nil, nil, err
		}
		if err = s.equipmentAdvancedProperties(equip, map[string]float64{}); err != nil {
			return nil, nil, err
		}
		if policy == constUtil.EquipmentRefundRejectProgressed && (equip.GetLevel() > 1 || equip.GetRefineLv() > 0 || equip.GetRefineExp() > 0 || equip.GetStar() > 0 || equip.GetForgeLevel() > 0) {
			return nil, nil, errors.New("本地SQL规则要求先重置进阶装备")
		}
		quality := s.table("Equip", "id", equip.GetEquipId())["quality"]
		row := s.table("EquipDecompose", "quality", quality)
		if row == nil {
			return nil, nil, errors.New("装备分解资质奖励配置缺失")
		}
		for _, part := range strings.Split(row["retRes"], ";") {
			pair := strings.Split(part, ",")
			if len(pair) != 2 || (pair[0] != "coin" && pair[0] != "point") {
				return nil, nil, errors.New("装备分解奖励类型尚未恢复")
			}
			num, err := strconv.ParseInt(pair[1], 10, 32)
			if err != nil || num <= 0 || amounts[pair[0]]+num > math.MaxInt32 {
				return nil, nil, errors.New("装备分解奖励数量无效")
			}
			amounts[pair[0]] += num
		}
		costs = append(costs, &pb.BaseResource{Id: proto.String(equip.GetEquipId()), Guid: proto.String(guid), Num: proto.Int32(1)})
	}
	if player.GetCoin() < 0 || amounts["coin"] > math.MaxInt64-player.GetCoin() || player.GetPoint() < 0 || int64(player.GetPoint())+amounts["point"] > math.MaxInt32 {
		return nil, nil, errors.New("装备分解资源余额溢出")
	}
	player.Coin = proto.Int64(player.GetCoin() + amounts["coin"])
	player.Point = proto.Int32(player.GetPoint() + int32(amounts["point"]))
	var remaining []*pb.Equipment
	for _, equip := range game.Equipment {
		if !selected[equip.GetGuid()] {
			remaining = append(remaining, equip)
		}
	}
	game.Equipment = remaining
	gain := &pb.ResultObject{}
	for _, key := range []string{"point", "coin"} {
		if amounts[key] > 0 {
			gain.Simps = append(gain.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(amounts[key]))})
		}
	}
	return gain, costs, nil
}

// equipmentForge 按原百分比决定成功，成功升一级、失败保级，两种结果均扣原金币和本卡碎片。
func (s *GameService) equipmentForge(ctx context.Context, id int64, request *pb.CSEquipForge, stamp int64) ([]proto.Message, error) {
	result := &pb.SCEquipForge{}
	err := s.equipmentMutation(ctx, id, constUtil.EquipmentForgeOpcode, stamp, request, result, func(_ *gorm.DB, player *pb.Player, game *model.GameState) error {
		equip, err := s.equipmentRecord(game, request.GetGuid())
		if err != nil {
			return err
		}
		if err = equipmentBasicState(equip); err != nil {
			return err
		}
		if err = s.equipmentAdvancedProperties(equip, map[string]float64{}); err != nil {
			return err
		}
		gate := tableNumber(s.table("GameConfigure", "StaticName", "EquipForgeLevel"), "NameValue")
		if gate <= 0 || float64(player.GetLevel()) < gate {
			return errors.New("玩家等级尚未达到装备锻造要求")
		}
		costs, odds, err := s.equipmentForgeCosts(equip)
		if err != nil {
			return err
		}
		if !equipmentResourcesEqual(costs, request.GetCost()) {
			return errors.New("锻造费用与原表不匹配")
		}
		// 先验证并扣除所有权，再在当前事务中抽取一次；回执重放不会再次抽取。
		if err = s.equipmentForgeConsume(player, game, costs); err != nil {
			return err
		}
		roll, err := rand.Int(rand.Reader, big.NewInt(100))
		if err != nil {
			return err
		}
		result.Result = proto.Int32(0)
		if roll.Int64() < odds {
			equip.ForgeLevel = proto.Int32(equip.GetForgeLevel() + 1)
			result.Result = proto.Int32(1)
		}
		result.Equip = proto.Clone(equip).(*pb.Equipment)
		result.Cost = costs
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) equipmentForgeCosts(equip *pb.Equipment) ([]*pb.BaseResource, int64, error) {
	quality := s.table("Equip", "id", equip.GetEquipId())["quality"]
	level := equip.GetForgeLevel() + 1
	var found map[string]string
	for _, row := range s.Tables["EquipForgeOdds"] {
		if row["quality"] == quality && row["forgeLevel"] == strconv.Itoa(int(level)) {
			if found != nil {
				return nil, 0, errors.New("锻造概率配置重复")
			}
			found = row
		}
	}
	if found == nil {
		return nil, 0, errors.New("该装备无锻造配置或已达上限")
	}
	if err := s.equipmentForgeProperties(&pb.Equipment{EquipId: equip.EquipId, ForgeLevel: proto.Int32(level)}, map[string]float64{}); err != nil {
		return nil, 0, err
	}
	odds, err := strconv.ParseInt(found["odds"], 10, 64)
	if err != nil || odds < 0 || odds > 100 {
		return nil, 0, errors.New("锻造百分比无效")
	}
	pair := strings.Split(found["cost"], ",")
	if len(pair) != 2 || pair[0] != "coin" {
		return nil, 0, errors.New("锻造金币配置无效")
	}
	coin, err := strconv.ParseInt(pair[1], 10, 32)
	if err != nil || coin <= 0 {
		return nil, 0, errors.New("锻造金币数量无效")
	}
	frag, err := strconv.ParseInt(found["fragNum"], 10, 32)
	key := "frag_" + equip.GetEquipId()
	if err != nil || frag <= 0 || s.table("Frag", "id", key)["target"] != equip.GetEquipId() {
		return nil, 0, errors.New("锻造本卡碎片配置无效")
	}
	return []*pb.BaseResource{{Id: proto.String(key), Num: proto.Int32(int32(frag))}, {Id: proto.String("coin"), Num: proto.Int32(int32(coin))}}, odds, nil
}

func equipmentResourcesEqual(expected, actual []*pb.BaseResource) bool {
	if len(expected) != len(actual) {
		return false
	}
	seen := map[string]bool{}
	for _, resource := range actual {
		if resource == nil || resource.GetGuid() != "" || seen[resource.GetId()] {
			return false
		}
		seen[resource.GetId()] = true
		matched := false
		for _, cost := range expected {
			if resource.GetId() == cost.GetId() && resource.GetNum() == cost.GetNum() {
				matched = true
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (s *GameService) equipmentForgeConsume(player *pb.Player, game *model.GameState, costs []*pb.BaseResource) error {
	for _, cost := range costs {
		if cost.GetId() == "coin" {
			if player.GetCoin() < int64(cost.GetNum()) {
				return errors.New("锻造金币不足")
			}
			player.Coin = proto.Int64(player.GetCoin() - int64(cost.GetNum()))
			continue
		}
		var owned *pb.SimpleItem
		for _, item := range game.Items {
			if item.GetId() == cost.GetId() {
				if owned != nil {
					return errors.New("装备碎片存档重复")
				}
				owned = item
			}
		}
		if owned == nil || owned.GetNum() < cost.GetNum() {
			return errors.New("锻造本卡碎片不足")
		}
		owned.Num = proto.Int32(owned.GetNum() - cost.GetNum())
	}
	var remaining []*pb.SimpleItem
	for _, item := range game.Items {
		if item.GetNum() != 0 {
			remaining = append(remaining, item)
		}
	}
	game.Items = remaining
	return nil
}

// equipmentForgeProperties 累加所有已完成等级的固定属性，与原 UIEquipment IL 一致。
func (s *GameService) equipmentForgeProperties(equip *pb.Equipment, values map[string]float64) error {
	if equip.GetForgeLevel() == 0 {
		return nil
	}
	if equip.GetForgeLevel() < 0 || equip.GetForgeLevel() > 15 {
		return errors.New("装备锻造等级存档无效")
	}
	seen := map[int32]bool{}
	for _, row := range s.Tables["EquipForgeProp"] {
		if row["equipId"] != equip.GetEquipId() {
			continue
		}
		level, err := strconv.ParseInt(row["forgeLevel"], 10, 32)
		if err != nil || level < 1 {
			return errors.New("装备锻造属性配置无效")
		}
		if level > int64(equip.GetForgeLevel()) {
			continue
		}
		if seen[int32(level)] {
			return errors.New("装备锻造属性等级重复")
		}
		seen[int32(level)] = true
		if err = equipmentPropertyString(values, row["prop"], false); err != nil {
			return err
		}
	}
	if len(seen) != int(equip.GetForgeLevel()) {
		return errors.New("装备锻造属性等级缺档")
	}
	return nil
}
