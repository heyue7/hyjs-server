package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// DispatchP5Next 单独暴露下一组候选接点，当前已发布P5入口保持原行为。
// HeroReplacePage发送CSFormationOn，替换任意主战位均保留至少一名角色。
func (s *GameService) DispatchP5Next(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSWarriorStrengthen:
		out, err := s.p5WarriorStrengthen(ctx, id, b, stamp)
		return out, true, err
	case *pb.CSTreasureStrengthen:
		out, err := s.p5TreasureStrengthen(ctx, id, b, stamp)
		return out, true, err
	case *pb.CSUseTaticalFormation:
		out, err := s.p5TacticalFormationUse(ctx, id, b, stamp)
		return out, true, err
	case *pb.CSTaticalFormationChange:
		out, err := s.p5TacticalFormationChange(ctx, id, b, stamp)
		return out, true, err
	case *pb.CSFormationOn:
		if strings.HasPrefix(b.GetId(), "equip_") {
			out, err := s.p5EquipmentFormationOn(ctx, id, b, stamp)
			return out, true, err
		}
		if strings.HasPrefix(b.GetId(), "trea_") {
			out, err := s.p5TreasureFormationOn(ctx, id, b, stamp)
			return out, true, err
		}
	case *pb.CSFormationOff:
		if strings.HasPrefix(b.GetId(), "equip_") {
			out, err := s.p5EquipmentFormationOff(ctx, id, b, stamp)
			return out, true, err
		}
		if strings.HasPrefix(b.GetId(), "trea_") {
			out, err := s.p5TreasureFormationOff(ctx, id, b, stamp)
			return out, true, err
		}
	}
	if b, ok := request.(*pb.CSHelperOn); ok {
		out, err := s.p5HelperOnNative(ctx, id, b, stamp)
		return out, true, err
	}
	b, ok := request.(*pb.CSFormationOn)
	if !ok || !strings.HasPrefix(b.GetId(), "hero_") {
		return nil, false, nil
	}
	out, err := s.p5FormationOnNative(ctx, id, b, stamp)
	return out, true, err
}

// p5HelperOnNative 在原主角可退出主战阵容后仍按精确GUID保护其主角身份。
func (s *GameService) p5HelperOnNative(ctx context.Context, id int64, b *pb.CSHelperOn, stamp int64) ([]proto.Message, error) {
	return s.p5Mutation(ctx, id, constUtil.P5HelperOnOperation, stamp, b, &pb.SCHelperOn{}, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if b.GetPos() < 1 || b.GetPos() > 6 || b.GetGuid() == p.GetHeroWarId() {
			return p5Error()
		}
		gate := int32(tableNumber(s.table("GameConfigure", "StaticName", fmt.Sprintf("array_help_pos%d", b.GetPos())), "NameValue"))
		if gate <= 0 || p.GetLevel() < gate {
			return p5Error()
		}
		w, err := p5Warrior(g, b.GetGuid())
		if err != nil {
			return err
		}
		row := s.table("Hero", "id", w.GetId())
		if row == nil || row["heroClass"] == "99" || row["heroClass"] == "100" || row["heroClass"] == "101" {
			return p5Error()
		}
		for _, slot := range g.Formation {
			if h := formationHero(slot); h != nil && (h.GetGuid() == w.GetGuid() || h.GetId() == w.GetId()) {
				return p5Error()
			}
		}
		for _, h := range P5HelperList(g) {
			if h.GetPos() != b.GetPos() && (h.GetGuid() == w.GetGuid() || h.GetId() == w.GetId()) {
				return p5Error()
			}
		}
		if p5KV(g, "p5.dispatch:"+w.GetGuid()) != "" {
			return p5Error()
		}
		p5SetKV(g, fmt.Sprintf("p5.helper:%d", b.GetPos()), w.GetGuid())
		return nil
	})
}

// p5FormationOnNative 更换阵容位上的已有角色，保留原主角GUID、成长和该位穿戴。
// 原包没有独立主角身份转换消息；新队员继续使用自己的实例与成长数据。
func (s *GameService) p5FormationOnNative(ctx context.Context, id int64, b *pb.CSFormationOn, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFormationOn{}
	return s.p5Mutation(ctx, id, constUtil.P5FormationOnOperation, stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		w, err := p5Warrior(g, b.GetGuid())
		if err != nil || w.GetId() != b.GetId() {
			return p5Error()
		}
		if _, err = s.formationWarrior(g, b.GetGuid(), b.GetId()); err != nil {
			return err
		}
		for _, helper := range P5HelperList(g) {
			if helper.GetGuid() == b.GetGuid() || helper.GetId() == b.GetId() {
				return errors.New("助阵伙伴须先退出助阵才能上阵")
			}
		}
		if p5KV(g, "p5.dispatch:"+b.GetGuid()) != "" {
			return errors.New("派遣中的伙伴不可上阵")
		}
		if err = s.PrepareFormation(g); err != nil {
			return err
		}
		if err = s.formationPosition(p, g, b.GetFormatPos()); err != nil {
			return err
		}
		var target *pb.FormationItem
		for _, slot := range g.Formation {
			if slot.GetFormatPos() == b.GetFormatPos() {
				if target != nil {
					return p5Error()
				}
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
		// 阵容装备归该阵容位，换英雄只替换pos=0，随后同步实际装备主人。
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
		if b.GetFormatPos() == 2 && g.GuideRecruit != nil && p.GetNoviceGuideSchedule() < 3 {
			p.NoviceGuideSchedule = proto.Int32(3)
		}
		if err = s.syncEquipmentOwners(g); err != nil {
			return err
		}
		power, err := s.formationFightPower(p, g)
		if err != nil {
			return err
		}
		out.FormationItem = proto.Clone(target).(*pb.FormationItem)
		out.Fightpower = proto.Int32(power)
		return nil
	})
}

// validateP5FormationReceipt keeps the original complete slot callback current.
// HeroReplacePage.InitData replaces all slot things, so an earlier hero-on
// response must not restore a removed hero, equipment or guardian descriptor.
func validateP5FormationReceipt(p *pb.Player, g *model.GameState, response proto.Message) error {
	result, ok := response.(*pb.SCFormationOn)
	if !ok {
		return nil
	}
	snapshot := result.GetFormationItem()
	if snapshot == nil {
		return errors.New(constUtil.P5GrowthReceiptChanged)
	}
	var current *pb.FormationItem
	for _, slot := range g.Formation {
		if slot.GetFormatPos() == snapshot.GetFormatPos() {
			if current != nil {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			current = slot
		}
	}
	if current == nil || !proto.Equal(current, snapshot) || result.GetFightpower() != p.GetFightPower() {
		return errors.New(constUtil.P5GrowthReceiptChanged)
	}
	return nil
}

// validateP5HelperReceipt binds empty original acknowledgements to their live
// request effects. The original closures write/remove the captured helper slot.
// Current helpers may grow without invalidating an unchanged assignment.
func validateP5HelperReceipt(g *model.GameState, request, response proto.Message) error {
	switch response.(type) {
	case *pb.SCHelperOn:
		original, ok := request.(*pb.CSHelperOn)
		if !ok || original.GetPos() < 1 || original.GetPos() > 6 || p5KV(g, fmt.Sprintf("p5.helper:%d", original.GetPos())) != original.GetGuid() {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
		if _, err := p5Warrior(g, original.GetGuid()); err != nil {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	case *pb.SCHelperOff:
		original, ok := request.(*pb.CSHelperOff)
		if !ok || original.GetPos() < 1 || original.GetPos() > 6 || p5KV(g, fmt.Sprintf("p5.helper:%d", original.GetPos())) != "" {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	}
	return nil
}

// validateP5FormationExchangeReceipt protects the original InitBattleCamp whole
// list callback. Descriptors allow current growth, but every referenced GUID
// must remain owned and the full current arrangement must still match.
func (s *GameService) validateP5FormationExchangeReceipt(tx *gorm.DB, id int64, g *model.GameState, result *pb.SCFormationExChange) error {
	current := &pb.SCFormationExChange{FormationList: formationListResult(g).GetFormationList()}
	if result == nil || !proto.Equal(current, result) {
		return errors.New(constUtil.P5GrowthReceiptChanged)
	}
	positions := map[int32]bool{}
	guids := map[string]bool{}
	for _, slot := range result.GetFormationList() {
		if slot == nil || positions[slot.GetFormatPos()] {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
		positions[slot.GetFormatPos()] = true
		types := map[int32]bool{}
		for _, thing := range slot.GetThings() {
			if thing == nil || thing.GetGuid() == "" || thing.GetId() == "" || guids[thing.GetGuid()] || types[thing.GetPos()] {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			guids[thing.GetGuid()] = true
			types[thing.GetPos()] = true
			var currentID string
			var err error
			switch thing.GetPos() {
			case 0:
				var hero *pb.Warrior
				hero, err = p5Warrior(g, thing.GetGuid())
				currentID = hero.GetId()
			case 1, 2, 3, 4:
				var equip *pb.Equipment
				equip, err = s.equipmentRecord(g, thing.GetGuid())
				currentID = equip.GetEquipId()
			case 5, 6:
				var treasure *pb.Treasure
				treasure, err = s.treasureRecord(g, thing.GetGuid())
				currentID = treasure.GetTreasureId()
			case 7:
				var pet *pb.Pet
				pet, err = p9OwnedPet(tx, id, thing.GetGuid())
				currentID = pet.GetId()
			default:
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			if err != nil || currentID != thing.GetId() {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
		}
	}
	return nil
}

// p5TacticalFormationUse 冻结最近一次原选择所使用的完整坐标和技能等级。
// 原回调还会读取页面缓存的等级/站位，后续选择或映射变化须拒绝旧响应。
func (s *GameService) p5TacticalFormationUse(ctx context.Context, id int64, b *pb.CSUseTaticalFormation, stamp int64) ([]proto.Message, error) {
	out := &pb.SCUseTaticalFormation{}
	return s.p5Mutation(ctx, id, constUtil.P5TacticalUseOperation, stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if err := s.tacticalFormationUseApply(p, g, b, out); err != nil {
			return err
		}
		current, err := s.currentTacticalFormation(g)
		if err != nil {
			return err
		}
		raw, err := proto.Marshal(current)
		if err != nil {
			return err
		}
		// 只保存最新一次选择的有限快照，不随历史请求增加存档字段。
		p5SetKV(g, constUtil.P5TacticalUseReceiptKey, strconv.FormatInt(stamp, 10)+":"+base64.RawStdEncoding.EncodeToString(raw))
		return nil
	})
}

func (s *GameService) p5TacticalFormationChange(ctx context.Context, id int64, b *pb.CSTaticalFormationChange, stamp int64) ([]proto.Message, error) {
	out := &pb.SCTaticalFormationChange{}
	return s.p5Mutation(ctx, id, constUtil.P5TacticalChangeOperation, stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.tacticalFormationChangeApply(p, g, b, out)
	})
}

// validateP5TacticalReceipt 同事务核对原回调将写入的选择/等级/完整站位。
func (s *GameService) validateP5TacticalReceipt(g *model.GameState, request, response proto.Message, stamp int64) error {
	switch result := response.(type) {
	case *pb.SCUseTaticalFormation:
		original, ok := request.(*pb.CSUseTaticalFormation)
		current, err := s.currentTacticalFormation(g)
		if !ok || err != nil || result.GetId() != original.GetId() || current.GetId() != result.GetId() {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
		raw, err := proto.Marshal(current)
		if err != nil {
			return err
		}
		if p5KV(g, constUtil.P5TacticalUseReceiptKey) != strconv.FormatInt(stamp, 10)+":"+base64.RawStdEncoding.EncodeToString(raw) {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	case *pb.SCTaticalFormationChange:
		original, ok := request.(*pb.CSTaticalFormationChange)
		current, err := s.currentTacticalFormation(g)
		if !ok || err != nil || current.GetId() != original.GetId() || !proto.Equal(current, result.GetFormation()) {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	}
	return nil
}
