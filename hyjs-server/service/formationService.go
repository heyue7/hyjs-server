package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// PrepareFormation 为旧存档补齐默认阵型，并把历史顺序站位转换为 APK 战斗坐标。
func (s *GameService) PrepareFormation(g *model.GameState) error {
	if len(g.TacticalFormations) == 0 {
		formation, err := s.newTacticalFormation("1", 1)
		if err != nil {
			return err
		}
		// 旧存档没有战术阵型。合法的历史布阵坐标继续保留，其余使用默认位置。
		used := map[int32]bool{}
		legacy := true
		for _, slot := range g.Formation {
			if slot.GetBattlePos() != slot.GetFormatPos() {
				legacy = false
			}
		}
		for _, slot := range g.Formation {
			if slot.GetFormatPos() < 1 || int(slot.GetFormatPos()) > len(formation.PosInfoList) {
				return errors.New("旧阵容位置超出默认阵型容量")
			}
			if !legacy && slices.Contains(s.tacticalPositions("1"), slot.GetBattlePos()) && !used[slot.GetBattlePos()] {
				formation.PosInfoList[slot.GetFormatPos()-1].BattlePos = proto.Int32(slot.GetBattlePos())
				used[slot.GetBattlePos()] = true
			}
		}
		// 填补空位置时避开已经保留的坐标，保证每个位置有独立战斗坐标。
		for _, pos := range formation.PosInfoList {
			var preserved bool
			for _, slot := range g.Formation {
				if !legacy && slot.GetFormatPos() == pos.GetFormatPos() && used[slot.GetBattlePos()] && pos.GetBattlePos() == slot.GetBattlePos() {
					preserved = true
				}
			}
			if preserved {
				continue
			}
			for _, candidate := range s.tacticalPositions("1") {
				if !used[candidate] {
					pos.BattlePos = proto.Int32(candidate)
					used[candidate] = true
					break
				}
			}
		}
		g.TacticalFormations = []*pb.TaticalFormation{formation}
		g.TacticalSkillId = s.tacticalSkill("1", 1)["id"]
	}
	if g.TacticalSkillId == "" {
		g.TacticalSkillId = s.tacticalSkill("1", 1)["id"]
	}
	current, err := s.currentTacticalFormation(g)
	if err != nil {
		return err
	}
	return s.syncFormationPositions(g, current)
}

func (s *GameService) tacticalPositions(id string) []int32 {
	var positions []int32
	for _, value := range strings.Split(s.table("Formation", "id", id)["posInfo"], ",") {
		position, err := strconv.ParseInt(value, 10, 32)
		if err != nil || position <= 0 || slices.Contains(positions, int32(position)) {
			return nil
		}
		positions = append(positions, int32(position))
	}
	return positions
}

func (s *GameService) tacticalSkill(id string, level int32) map[string]string {
	for _, row := range s.Tables["FormationSkill"] {
		if row["fId"] == id && row["fLevel"] == strconv.FormatInt(int64(level), 10) {
			return row
		}
	}
	return nil
}

func (s *GameService) newTacticalFormation(id string, level int32) (*pb.TaticalFormation, error) {
	positions := s.tacticalPositions(id)
	if len(positions) < 4 || len(positions) > 5 || s.tacticalSkill(id, 1) == nil {
		return nil, errors.New("阵型配置不存在")
	}
	formation := &pb.TaticalFormation{Id: proto.String(id), Level: proto.Int32(level)}
	for index, position := range positions {
		formation.PosInfoList = append(formation.PosInfoList, &pb.FormationPosInfo{FormatPos: proto.Int32(int32(index + 1)), BattlePos: proto.Int32(position)})
	}
	return formation, nil
}

func (s *GameService) currentTacticalFormation(g *model.GameState) (*pb.TaticalFormation, error) {
	skill := s.table("FormationSkill", "id", g.TacticalSkillId)
	if skill == nil {
		return nil, errors.New("当前阵型技能配置不存在")
	}
	for _, formation := range g.TacticalFormations {
		if formation.GetId() == skill["fId"] && strconv.FormatInt(int64(formation.GetLevel()), 10) == skill["fLevel"] {
			return formation, nil
		}
	}
	return nil, errors.New("当前阵型尚未解锁")
}

func (s *GameService) syncFormationPositions(g *model.GameState, tactical *pb.TaticalFormation) error {
	var (
		positions = map[int32]int32{}
		used      = map[int32]bool{}
	)
	if len(tactical.GetPosInfoList()) != len(s.tacticalPositions(tactical.GetId())) {
		return errors.New("保存的阵型位置不完整")
	}
	for _, pos := range tactical.GetPosInfoList() {
		if pos.GetFormatPos() < 1 || pos.GetFormatPos() > int32(len(s.tacticalPositions(tactical.GetId()))) || positions[pos.GetFormatPos()] != 0 || used[pos.GetBattlePos()] || !slices.Contains(s.tacticalPositions(tactical.GetId()), pos.GetBattlePos()) {
			return errors.New("保存的阵型位置无效")
		}
		positions[pos.GetFormatPos()] = pos.GetBattlePos()
		used[pos.GetBattlePos()] = true
	}
	seen := map[int32]bool{}
	for _, slot := range g.Formation {
		if seen[slot.GetFormatPos()] || (positions[slot.GetFormatPos()] == 0 && formationHero(slot) != nil) {
			return errors.New("当前英雄阵容不适用于该阵型")
		}
		seen[slot.GetFormatPos()] = true
	}
	// Preserve HP by the server-owned camp identity before changing board cells.
	if err := syncFormationSideHP(g, positions); err != nil {
		return err
	}
	for _, slot := range g.Formation {
		slot.BattlePos = proto.Int32(positions[slot.GetFormatPos()])
	}
	return nil
}

func (s *GameService) tacticalGate(player *pb.Player) error {
	level, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", "formationLevelLimit")["NameValue"], 10, 32)
	if err != nil || level != 20 {
		return errors.New("阵型开放等级配置无效")
	}
	if int64(player.GetLevel()) < level {
		return errors.New("20级开放战术阵型")
	}
	return nil
}

func (s *GameService) formationPosition(player *pb.Player, g *model.GameState, position int32) error {
	if position < 1 || position > 5 {
		return errors.New("主战位置无效")
	}
	limit, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", fmt.Sprintf("array_pos%d", position))["NameValue"], 10, 32)
	if err != nil || limit < 1 {
		return errors.New("阵位开放等级配置不存在")
	}
	if int64(player.GetLevel()) < limit {
		return errors.New("该阵位尚未达到开放等级")
	}
	current, err := s.currentTacticalFormation(g)
	if err != nil {
		return err
	}
	if position > int32(len(current.GetPosInfoList())) {
		return errors.New("当前阵型不支持该阵位")
	}
	return nil
}

func formationHero(slot *pb.FormationItem) *pb.FormationThing {
	for _, thing := range slot.GetThings() {
		if thing.GetPos() == 0 {
			return thing
		}
	}
	return nil
}

func (s *GameService) formationWarrior(g *model.GameState, guid, id string) (*pb.Warrior, error) {
	for _, warrior := range g.Warriors {
		if warrior.GetGuid() == guid && warrior.GetId() == id {
			hero := s.table("Hero", "id", id)
			if hero == nil || hero["heroClass"] == "99" {
				return nil, errors.New("经验材料不能上阵")
			}
			return warrior, nil
		}
	}
	return nil, errors.New("未拥有该角色")
}

// formationList 返回当前阵位与真实技能编号，并持久化旧存档的默认阵型。
func (s *GameService) formationList(ctx context.Context, id int64) ([]proto.Message, error) {
	var result *pb.SCFormationList
	err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		result = formationListResult(g)
		return nil
	})
	return []proto.Message{result}, err
}

func formationListResult(g *model.GameState) *pb.SCFormationList {
	result := &pb.SCFormationList{TacticalSkillId: proto.String(g.TacticalSkillId)}
	result.HelperList = P5HelperList(g)
	for _, slot := range g.Formation {
		// 原InitBattleCamp会无条件读取hero.heroId；空阵位留在存档，响应只投放可初始化的角色阵位。
		if formationHero(slot) == nil {
			continue
		}
		result.FormationList = append(result.FormationList, proto.Clone(slot).(*pb.FormationItem))
	}
	sort.Slice(result.FormationList, func(i, j int) bool {
		return result.FormationList[i].GetFormatPos() < result.FormationList[j].GetFormatPos()
	})
	return result
}

// formationOn 使用已开放主战阵位放入英雄，保留该阵位的装备与宝物。
func (s *GameService) formationOn(ctx context.Context, id int64, b *pb.CSFormationOn) ([]proto.Message, error) {
	if strings.HasPrefix(b.GetId(), "equip_") {
		return s.equipmentFormationOn(ctx, id, b)
	}
	var result *pb.SCFormationOn
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if _, err := s.formationWarrior(g, b.GetGuid(), b.GetId()); err != nil {
			return err
		}
		if err := s.P5FormationHeroAllowed(b.GetId()); err != nil {
			return err
		}
		for _, helper := range P5HelperList(g) {
			if helper.GetGuid() == b.GetGuid() || helper.GetId() == b.GetId() {
				return errors.New("助阵伙伴须先退出助阵才能上阵")
			}
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		if err := s.formationPosition(player, g, b.GetFormatPos()); err != nil {
			return err
		}
		var target *pb.FormationItem
		for _, slot := range g.Formation {
			if slot.GetFormatPos() == b.GetFormatPos() {
				target = slot
			}
			if hero := formationHero(slot); hero != nil && slot.GetFormatPos() != b.GetFormatPos() && (hero.GetGuid() == b.GetGuid() || hero.GetId() == b.GetId()) {
				return errors.New("该角色已经在其他位置上阵")
			}
		}
		if target == nil {
			target = &pb.FormationItem{FormatPos: proto.Int32(b.GetFormatPos())}
			g.Formation = append(g.Formation, target)
		}
		things := []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String(b.GetId()), Guid: proto.String(b.GetGuid())}}
		for _, thing := range target.GetThings() {
			if thing.GetPos() != 0 {
				things = append(things, thing)
			}
		}
		target.Things = things
		current, err := s.currentTacticalFormation(g)
		if err != nil {
			return err
		}
		if err = s.syncFormationPositions(g, current); err != nil {
			return err
		}
		if b.GetFormatPos() == 2 && g.GuideRecruit != nil && player.GetNoviceGuideSchedule() < 3 {
			player.NoviceGuideSchedule = proto.Int32(3)
		}
		if err := s.syncEquipmentOwners(g); err != nil {
			return err
		}
		power, err := s.formationFightPower(player, g)
		if err != nil {
			return err
		}
		result = &pb.SCFormationOn{FormationItem: proto.Clone(target).(*pb.FormationItem), Fightpower: proto.Int32(power)}
		return nil
	})
	return []proto.Message{result}, err
}

// formationOff 清除指定英雄，重复下阵请求不会移除后来换入的其他英雄。
func (s *GameService) formationOff(ctx context.Context, id int64, b *pb.CSFormationOff) ([]proto.Message, error) {
	if strings.HasPrefix(b.GetId(), "equip_") {
		return s.equipmentFormationOff(ctx, id, b)
	}
	var result *pb.SCFormationOff
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if _, err := s.formationWarrior(g, b.GetGuid(), b.GetId()); err != nil {
			return err
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		if err := s.formationPosition(player, g, b.GetFormatPos()); err != nil {
			return err
		}
		for _, slot := range g.Formation {
			if slot.GetFormatPos() != b.GetFormatPos() {
				continue
			}
			hero := formationHero(slot)
			if hero != nil && (hero.GetGuid() != b.GetGuid() || hero.GetId() != b.GetId()) {
				return errors.New("指定英雄已不在该阵位")
			}
			// 只有实际卸下英雄时才核对剩余人数，空位重试保持幂等。
			if hero != nil {
				members := 0
				for _, member := range g.Formation {
					if formationHero(member) != nil {
						members++
					}
				}
				if members <= 1 {
					return errors.New(constUtil.FormationRequiresHero)
				}
			}
			// 下阵同时解除该槽穿戴，库存实例及其培养仍保留，冷登不会产生隐藏穿戴。
			for _, thing := range slot.GetThings() {
				if thing.GetPos() >= 1 && thing.GetPos() <= 4 {
					equip, err := s.equipmentRecord(g, thing.GetGuid())
					if err != nil {
						return err
					}
					equip.LastHeroId = proto.String("")
				}
			}
			slot.Things = nil
		}
		if err := s.syncEquipmentOwners(g); err != nil {
			return err
		}
		power, err := s.formationFightPower(player, g)
		if err != nil {
			return err
		}
		result = &pb.SCFormationOff{Fightpower: proto.Int32(power)}
		return nil
	})
	return []proto.Message{result}, err
}

// formationExchange 交换整套英雄与挂载对象，并通过原请求时间戳识别网络重试。
func (s *GameService) formationExchange(ctx context.Context, id int64, b *pb.CSFormationExChange, stamp int64) ([]proto.Message, error) {
	var result *pb.SCFormationExChange
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		if stamp <= 0 {
			return errors.New("交换请求缺少有效时间戳")
		}
		if receipt := g.FormationExchangeReceipt; receipt != nil {
			if stamp == receipt.Timestamp {
				if receipt.FromPos != b.GetFromPos() || receipt.ToPos != b.GetToPos() || receipt.Response == nil {
					return errors.New("同一交换时间戳对应不同请求")
				}
				if err := s.validateP5FormationExchangeReceipt(tx, id, g, receipt.Response); err != nil {
					return err
				}
				result = proto.Clone(receipt.Response).(*pb.SCFormationExChange)
				return nil
			}
			if stamp < receipt.Timestamp {
				return errors.New("交换请求已过期")
			}
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		if err := s.formationPosition(player, g, b.GetFromPos()); err != nil {
			return err
		}
		if err := s.formationPosition(player, g, b.GetToPos()); err != nil {
			return err
		}
		var from, to *pb.FormationItem
		for _, slot := range g.Formation {
			if slot.GetFormatPos() == b.GetFromPos() {
				from = slot
			}
			if slot.GetFormatPos() == b.GetToPos() {
				to = slot
			}
		}
		if formationHero(from) == nil || formationHero(to) == nil {
			return errors.New("交换位置必须都有上阵英雄")
		}
		for _, slot := range []*pb.FormationItem{from, to} {
			hero := formationHero(slot)
			if _, err := s.formationWarrior(g, hero.GetGuid(), hero.GetId()); err != nil {
				return err
			}
		}
		// Avatar exchange carries life with each hero while board cells stay put.
		positions := map[int32]int32{}
		for _, slot := range g.Formation {
			positions[slot.GetFormatPos()] = slot.GetBattlePos()
		}
		positions[from.GetFormatPos()] = to.GetBattlePos()
		positions[to.GetFormatPos()] = from.GetBattlePos()
		if err := syncFormationSideHP(g, positions); err != nil {
			return err
		}
		from.Things, to.Things = to.Things, from.Things
		if err := s.syncEquipmentOwners(g); err != nil {
			return err
		}
		result = &pb.SCFormationExChange{FormationList: formationListResult(g).FormationList}
		g.FormationExchangeReceipt = &model.FormationExchangeReceipt{Timestamp: stamp, FromPos: b.GetFromPos(), ToPos: b.GetToPos(), Response: proto.Clone(result).(*pb.SCFormationExChange)}
		return nil
	})
	return []proto.Message{result}, err
}

// tacticalFormationList 列出实际已解锁阵型与一一对应的等级。
func (s *GameService) tacticalFormationList(ctx context.Context, id int64) ([]proto.Message, error) {
	var result *pb.SCTaticalFormationList
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if err := s.tacticalGate(player); err != nil {
			return err
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		result = &pb.SCTaticalFormationList{}
		for _, tactical := range g.TacticalFormations {
			result.UnlockTacticalList = append(result.UnlockTacticalList, tactical.GetId())
			result.TacticalLevelList = append(result.TacticalLevelList, tactical.GetLevel())
		}
		return nil
	})
	return []proto.Message{result}, err
}

// tacticalFormationGet 使用零等级展示未解锁阵型，已解锁阵型返回保存的站位。
func (s *GameService) tacticalFormationGet(ctx context.Context, id int64, b *pb.CSGetTaticalFormation) ([]proto.Message, error) {
	var result *pb.SCGetTaticalFormation
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if err := s.tacticalGate(player); err != nil {
			return err
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		formation, err := s.newTacticalFormation(b.GetId(), 0)
		if err != nil {
			return err
		}
		for _, existing := range g.TacticalFormations {
			if existing.GetId() == b.GetId() {
				formation = proto.Clone(existing).(*pb.TaticalFormation)
			}
		}
		result = &pb.SCGetTaticalFormation{Formation: tacticalFormationView(g, formation)}
		return nil
	})
	return []proto.Message{result}, err
}

// tacticalFormationUnlock 按第一级配置扣费，并重播同包回执以同步客户端材料显示。
func (s *GameService) tacticalFormationUnlock(ctx context.Context, id int64, b *pb.CSTaticalFormationUnLock, stamp int64) ([]proto.Message, error) {
	var result *pb.SCTaticalFormationUnLock
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if err := s.tacticalGate(player); err != nil {
			return err
		}
		if stamp <= 0 {
			return errors.New("解锁请求缺少有效时间戳")
		}
		if receipt := g.TacticalUnlockReceipts[b.GetTacticalId()]; receipt != nil {
			if stamp == receipt.Timestamp {
				if receipt.Response == nil {
					return errors.New("阵型解锁回执无效")
				}
				result = proto.Clone(receipt.Response).(*pb.SCTaticalFormationUnLock)
				return nil
			}
			if stamp < receipt.Timestamp {
				return errors.New("阵型解锁请求已过期")
			}
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		if g.TacticalUnlockReceipts == nil {
			g.TacticalUnlockReceipts = map[string]*model.TacticalUnlockReceipt{}
		}
		for _, formation := range g.TacticalFormations {
			if formation.GetId() == b.GetTacticalId() {
				result = &pb.SCTaticalFormationUnLock{TacticalId: proto.String(formation.GetId()), CurrLevel: proto.Int32(formation.GetLevel())}
				g.TacticalUnlockReceipts[formation.GetId()] = &model.TacticalUnlockReceipt{Timestamp: stamp, Response: proto.Clone(result).(*pb.SCTaticalFormationUnLock)}
				return nil
			}
		}
		formation, err := s.newTacticalFormation(b.GetTacticalId(), 1)
		if err != nil {
			return err
		}
		costs, err := s.payFormationCost(player, g, s.tacticalSkill(b.GetTacticalId(), 1)["levelUpCost"])
		if err != nil {
			return err
		}
		g.TacticalFormations = append(g.TacticalFormations, formation)
		result = &pb.SCTaticalFormationUnLock{TacticalId: proto.String(formation.GetId()), CurrLevel: proto.Int32(1), Cost: costs}
		g.TacticalUnlockReceipts[formation.GetId()] = &model.TacticalUnlockReceipt{Timestamp: stamp, Response: proto.Clone(result).(*pb.SCTaticalFormationUnLock)}
		return nil
	})
	return []proto.Message{result}, err
}

// tacticalFormationLevelUp 读取下一等级消耗，并保存有限回执防止重试重复升级。
func (s *GameService) tacticalFormationLevelUp(ctx context.Context, id int64, b *pb.CSTaticalFormationLevelUp, stamp int64) ([]proto.Message, error) {
	var result *pb.SCTaticalFormationLevelUp
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if err := s.tacticalGate(player); err != nil {
			return err
		}
		if stamp <= 0 {
			return errors.New("升级请求缺少有效时间戳")
		}
		if receipt := g.TacticalUpgradeReceipts[b.GetTacticalId()]; receipt != nil {
			if stamp == receipt.Timestamp {
				if receipt.Response == nil {
					return errors.New("阵型升级回执无效")
				}
				result = proto.Clone(receipt.Response).(*pb.SCTaticalFormationLevelUp)
				return nil
			}
			if stamp < receipt.Timestamp {
				return errors.New("阵型升级请求已过期")
			}
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		var target *pb.TaticalFormation
		for _, formation := range g.TacticalFormations {
			if formation.GetId() == b.GetTacticalId() {
				target = formation
			}
		}
		if target == nil {
			return errors.New("阵型尚未解锁")
		}
		next := s.tacticalSkill(target.GetId(), target.GetLevel()+1)
		if next == nil {
			return errors.New("阵型已达到最高等级")
		}
		current, err := s.currentTacticalFormation(g)
		if err != nil {
			return err
		}
		costs, err := s.payFormationCost(player, g, next["levelUpCost"])
		if err != nil {
			return err
		}
		target.Level = proto.Int32(target.GetLevel() + 1)
		if current == target {
			g.TacticalSkillId = next["id"]
		}
		result = &pb.SCTaticalFormationLevelUp{TacticalId: proto.String(target.GetId()), CurrLevel: proto.Int32(target.GetLevel()), Cost: costs}
		if g.TacticalUpgradeReceipts == nil {
			g.TacticalUpgradeReceipts = map[string]*model.TacticalUpgradeReceipt{}
		}
		g.TacticalUpgradeReceipts[target.GetId()] = &model.TacticalUpgradeReceipt{Timestamp: stamp, Response: proto.Clone(result).(*pb.SCTaticalFormationLevelUp)}
		return nil
	})
	return []proto.Message{result}, err
}

// payFormationCost 先验证全部余额，再按服务器配置扣除金币、钻石或道具。
func (s *GameService) payFormationCost(player *pb.Player, g *model.GameState, spec string) ([]*pb.BaseResource, error) {
	var (
		costs []*pb.BaseResource
		items = map[string]*pb.SimpleItem{}
		seen  = map[string]bool{}
	)
	if spec == "" {
		return nil, nil
	}
	for _, item := range g.Items {
		items[item.GetId()] = item
	}
	for _, part := range strings.Split(spec, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 || seen[pair[0]] {
			return nil, errors.New("阵型消耗配置无效")
		}
		num, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || num <= 0 {
			return nil, errors.New("阵型消耗数量无效")
		}
		seen[pair[0]] = true
		switch pair[0] {
		case "coin":
			if player.GetCoin() < num {
				return nil, errors.New("金币不足")
			}
		case "diamond":
			if int64(player.GetDiamond()) < num {
				return nil, errors.New("钻石不足")
			}
		default:
			if !strings.HasPrefix(pair[0], "item_") || items[pair[0]] == nil || int64(items[pair[0]].GetNum()) < num {
				return nil, errors.New("阵型材料不足")
			}
		}
		costs = append(costs, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(num))})
	}
	for _, cost := range costs {
		switch cost.GetId() {
		case "coin":
			player.Coin = proto.Int64(player.GetCoin() - int64(cost.GetNum()))
		case "diamond":
			player.Diamond = proto.Int64(player.GetDiamond() - int64(cost.GetNum()))
		default:
			item := items[cost.GetId()]
			item.Num = proto.Int32(item.GetNum() - cost.GetNum())
		}
	}
	return costs, nil
}

// tacticalFormationUse 切换已经解锁的阵型，保留各阵型各自的站位记录。
func (s *GameService) tacticalFormationUse(ctx context.Context, id int64, b *pb.CSUseTaticalFormation) ([]proto.Message, error) {
	result := &pb.SCUseTaticalFormation{}
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		return s.tacticalFormationUseApply(player, g, b, result)
	})
	return []proto.Message{result}, err
}

// tacticalFormationUseApply 保留原选择和战斗坐标更新，两个协议入口共用同一事务动作。
func (s *GameService) tacticalFormationUseApply(player *pb.Player, g *model.GameState, b *pb.CSUseTaticalFormation, result *pb.SCUseTaticalFormation) error {
	if err := s.tacticalGate(player); err != nil {
		return err
	}
	if err := s.PrepareFormation(g); err != nil {
		return err
	}
	for _, formation := range g.TacticalFormations {
		if formation.GetId() == b.GetId() {
			if err := s.syncFormationPositions(g, formation); err != nil {
				return err
			}
			g.TacticalSkillId = s.tacticalSkill(formation.GetId(), formation.GetLevel())["id"]
			result.Id = proto.String(formation.GetId())
			return nil
		}
	}
	return errors.New("阵型尚未解锁")
}

// changeTacticalPositions 校验完整在阵英雄映射，未占用的位置从剩余坐标补齐。
func (s *GameService) changeTacticalPositions(player *pb.Player, g *model.GameState, tactical *pb.TaticalFormation, spec string) error {
	var (
		active    = map[int32]bool{}
		seen      = map[int32]bool{}
		used      = map[int32]bool{}
		mapped    = map[int32]int32{}
		guids     = map[string]bool{}
		positions = s.tacticalPositions(tactical.GetId())
	)
	for _, slot := range g.Formation {
		hero := formationHero(slot)
		if hero == nil {
			continue
		}
		if _, err := s.formationWarrior(g, hero.GetGuid(), hero.GetId()); err != nil {
			return err
		}
		if active[slot.GetFormatPos()] || guids[hero.GetGuid()] || slot.GetFormatPos() < 1 || slot.GetFormatPos() > int32(len(positions)) {
			return errors.New("英雄阵容存在重复或不适用位置")
		}
		limit, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", fmt.Sprintf("array_pos%d", slot.GetFormatPos()))["NameValue"], 10, 32)
		if err != nil || int64(player.GetLevel()) < limit {
			return errors.New("英雄阵位尚未开放")
		}
		active[slot.GetFormatPos()] = true
		guids[hero.GetGuid()] = true
	}
	for _, part := range strings.Split(spec, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return errors.New("布阵映射格式无效")
		}
		formatPos, err := strconv.ParseInt(pair[0], 10, 32)
		if err != nil {
			return errors.New("阵容位置无效")
		}
		battlePos, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || !active[int32(formatPos)] || seen[int32(formatPos)] || used[int32(battlePos)] || !slices.Contains(positions, int32(battlePos)) {
			return errors.New("布阵包含未上阵、重复或非法位置")
		}
		seen[int32(formatPos)] = true
		used[int32(battlePos)] = true
		mapped[int32(formatPos)] = int32(battlePos)
	}
	if len(seen) != len(active) {
		return errors.New("布阵必须包含全部上阵英雄")
	}
	for formatPos := int32(1); formatPos <= int32(len(positions)); formatPos++ {
		if mapped[formatPos] != 0 {
			continue
		}
		// 保留闲置阵位的旧位置，冲突时使用第一个剩余合法坐标。
		for _, old := range tactical.GetPosInfoList() {
			if old.GetFormatPos() == formatPos && !used[old.GetBattlePos()] && slices.Contains(positions, old.GetBattlePos()) {
				mapped[formatPos] = old.GetBattlePos()
				used[old.GetBattlePos()] = true
			}
		}
		if mapped[formatPos] == 0 {
			for _, candidate := range positions {
				if !used[candidate] {
					mapped[formatPos] = candidate
					used[candidate] = true
					break
				}
			}
		}
	}
	tactical.PosInfoList = nil
	for formatPos := int32(1); formatPos <= int32(len(positions)); formatPos++ {
		tactical.PosInfoList = append(tactical.PosInfoList, &pb.FormationPosInfo{FormatPos: proto.Int32(formatPos), BattlePos: proto.Int32(mapped[formatPos])})
	}
	current, err := s.currentTacticalFormation(g)
	if err != nil {
		return err
	}
	if current == tactical {
		return s.syncFormationPositions(g, tactical)
	}
	return nil
}

// tacticalFormationChange 只更新已经解锁阵型的合法完整映射。
func (s *GameService) tacticalFormationChange(ctx context.Context, id int64, b *pb.CSTaticalFormationChange) ([]proto.Message, error) {
	result := &pb.SCTaticalFormationChange{}
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		return s.tacticalFormationChangeApply(player, g, b, result)
	})
	return []proto.Message{result}, err
}

// tacticalFormationChangeApply 沿用原完整映射校验和当前阵型的坐标同步。
func (s *GameService) tacticalFormationChangeApply(player *pb.Player, g *model.GameState, b *pb.CSTaticalFormationChange, result *pb.SCTaticalFormationChange) error {
	if err := s.tacticalGate(player); err != nil {
		return err
	}
	if err := s.PrepareFormation(g); err != nil {
		return err
	}
	for _, tactical := range g.TacticalFormations {
		if tactical.GetId() == b.GetId() {
			if err := s.changeTacticalPositions(player, g, tactical, b.GetFormation()); err != nil {
				return err
			}
			result.Formation = tacticalFormationView(g, tactical)
			return nil
		}
	}
	return errors.New("阵型尚未解锁")
}

// formationChange 兼容旧协议的拼写，将映射应用到当前战术阵型。
func (s *GameService) formationChange(ctx context.Context, id int64, b *pb.CSFormationChange) ([]proto.Message, error) {
	var result *pb.SCFormationChange
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if err := s.tacticalGate(player); err != nil {
			return err
		}
		if err := s.PrepareFormation(g); err != nil {
			return err
		}
		current, err := s.currentTacticalFormation(g)
		if err != nil {
			return err
		}
		if err := s.changeTacticalPositions(player, g, current, b.GetFomationMapStr()); err != nil {
			return err
		}
		result = &pb.SCFormationChange{FormationList: formationListResult(g).FormationList}
		return nil
	})
	return []proto.Message{result}, err
}

// AddFormationBonus 在英雄百分比结算前合并阵型加成，避免两次乘法放大面板。
func (s *GameService) AddFormationBonus(g *model.GameState, values, percent map[string]float64) error {
	if g.TacticalSkillId == "" {
		return nil
	}
	skill := s.table("FormationSkill", "id", g.TacticalSkillId)
	if skill == nil {
		return errors.New("阵型属性配置不存在")
	}
	if skill["bonus"] == "" {
		return nil
	}
	bonuses := map[string]float64{}
	for _, part := range strings.Split(skill["bonus"], ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return errors.New("阵型属性格式无效")
		}
		// 原表的阵型2六级含尾空格，APK Double.Parse 接受该写法。
		value, err := strconv.ParseFloat(strings.TrimSpace(pair[1]), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return errors.New("阵型属性数值无效")
		}
		// APK SetAttribute 使用 SetProp，同属性重复配置取最后一项。
		bonuses[strings.TrimSpace(pair[0])] = value
	}
	for key, value := range bonuses {
		switch key {
		case "perhp", "peratt", "perpdef", "permdef":
			percent[strings.TrimPrefix(key, "per")] += value * 0.01
		case "hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce":
			values[key] += value
		default:
			return errors.New("阵型属性类型无效")
		}
	}
	return nil
}

// formationFightPower 对齐 UIHeroFormation 的战斗力公式，并使用同一战斗属性来源。
func (s *GameService) formationFightPower(player *pb.Player, g *model.GameState) (int32, error) {
	party, err := s.battlePlayer(player, g)
	if err != nil {
		return 0, err
	}
	var total float64
	for _, actor := range party.GetFormation() {
		prop := actor.GetProp()
		hero := s.table("Hero", "id", actor.GetId())
		var defaultPercent float64
		for _, key := range []string{"hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce"} {
			defaultPercent += tableNumber(hero, key)
		}
		var weight float64
		property := map[string]float64{"att": prop.GetAtt(), "hp": prop.GetHp(), "pdef": prop.GetPdef(), "mdef": prop.GetMdef()}
		for _, key := range []string{"att", "hp", "pdef", "mdef"} {
			value := property[key]
			factor, err := strconv.ParseFloat(s.table("GameConfigure", "StaticName", "fightpower_"+key+"_factor")["NameValue"], 64)
			if err != nil || factor <= 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
				return 0, errors.New("战斗力配置无效")
			}
			weight += value * factor
		}
		total += weight * 0.1 * (100 + prop.GetHit() + prop.GetDodge() + prop.GetCri() + prop.GetAntiCri() + prop.GetCriFactor() + prop.GetDamageAdd() + prop.GetDamageReduce() - defaultPercent) * 0.01
	}
	if math.IsNaN(total) || math.IsInf(total, 0) || total < 0 || total > math.MaxInt32 {
		return 0, errors.New("战斗力超出协议范围")
	}
	return int32(math.RoundToEven(total)), nil
}
