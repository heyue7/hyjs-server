package service

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
)

func P5HelperList(g *model.GameState) []*pb.FormationThing {
	result := []*pb.FormationThing{}
	for pos := int32(1); pos <= 6; pos++ {
		guid := p5KV(g, fmt.Sprintf("p5.helper:%d", pos))
		if guid == "" {
			continue
		}
		w, err := p5Warrior(g, guid)
		if err == nil {
			result = append(result, &pb.FormationThing{Id: proto.String(w.GetId()), Guid: proto.String(guid), Pos: proto.Int32(pos)})
		}
	}
	return result
}

func (s *GameService) p5HelperOn(ctx context.Context, id int64, b *pb.CSHelperOn, stamp int64) ([]proto.Message, error) {
	return s.p5Mutation(ctx, id, "helper-on", stamp, b, &pb.SCHelperOn{}, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if b.GetPos() < 1 || b.GetPos() > 6 {
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
func (s *GameService) p5HelperOff(ctx context.Context, id int64, b *pb.CSHelperOff, stamp int64) ([]proto.Message, error) {
	return s.p5Mutation(ctx, id, "helper-off", stamp, b, &pb.SCHelperOff{}, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if b.GetPos() < 1 || b.GetPos() > 6 {
			return p5Error()
		}
		p5SetKV(g, fmt.Sprintf("p5.helper:%d", b.GetPos()), "")
		return nil
	})
}
func (s *GameService) p5SkillOrder(ctx context.Context, id int64, b *pb.CSSetSkillOrder, stamp int64) ([]proto.Message, error) {
	return s.p5Mutation(ctx, id, "skill-order", stamp, b, &pb.SCSetSkillOrder{}, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		w, err := p5Warrior(g, b.GetHeroGuid())
		if err != nil {
			return err
		}
		row := s.table("Hero", "id", w.GetId())
		order := strings.Split(b.GetSkillOrder(), ",")
		if row == nil || len(b.GetHeroGuid()) > 128 || len(b.GetSkillOrder()) > 128 || len(order) < 2 || len(order) > 3 {
			return p5Error()
		}
		// Original SkillSetting sends two basic skills, or three after awakening.
		// Ultimate remains fixed at BattleHero index 2 and is never a reorder option.
		allowed := map[string]bool{row["SkillId1"]: true, row["SkillId2"]: true}
		if w.GetAwake().GetStage() >= 2 && w.GetAwakenSkill() != "" && row["AwakeSkillId"] != "" && row["IsOpenAwake"] == "1" {
			allowed[row["AwakeSkillId"]] = true
		}
		if len(allowed) != len(order) {
			return p5Error()
		}
		for _, skill := range order {
			if skill == "" || !allowed[skill] {
				return p5Error()
			}
			delete(allowed, skill)
		}
		before := proto.Clone(w).(*pb.Warrior)
		w.SkillOrder = proto.String(b.GetSkillOrder())
		if len(order) == 3 {
			_, encoded, err := s.P5BattleSkillTransport(w)
			if err != nil {
				return err
			}
			w.AwakenSkill = proto.String(encoded)
		}
		// 排序只调整已拥有技能的槽位，在同一实例账本推进指纹。
		return RecordP5Investment(g, w.GetGuid(), before, w, nil)
	})
}

// validateP5SkillOrderReceipt keeps the empty original sorting acknowledgement
// tied to its current owned hero and order. SkillSetting.MsgCallback updates the
// captured hero's local skill slots even though the response has no hero body.
func validateP5SkillOrderReceipt(g *model.GameState, request, response proto.Message) error {
	if _, ok := response.(*pb.SCSetSkillOrder); !ok {
		return nil
	}
	original, ok := request.(*pb.CSSetSkillOrder)
	if !ok {
		return p5Error()
	}
	current, err := p5Warrior(g, original.GetHeroGuid())
	if err != nil || current.GetSkillOrder() != original.GetSkillOrder() {
		return p5Error()
	}
	return nil
}

// P5BattleSkillOrder returns the three original base IDs; awakening is encoded
// separately by P5BattleSkillTransport to preserve the native constructor.
func (s *GameService) P5BattleSkillOrder(w *pb.Warrior) (string, error) {
	order, _, err := s.P5BattleSkillTransport(w)
	return order, err
}

// P5DynamicTalentIDs preserves actual equipped source IDs; current APK source columns are empty.
func (s *GameService) P5DynamicTalentIDs(g *model.GameState, w *pb.Warrior) ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, slot := range g.Formation {
		hero := formationHero(slot)
		if hero == nil || hero.GetGuid() != w.GetGuid() {
			continue
		}
		for _, thing := range slot.GetThings() {
			if thing.GetPos() == 0 {
				continue
			}
			for _, spec := range []struct {
				table, key string
				level      int32
			}{{"EquipSkill", "equipId", 0}, {"TreasureSkill", "treaId", 0}} {
				level := int32(-1)
				for _, equip := range g.Equipment {
					if spec.table == "EquipSkill" && equip.GetGuid() == thing.GetGuid() {
						level = equip.GetRefineLv()
					}
				}
				for _, treasure := range g.Treasures {
					if spec.table == "TreasureSkill" && treasure.GetGuid() == thing.GetGuid() {
						level = treasure.GetRefineLevel()
					}
				}
				if level < 0 {
					continue
				}
				for _, row := range s.Tables[spec.table] {
					if row[spec.key] != thing.GetId() || tableNumber(row, "refineLevel") > float64(level) {
						continue
					}
					id := row["buffId"]
					if id == "" {
						id = row["talentID"]
					}
					if id == "" {
						id = row["talentId"]
					}
					if id == "" {
						continue
					}
					if s.table("Talent", "Id", id) == nil || seen[id] {
						return nil, p5Error()
					}
					seen[id] = true
					result = append(result, id)
				}
			}
		}
	}
	return result, nil
}

// P5ActiveRelations uses only the subject's equipment/treasures, and the battle+helper hero set.
func (s *GameService) P5ActiveRelations(g *model.GameState, w *pb.Warrior) []map[string]string {
	available := map[string]bool{}
	for _, slot := range g.Formation {
		if h := formationHero(slot); h != nil {
			available[h.GetId()] = true
		}
	}
	for _, h := range P5HelperList(g) {
		available[h.GetId()] = true
	}
	subject := map[string]bool{}
	for _, slot := range g.Formation {
		h := formationHero(slot)
		if h == nil || h.GetGuid() != w.GetGuid() {
			continue
		}
		for _, thing := range slot.GetThings() {
			if thing.GetPos() != 0 {
				subject[thing.GetId()] = true
			}
		}
	}
	result := []map[string]string{}
	for _, row := range s.Tables["Yuan"] {
		if row["heroId"] != w.GetId() {
			continue
		}
		active := true
		conditions := 0
		for _, key := range []string{"hero0", "hero1", "hero2", "hero3"} {
			if target := row[key]; target != "" {
				conditions++
				active = active && available[target]
			}
		}
		for _, key := range []string{"equip0", "treaId0"} {
			if target := row[key]; target != "" {
				conditions++
				active = active && subject[target]
			}
		}
		if active && conditions > 0 {
			result = append(result, row)
		}
	}
	return result
}

// AddP5StaticBonus excludes the existing world-seed layer and every conditional combat Buff.
func (s *GameService) AddP5StaticBonus(p *pb.Player, g *model.GameState, w *pb.Warrior, values, percent map[string]float64) error {
	for _, row := range s.P5ActiveRelations(g, w) {
		percent["hp"] += tableNumber(row, "perhp") * .01
		percent["att"] += tableNumber(row, "peratt") * .01
		percent["pdef"] += tableNumber(row, "perdef") * .01
		percent["mdef"] += tableNumber(row, "perdef") * .01
	}
	gate := int32(tableNumber(s.table("GameConfigure", "StaticName", "FriendWishLevel"), "NameValue"))
	if gate > 0 && p.GetLevel() >= gate {
		helpers := P5HelperList(g)
		if len(helpers) == 6 {
			minLevel := int32(math.MaxInt32)
			for _, h := range helpers {
				hero, err := p5Warrior(g, h.GetGuid())
				if err != nil {
					return err
				}
				minLevel = min(minLevel, hero.GetLevel())
			}
			var selected map[string]string
			for _, row := range s.Tables["StrengthenMaster"] {
				if row["type"] == "5" && tableNumber(row, "strengthenLevel") <= float64(minLevel) && (selected == nil || tableNumber(row, "level") > tableNumber(selected, "level")) {
					selected = row
				}
			}
			if selected != nil {
				for _, key := range []string{"hp", "att", "pdef", "mdef"} {
					values[key] += tableNumber(selected, key)
				}
			}
		}
	}
	return s.p5TitleBonus(g, values)
}

func p5SeedID(row map[string]string) int32 {
	n, _ := strconv.ParseInt(row["id"], 10, 32)
	return int32(n)
}
