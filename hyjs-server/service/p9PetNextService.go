package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP9PetNext(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P9PetInvestment{})
}

func p9ProjectPet(g *model.GameState, pet *pb.Pet) error {
	data, err := proto.Marshal(pet)
	if err != nil {
		return err
	}
	p5SetKV(g, "p9.pet:"+pet.GetGuid(), base64.StdEncoding.EncodeToString(data))
	return nil
}

func p9OwnedPet(tx *gorm.DB, id int64, guid string) (*pb.Pet, error) {
	if guid == "" || len(guid) > 64 {
		return nil, p5Error()
	}
	var row db.P9PetInstance
	if err := tx.First(&row, "player_id=? AND guid=?", id, guid).Error; err != nil {
		return nil, err
	}
	pet := &pb.Pet{}
	if err := proto.Unmarshal(row.Data, pet); err != nil {
		return nil, err
	}
	if row.Source == "" || pet.GetGuid() != guid || pet.GetId() != row.PetID || pet.GetEat() == nil || pet.GetBreached() == nil || pet.GetLevel() < 1 || pet.GetExp() < 0 {
		return nil, p5Error()
	}
	return pet, nil
}

func p9SavePet(tx *gorm.DB, id int64, g *model.GameState, pet *pb.Pet) error {
	data, err := proto.Marshal(pet)
	if err != nil {
		return err
	}
	result := tx.Model(&db.P9PetInstance{}).Where("player_id=? AND guid=?", id, pet.GetGuid()).Update("data", data)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return p5Error()
	}
	return p9ProjectPet(g, pet)
}

func p9Invest(tx *gorm.DB, id int64, guid string, costs []*pb.BaseResource, donors []*pb.Pet) error {
	var row db.P9PetInvestment
	err := tx.First(&row, "guid=?", guid).Error
	pool := map[string]int64{}
	prior := []*pb.Pet{}
	if err == nil {
		if row.PlayerID != id || row.Source == "" {
			return p5Error()
		}
		if err = json.Unmarshal(row.Costs, &pool); err != nil {
			return err
		}
		if err = json.Unmarshal(row.Donors, &prior); err != nil {
			return err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		row = db.P9PetInvestment{GUID: guid, PlayerID: id, Source: constUtil.P9PetSource + "；实际成长成本与原同ID基础实例捐献快照"}
	} else {
		return err
	}
	for _, cost := range costs {
		if cost.GetGuid() != "" {
			continue
		}
		if cost.GetNum() < 1 || pool[cost.GetId()] > math.MaxInt64-int64(cost.GetNum()) {
			return p5Error()
		}
		pool[cost.GetId()] += int64(cost.GetNum())
	}
	row.Costs, err = json.Marshal(pool)
	if err != nil {
		return err
	}
	prior = append(prior, donors...)
	row.Donors, err = json.Marshal(prior)
	if err != nil {
		return err
	}
	return tx.Save(&row).Error
}

func p9Guarded(g *model.GameState, guid string) bool {
	for _, slot := range g.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetPos() == 7 && thing.GetGuid() == guid {
				return true
			}
		}
	}
	return false
}

func (s *GameService) p9GuardOn(ctx context.Context, id int64, b *pb.CSFormationOn, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFormationOn{}
	return s.p9NextMutation(ctx, id, "pet-guard-on", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetGuid())
		if err != nil || pet.GetId() != b.GetId() {
			return p5Error()
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
				target = slot
			}
			for _, thing := range slot.GetThings() {
				if thing.GetPos() == 7 && thing.GetGuid() == pet.GetGuid() && slot.GetFormatPos() != b.GetFormatPos() {
					return errors.New("同一精灵实例已经守护其他队员")
				}
			}
		}
		if target == nil || formationHero(target) == nil {
			return errors.New("精灵只能守护已有主战队员")
		}
		remaining := []*pb.FormationThing{}
		for _, thing := range target.GetThings() {
			if thing.GetPos() != 7 {
				remaining = append(remaining, thing)
			}
		}
		target.Things = append(remaining, &pb.FormationThing{Id: proto.String(pet.GetId()), Guid: proto.String(pet.GetGuid()), Pos: proto.Int32(7)})
		if err = p9ProjectPet(g, pet); err != nil {
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

func (s *GameService) p9GuardOff(ctx context.Context, id int64, b *pb.CSFormationOff, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFormationOff{}
	return s.p9NextMutation(ctx, id, "pet-guard-off", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetGuid())
		if err != nil || pet.GetId() != b.GetId() {
			return p5Error()
		}
		if err = s.PrepareFormation(g); err != nil {
			return err
		}
		if err = s.formationPosition(p, g, b.GetFormatPos()); err != nil {
			return err
		}
		found := false
		for _, slot := range g.Formation {
			if slot.GetFormatPos() != b.GetFormatPos() {
				continue
			}
			remaining := []*pb.FormationThing{}
			for _, thing := range slot.GetThings() {
				if thing.GetPos() == 7 && thing.GetGuid() == b.GetGuid() {
					found = true
					continue
				}
				remaining = append(remaining, thing)
			}
			slot.Things = remaining
		}
		if !found {
			return p5Error()
		}
		power, err := s.formationFightPower(p, g)
		if err != nil {
			return err
		}
		out.Fightpower = proto.Int32(power)
		return nil
	})
}

func (s *GameService) p9Material(pet *pb.Pet) map[string]string {
	for _, row := range s.Tables["PetTalentMaterial"] {
		if row["petId"] == pet.GetId() && row["talentLevel"] == strconv.Itoa(int(pet.GetBreached().GetLevel())) {
			return row
		}
	}
	return nil
}

func (s *GameService) p9LevelExp(pet *pb.Pet, level int32) (int64, error) {
	quality := s.table("Pet", "id", pet.GetId())["quality"]
	row := s.p9TablePair("PetExp", "quality", quality, "level", strconv.Itoa(int(level)))
	if row == nil {
		return 0, p5Error()
	}
	value, err := strconv.ParseInt(row["exp"], 10, 64)
	if err != nil || value < 0 {
		return 0, p5Error()
	}
	return value, nil
}

func (s *GameService) p9PetLevelup(ctx context.Context, id int64, b *pb.CSPetLevelup, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetLevelup{}
	return s.p9NextMutation(ctx, id, "pet-level-up", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetGuid())
		if err != nil {
			return err
		}
		if len(b.GetCost()) < 1 || len(b.GetCost()) > 20 {
			return p5Error()
		}
		material := s.p9Material(pet)
		if material == nil {
			return p5Error()
		}
		cap := int32(tableNumber(material, "maxPetLevel"))
		if cap > p.GetLevel() {
			cap = p.GetLevel()
		}
		if cap < pet.GetLevel() {
			return p5Error()
		}
		var gain int64
		var smallestMaterialExp int64
		seen := map[string]bool{}
		costs := []*pb.BaseResource{}
		for _, cost := range b.GetCost() {
			row := s.table("Item", "itemId", cost.GetId())
			value, err := strconv.ParseInt(row["exp"], 10, 32)
			if err != nil || row["category"] != "8" || value < 1 || cost.GetNum() < 1 || cost.GetGuid() != "" || seen[cost.GetId()] || int64(cost.GetNum()) > math.MaxInt32/value {
				return p5Error()
			}
			seen[cost.GetId()] = true
			add := value * int64(cost.GetNum())
			if gain > math.MaxInt32-add {
				return p5Error()
			}
			gain += add
			if smallestMaterialExp == 0 || value < smallestMaterialExp {
				smallestMaterialExp = value
			}
			costs = append(costs, proto.Clone(cost).(*pb.BaseResource))
		}
		// 原PetExp最末行的exp=1，材料最小100，部分
		// 品质累计经验尾数50，不能要求材料数恰好整除末阶剩余经验。
		// 具名兼容策略只容纳最小所选材料的最后一个余量，仍全额扣费。
		var originalMaximum int32
		quality := s.table("Pet", "id", pet.GetId())["quality"]
		for _, row := range s.Tables["PetExp"] {
			if row["quality"] == quality && int32(tableNumber(row, "level")) > originalMaximum {
				originalMaximum = int32(tableNumber(row, "level"))
			}
		}
		terminal := cap == originalMaximum && originalMaximum > 0
		if terminal && pet.GetLevel() >= cap {
			return errors.New("精灵已达到原品质最高等级")
		}
		room := -pet.GetExp()
		for level := pet.GetLevel(); level <= cap; level++ {
			if terminal && level == cap {
				break
			}
			need, err := s.p9LevelExp(pet, level)
			if err != nil {
				return err
			}
			if need == 0 {
				break
			}
			room += need
		}
		allowed := room
		if terminal && smallestMaterialExp > 0 {
			allowed += smallestMaterialExp - 1
		}
		if gain > allowed || room <= 0 {
			return errors.New("精灵经验超过原等级或突破上限可容纳经验")
		}
		coinValue := tableNumber(s.table("GameConfigure", "StaticName", "petLevelUpCostCoinProportion"), "NameValue")
		coin := float64(gain) * coinValue
		if coinValue <= 0 || coin > math.MaxInt32 || coin != math.Trunc(coin) {
			return p5Error()
		}
		costs = append(costs, &pb.BaseResource{Id: proto.String("coin"), Num: proto.Int32(int32(coin))})
		for _, cost := range costs {
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
		}
		pet.Exp = proto.Int64(pet.GetExp() + gain)
		for pet.GetLevel() < cap {
			need, err := s.p9LevelExp(pet, pet.GetLevel())
			if err != nil || need <= 0 {
				return p5Error()
			}
			if pet.GetExp() < need {
				break
			}
			pet.Exp = proto.Int64(pet.GetExp() - need)
			pet.Level = proto.Int32(pet.GetLevel() + 1)
		}
		if terminal && pet.GetLevel() == cap {
			pet.Exp = proto.Int64(0)
		}
		if err = p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		if err = p9Invest(tx, id, pet.GetGuid(), costs, nil); err != nil {
			return err
		}
		if terminal && gain > room {
			var investment db.P9PetInvestment
			if err = tx.First(&investment, "guid=? AND player_id=?", pet.GetGuid(), id).Error; err != nil {
				return err
			}
			if err = tx.Model(&investment).Update("source", investment.Source+"；"+constUtil.P9PetTerminalRoundingSource).Error; err != nil {
				return err
			}
		}
		out.Pet = proto.Clone(pet).(*pb.Pet)
		out.Cost = costs
		return s.p9RecordCosts(tx, id, "p9:level:"+strconv.FormatInt(stamp, 10), costs, LocalPlayNow(ctx))
	})
}

func p9BasicDonor(pet *pb.Pet) bool {
	if pet.GetLevel() != 1 || pet.GetExp() != 0 || pet.GetBreached().GetLevel() != 0 || pet.GetEat().GetLevel() != 0 || pet.GetEat().GetExp() != 0 {
		return false
	}
	for _, equip := range pet.GetPetEquipList() {
		if equip.GetStrengthLevel() != 0 || equip.GetAdvanceLevel() != 0 || equip.GetAwakeLevel() != 0 {
			return false
		}
	}
	return true
}

func (s *GameService) p9PetBreach(ctx context.Context, id int64, b *pb.CSPetBreached, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetBreached{}
	return s.p9NextMutation(ctx, id, "pet-breach", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetGuid())
		if err != nil {
			return err
		}
		material := s.p9Material(pet)
		max := int32(tableNumber(s.table("GameConfigure", "StaticName", "PetBreachedMaxLevel"), "NameValue"))
		if len(b.GetCost()) != 0 || material == nil || pet.GetBreached().GetLevel() >= max || pet.GetLevel() < int32(tableNumber(material, "petLevel")) {
			return p5Error()
		}
		expected, err := p3Resources(material["cost"])
		if err != nil {
			return err
		}
		costs := []*pb.BaseResource{}
		donors := []*pb.Pet{}
		for _, cost := range expected {
			if strings.HasPrefix(cost.GetId(), "pet_") {
				if cost.GetId() != pet.GetId() {
					return p5Error()
				}
				owned, err := P9PetList(tx, id)
				if err != nil {
					return err
				}
				candidates := []*pb.Pet{}
				for _, other := range owned {
					if other.GetGuid() != pet.GetGuid() && other.GetId() == cost.GetId() && !p9Guarded(g, other.GetGuid()) && p9BasicDonor(other) {
						candidates = append(candidates, other)
					}
				}
				sort.Slice(candidates, func(i, j int) bool { return candidates[i].GetGuid() < candidates[j].GetGuid() })
				if len(candidates) < int(cost.GetNum()) {
					return errors.New("同名未培养且未守护的精灵材料不足")
				}
				for _, donor := range candidates[:cost.GetNum()] {
					if err = tx.Delete(&db.P9PetInstance{}, "player_id=? AND guid=?", id, donor.GetGuid()).Error; err != nil {
						return err
					}
					p5SetKV(g, "p9.pet:"+donor.GetGuid(), "")
					donors = append(donors, donor)
					costs = append(costs, &pb.BaseResource{Id: proto.String(donor.GetId()), Guid: proto.String(donor.GetGuid()), Num: proto.Int32(1)})
				}
			} else {
				if err = deductPoolCost(p, g, cost); err != nil {
					return err
				}
				costs = append(costs, cost)
			}
		}
		pet.Breached.Level = proto.Int32(pet.GetBreached().GetLevel() + 1)
		if err = p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		if err = p9Invest(tx, id, pet.GetGuid(), costs, donors); err != nil {
			return err
		}
		out.Pet = proto.Clone(pet).(*pb.Pet)
		out.Cost = costs
		return s.p9RecordCosts(tx, id, "p9:breach:"+strconv.FormatInt(stamp, 10), costs, LocalPlayNow(ctx))
	})
}

func p9AddProperties(values map[string]float64, text string, multiplier float64) error {
	if text == "" {
		return nil
	}
	for _, spec := range strings.Split(text, ";") {
		pair := strings.Split(spec, ",")
		if len(pair) != 2 {
			return p5Error()
		}
		switch pair[0] {
		case "hp", "att", "pdef", "mdef", "hit", "dodge", "cri", "criFactor", "antiCri", "damageAdd", "damageReduce":
		default:
			return p5Error()
		}
		value, err := strconv.ParseFloat(pair[1], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return p5Error()
		}
		values[pair[0]] += value * multiplier
		if math.IsNaN(values[pair[0]]) || math.IsInf(values[pair[0]], 0) {
			return p5Error()
		}
	}
	return nil
}

func (s *GameService) p9PetProperties(pet *pb.Pet) (map[string]float64, []string, error) {
	values := map[string]float64{}
	talents := []string{}
	row := s.table("Pet", "id", pet.GetId())
	if row == nil || pet.GetLevel() < 1 || pet.GetBreached().GetLevel() < 0 || pet.GetBreached().GetLevel() > 4 {
		return nil, nil, p5Error()
	}
	init := s.p9TablePair("PetTalent", "petTalentId", pet.GetInitProp(), "talentLevel", strconv.Itoa(int(pet.GetBreached().GetLevel())))
	if init == nil {
		return nil, nil, p5Error()
	}
	for _, part := range []struct {
		key        string
		multiplier float64
	}{{"talent", 1}, {"talentConstGrow", 1}, {"talentGrow", float64(pet.GetLevel() - 1)}} {
		if err := p9AddProperties(values, init[part.key], part.multiplier); err != nil {
			return nil, nil, err
		}
	}
	intimacy := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(pet.GetEat().GetLevel())))
	unlocked := int(tableNumber(intimacy, "additionalTalent"))
	if intimacy == nil || unlocked < 0 || unlocked > len(pet.GetRandPropList()) {
		return nil, nil, errors.New("精灵亲密随机属性尚未恢复")
	}
	for _, id := range pet.GetRandPropList()[:unlocked] {
		extra := s.p9TablePair("PetTalent", "petTalentId", id, "talentLevel", strconv.Itoa(int(pet.GetBreached().GetLevel())))
		if extra == nil {
			return nil, nil, p5Error()
		}
		for _, part := range []struct {
			key        string
			multiplier float64
		}{{"talent", 1}, {"talentConstGrow", 1}, {"talentGrow", float64(pet.GetLevel() - 1)}} {
			if err := p9AddProperties(values, extra[part.key], part.multiplier); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, talent := range pet.GetTalentList() {
		if talent.GetTalentLevel() <= pet.GetBreached().GetLevel() {
			if s.table("Talent", "Id", talent.GetTalent()) == nil {
				return nil, nil, p5Error()
			}
			talents = append(talents, talent.GetTalent())
		}
	}
	for _, equip := range pet.GetPetEquipList() {
		base := s.table("PetEquip", "id", equip.GetEquipId())
		if base["petId"] != pet.GetId() || equip.GetPetGuid() != pet.GetGuid() {
			return nil, nil, p5Error()
		}
		if err := p9AddProperties(values, base["prop"], 1); err != nil {
			return nil, nil, err
		}
		for _, step := range []struct {
			table string
			level int32
		}{{"PetEquipStrength", equip.GetStrengthLevel()}, {"PetEquipAdvance", equip.GetAdvanceLevel()}, {"PetEquipAwake", equip.GetAwakeLevel()}} {
			if step.level == 0 {
				continue
			}
			advanced := s.p9TablePair(step.table, "equipId", equip.GetEquipId(), "level", strconv.Itoa(int(step.level)))
			if advanced == nil {
				return nil, nil, p5Error()
			}
			if err := p9AddProperties(values, advanced["prop"], 1); err != nil {
				return nil, nil, err
			}
		}
	}
	return values, talents, nil
}

// P9GuardProperties uses the transactionally published native snapshot and never
// opens a second DB connection inside a caller's profile transaction.
func (s *GameService) P9GuardProperties(g *model.GameState, slot *pb.FormationItem) (map[string]float64, []string, string, error) {
	for _, thing := range slot.GetThings() {
		if thing.GetPos() != 7 {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(p5KV(g, "p9.pet:"+thing.GetGuid()))
		if err != nil || len(raw) == 0 {
			return nil, nil, "", p5Error()
		}
		pet := &pb.Pet{}
		if err = proto.Unmarshal(raw, pet); err != nil {
			return nil, nil, "", err
		}
		if pet.GetGuid() != thing.GetGuid() || pet.GetId() != thing.GetId() {
			return nil, nil, "", p5Error()
		}
		values, talents, err := s.p9PetProperties(pet)
		return values, talents, pet.GetId(), err
	}
	return map[string]float64{}, nil, "", nil
}

func (s *GameService) DispatchP9PetNext(ctx context.Context, id int64, b proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var out []proto.Message
	var err error
	switch request := b.(type) {
	case *pb.CSFormationOn:
		if !strings.HasPrefix(request.GetId(), "pet_") {
			return nil, false, nil
		}
		out, err = s.p9GuardOn(ctx, id, request, stamp)
	case *pb.CSFormationOff:
		if !strings.HasPrefix(request.GetId(), "pet_") {
			return nil, false, nil
		}
		out, err = s.p9GuardOff(ctx, id, request, stamp)
	case *pb.CSPetLevelup:
		out, err = s.p9PetLevelup(ctx, id, request, stamp)
	case *pb.CSPetBreached:
		out, err = s.p9PetBreach(ctx, id, request, stamp)
	default:
		return nil, false, nil
	}
	return out, true, err
}

func (s *GameService) p9TablePair(name, key1, value1, key2, value2 string) map[string]string {
	for _, row := range s.Tables[name] {
		if row[key1] == value1 && row[key2] == value2 {
			return row
		}
	}
	return nil
}

func (s *GameService) p9NextMutation(ctx context.Context, id int64, op string, stamp int64, request, response proto.Message, apply func(*gorm.DB, *pb.Player, *model.GameState) error) ([]proto.Message, error) {
	return s.p9Mutation(ctx, id, op, stamp, request, response, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if g.Run != nil && !g.Run.Complete || g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled {
			return errors.New("战斗中不能修改精灵守护或成长")
		}
		if gameSchemaHasTable(tx, &db.DailyCopyFight{}) {
			if err := s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
				return err
			}
		}
		if err := apply(tx, p, g); err != nil {
			return err
		}
		power, err := s.formationFightPower(p, g)
		if err != nil {
			return err
		}
		p.FightPower = proto.Int32(power)
		return nil
	})
}

func (s *GameService) p9RecordCosts(tx *gorm.DB, id int64, source string, costs []*pb.BaseResource, at time.Time) error {
	ordinary := []*pb.BaseResource{}
	var donors int64
	for _, cost := range costs {
		if strings.HasPrefix(cost.GetId(), "pet_") {
			if cost.GetGuid() == "" || cost.GetNum() != 1 || s.table("Pet", "id", cost.GetId()) == nil {
				return p5Error()
			}
			donors++
		} else {
			ordinary = append(ordinary, cost)
		}
	}
	if err := s.RecordP1Cost(tx, id, source, ordinary, at); err != nil {
		return err
	}
	if donors > 0 {
		return s.RecordP1Event(tx, id, source, "pet_donor_consumed", donors, at)
	}
	return nil
}
