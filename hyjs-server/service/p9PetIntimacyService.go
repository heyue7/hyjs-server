package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
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

// InitializeP9PetIntimacy publishes explicit compatibility rules without replacing operator edits.
func (s *GameService) InitializeP9PetIntimacy(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P9PetIntimacyRule{}, &db.P9PetTalentAllocation{}, &db.P9PetDeferredConsumption{}); err != nil {
		return err
	}
	for _, group := range []struct{ quality, first int64 }{{23, 1}, {20, 11}, {18, 21}} {
		pool := []string{}
		for index := group.first; index < group.first+10; index++ {
			key := "PetTalent_" + strconv.FormatInt(index, 10)
			for level := 0; level <= 4; level++ {
				row := s.p9TablePair("PetTalent", "petTalentId", key, "talentLevel", strconv.Itoa(level))
				if row == nil {
					return p5Error()
				}
			}
			pool = append(pool, key)
		}
		encoded, err := json.Marshal(pool)
		if err != nil {
			return err
		}
		rule := db.P9PetIntimacyRule{Quality: group.quality, Enabled: true, Version: 1, Pool: encoded, Source: constUtil.P9IntimacyCompatibilitySource + constUtil.P9IntimacyTerminalSource}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error; err != nil {
			return err
		}
	}
	if err := s.initializeP9Hudong(tx); err != nil {
		return err
	}
	s.P9IntimacyReady = true
	return nil
}

// p9AllocateIntimacy chooses only newly unlocked properties under the locked profile transaction.
func (s *GameService) p9AllocateIntimacy(tx *gorm.DB, id int64, pet *pb.Pet, unlocked int) error {
	if unlocked <= len(pet.GetRandPropList()) {
		return nil
	}
	quality := int64(tableNumber(s.table("Pet", "id", pet.GetId()), "quality"))
	maximum := int(tableNumber(s.p9TablePair("PetIntimacy", "quality", strconv.FormatInt(quality, 10), "intimacyLevel", "7"), "additionalTalent"))
	if unlocked < 1 || unlocked > maximum || maximum > 4 {
		return p5Error()
	}
	var rule db.P9PetIntimacyRule
	if err := tx.First(&rule, "quality=?", quality).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(constUtil.P9IntimacyPolicyUnavailable)
		}
		return err
	}
	pool := []string{}
	if !rule.Enabled {
		return errors.New(constUtil.P9IntimacyPolicyUnavailable)
	}
	if rule.Version < 1 || rule.Source == "" || json.Unmarshal(rule.Pool, &pool) != nil || len(pool) < maximum+1 || len(pool) > 33 {
		return errors.New("亲密随机属性策略未发布或无效")
	}
	seen := map[string]bool{}
	for _, key := range pool {
		if seen[key] {
			return p5Error()
		}
		seen[key] = true
		for level := 0; level <= 4; level++ {
			row := s.p9TablePair("PetTalent", "petTalentId", key, "talentLevel", strconv.Itoa(level))
			if row == nil {
				return p5Error()
			}
			for _, field := range []string{"talent", "talentGrow", "talentConstGrow"} {
				if err := p9AddProperties(map[string]float64{}, row[field], 1); err != nil {
					return err
				}
			}
		}
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	hash := p6Digest(encoded)
	var allocation db.P9PetTalentAllocation
	err = tx.First(&allocation, "guid=?", pet.GetGuid()).Error
	if err == nil {
		previous := []string{}
		if allocation.PlayerID != id || allocation.PolicyHash != hash || allocation.Source == "" || json.Unmarshal(allocation.Draws, &previous) != nil || !equalP9Strings(previous, pet.GetRandPropList()) {
			return errors.New("精灵已分配属性与冻结策略冲突")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	excluded := map[string]bool{pet.GetInitProp(): true}
	for _, key := range pet.GetRandPropList() {
		// Imported properties keep their existing order and duplicates. The
		// published distribution constrains only choices made by this allocation.
		if s.p9TablePair("PetTalent", "petTalentId", key, "talentLevel", "0") == nil {
			return p5Error()
		}
		excluded[key] = true
	}
	available := []string{}
	for _, key := range pool {
		if !excluded[key] {
			available = append(available, key)
		}
	}
	for len(pet.GetRandPropList()) < unlocked {
		if len(available) == 0 {
			return p5Error()
		}
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(available))))
		if err != nil {
			return err
		}
		chosen := int(index.Int64())
		pet.RandPropList = append(pet.RandPropList, available[chosen])
		available = append(available[:chosen], available[chosen+1:]...)
	}
	draws, err := json.Marshal(pet.GetRandPropList())
	if err != nil {
		return err
	}
	allocation = db.P9PetTalentAllocation{GUID: pet.GetGuid(), PlayerID: id, PolicyHash: hash, Draws: draws, Source: rule.Source}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "guid"}}, DoUpdates: clause.AssignmentColumns([]string{"draws"})}).Create(&allocation).Error
}

func equalP9Strings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func (s *GameService) p9PetEat(ctx context.Context, id int64, b *pb.CSPetEat, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetEat{}
	return s.p9NextMutation(ctx, id, "pet-eat", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetGuid())
		if err != nil {
			return err
		}
		if err = s.p9DrainDeferredIntimacy(tx, id, g, pet); err != nil {
			return err
		}
		if len(b.GetCost()) < 1 || len(b.GetCost()) > 20 || pet.GetEat().GetExp() < 0 || pet.GetEat().GetLevel() < 0 {
			return p5Error()
		}
		petRow := s.table("Pet", "id", pet.GetId())
		if petRow == nil {
			return p5Error()
		}
		seen := map[string]bool{}
		var gain int64
		minimumGain := int64(math.MaxInt32)
		fees := []string{}
		for _, cost := range b.GetCost() {
			row := s.table("Item", "itemId", cost.GetId())
			if row == nil || row["category"] != "9" || cost.GetGuid() != "" || cost.GetNum() < 1 || cost.GetNum() > 1000 || seen[cost.GetId()] {
				return p5Error()
			}
			seen[cost.GetId()] = true
			ratio := int64(tableNumber(s.table("GameConfigure", "StaticName", "PetDifferentNatureIntimacyProportion"), "NameValue"))
			if strings.Contains(row["nature"], petRow["nature"]) {
				ratio = int64(tableNumber(s.table("GameConfigure", "StaticName", "PetSameNatureIntimacyProportion"), "NameValue"))
			}
			base := int64(tableNumber(row, "exp"))
			if base <= 0 || ratio < 1 || ratio > 10 || base > math.MaxInt32/int64(cost.GetNum())/ratio {
				return p5Error()
			}
			add := base * int64(cost.GetNum()) * ratio
			if base*ratio < minimumGain {
				minimumGain = base * ratio
			}
			if gain > math.MaxInt32-add {
				return p5Error()
			}
			gain += add
			fees = append(fees, cost.GetId()+","+strconv.Itoa(int(cost.GetNum())))
		}
		total := int64(pet.GetEat().GetExp()) + gain
		if total > math.MaxInt32 {
			return p5Error()
		}
		level := pet.GetEat().GetLevel()
		maximum := int32(tableNumber(s.table("GameConfigure", "StaticName", "PetMaxIntimacyLevel"), "NameValue"))
		if maximum != 7 || level >= maximum {
			return errors.New("精灵亲密度已经达到原上限")
		}
		for {
			row := s.p9TablePair("PetIntimacy", "quality", petRow["quality"], "intimacyLevel", strconv.Itoa(int(level)))
			need := int64(tableNumber(row, "intimacy"))
			if row == nil || need < 1 {
				return p5Error()
			}
			if total < need {
				break
			}
			next := s.p9TablePair("PetIntimacy", "quality", petRow["quality"], "intimacyLevel", strconv.Itoa(int(level+1)))
			if next == nil {
				return p5Error()
			}
			unlocked := int(tableNumber(next, "additionalTalent"))
			if unlocked < 0 {
				return p5Error()
			}
			if unlocked > len(pet.GetRandPropList()) {
				if err = s.p9AllocateIntimacy(tx, id, pet, unlocked); err != nil {
					return err
				}
			}
			total -= need
			level++
			if level >= maximum {
				if !s.P9IntimacyReady {
					return errors.New("亲密末层兼容策略未初始化")
				}
				if total >= minimumGain {
					return errors.New("亲密最高级食品超额超过本地末层兼容范围")
				}
				total = 0
				break
			}
		}
		costs, err := s.growthCost(p, g, strings.Join(fees, ";"), 1)
		if err != nil {
			return err
		}
		pet.Eat.Level = proto.Int32(level)
		pet.Eat.Exp = proto.Int32(int32(total))
		if err = p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		if err = p9Invest(tx, id, pet.GetGuid(), costs, nil); err != nil {
			return err
		}
		out.Pet = proto.Clone(pet).(*pb.Pet)
		out.Cost = costs
		return s.p9RecordCosts(tx, id, "p9:eat:"+strconv.FormatInt(stamp, 10), costs, LocalPlayNow(ctx))
	})
}

func (s *GameService) p9PetHudongShow(ctx context.Context, id int64, b *pb.CSPetHudongShow) ([]proto.Message, error) {
	out := &pb.SCPetHudongShow{}
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetGuid())
		if err != nil {
			return err
		}
		if err = s.p9DrainDeferredIntimacy(tx, id, g, pet); err != nil {
			return err
		}
		if s.P9IntimacyReady {
			prior := proto.Clone(pet).(*pb.Pet)
			if _, _, err := s.p9HudongState(tx, id, pet, time.Now()); err != nil {
				if err.Error() != constUtil.P9HudongDisabled {
					return err
				}
			} else if !proto.Equal(prior, pet) {
				// MySQL reports zero affected rows for an unchanged projection.
				// Persist only a real mood/window change; repeated Show remains a query.
				if err := p9SavePet(tx, id, g, pet); err != nil {
					return err
				}
			}
		}
		out.Pet = proto.Clone(pet).(*pb.Pet)
		power, err := s.formationFightPower(p, g)
		if err != nil {
			return err
		}
		p.FightPower = proto.Int32(power)
		return nil
	})
	return []proto.Message{out}, err
}

func (s *GameService) DispatchP9PetIntimacy(ctx context.Context, id int64, b proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request := b.(type) {
	case *pb.CSPetEat:
		out, err := s.p9PetEat(ctx, id, request, stamp)
		return out, true, err
	case *pb.CSPetHudongShow:
		out, err := s.p9PetHudongShow(ctx, id, request)
		return out, true, err
	case *pb.CSPetHudong:
		out, err := s.p9PetHudongAt(ctx, id, request, stamp, time.Now())
		return out, true, err
	default:
		return nil, false, nil
	}
}
