package service

import (
	"context"
	"encoding/json"
	"errors"
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

type P9CopyIntimacyFact struct {
	GUID     string `json:"guid"`
	Earned   int32  `json:"earned"`
	Applied  int32  `json:"applied"`
	Deferred int32  `json:"deferred"`
	Capped   int32  `json:"capped"`
}

func (s *GameService) InitializeP9CopyIntimacy(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P9PetCopyGrant{})
}

// p9CopyIntimacyGain applies verified growth and retains every earned point that
// would require an unverified new random attribute. It never generates attributes.
func (s *GameService) p9CopyIntimacyGain(pet *pb.Pet, earned int32) (*pb.Pet, P9CopyIntimacyFact, error) {
	return s.p9CopyIntimacyGainAllocated(pet, earned, nil)
}

func (s *GameService) p9CopyIntimacyGainAllocated(pet *pb.Pet, earned int32, allocate func(*pb.Pet, int) error) (*pb.Pet, P9CopyIntimacyFact, error) {
	fact := P9CopyIntimacyFact{GUID: pet.GetGuid(), Earned: earned}
	clone := proto.Clone(pet).(*pb.Pet)
	row := s.table("Pet", "id", pet.GetId())
	maximum := int32(tableNumber(s.table("GameConfigure", "StaticName", "PetMaxIntimacyLevel"), "NameValue"))
	if row == nil || maximum != 7 || earned < 1 || earned > 2 || clone.GetEat() == nil || clone.GetEat().GetLevel() < 0 || clone.GetEat().GetLevel() > maximum || clone.GetEat().GetExp() < 0 {
		return nil, fact, p5Error()
	}
	if _, _, err := s.p9PetProperties(clone); err != nil {
		return nil, fact, err
	}
	remaining := earned
	for remaining > 0 {
		level := clone.GetEat().GetLevel()
		if level == maximum {
			fact.Capped += remaining
			break
		}
		current := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(level)))
		next := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(level+1)))
		need := int32(tableNumber(current, "intimacy"))
		unlocked := int(tableNumber(next, "additionalTalent"))
		if current == nil || next == nil || need < 1 || clone.GetEat().GetExp() >= need || unlocked < 0 {
			return nil, fact, p5Error()
		}
		room := need - clone.GetEat().GetExp()
		if remaining < room {
			clone.Eat.Exp = proto.Int32(clone.GetEat().GetExp() + remaining)
			fact.Applied += remaining
			remaining = 0
			break
		}
		if unlocked > len(clone.GetRandPropList()) {
			if allocate != nil {
				if err := allocate(clone, unlocked); err != nil {
					if err.Error() != constUtil.P9IntimacyPolicyUnavailable {
						return nil, fact, err
					}
				}
			}
			if unlocked > len(clone.GetRandPropList()) {
				allowed := room - 1
				if allowed > 0 {
					clone.Eat.Exp = proto.Int32(clone.GetEat().GetExp() + allowed)
					fact.Applied += allowed
					remaining -= allowed
				}
				fact.Deferred += remaining
				remaining = 0
				break
			}
		}
		clone.Eat.Level = proto.Int32(level + 1)
		clone.Eat.Exp = proto.Int32(0)
		fact.Applied += room
		remaining -= room
	}
	if fact.Earned != fact.Applied+fact.Deferred+fact.Capped {
		return nil, fact, p5Error()
	}
	return clone, fact, nil
}

// p9DrainDeferredIntimacy preserves old frozen responses and journals only the unconsumed award.
func (s *GameService) p9DrainDeferredIntimacy(tx *gorm.DB, id int64, g *model.GameState, pet *pb.Pet) error {
	if !s.P9IntimacyReady {
		return nil
	}
	// Frozen in-flight teams retain their properties; pending credit can be applied after returning.
	if g.Run != nil && !g.Run.Complete || g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled {
		return nil
	}
	if tx.Migrator().HasTable(&db.DailyCopyFight{}) {
		if err := s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
			return nil
		}
	}
	var rows []db.P9PetCopyGrant
	if err := tx.Where("player_id=?", id).Order("created_at, source_key").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		facts := []P9CopyIntimacyFact{}
		if row.Source == "" || row.CompletedCount != 1 || json.Unmarshal(row.Facts, &facts) != nil {
			return p5Error()
		}
		seen := map[string]bool{}
		for _, fact := range facts {
			if fact.GUID == "" || seen[fact.GUID] || fact.Earned != 2 || fact.Applied < 0 || fact.Deferred < 0 || fact.Capped < 0 || fact.Applied+fact.Deferred+fact.Capped != fact.Earned {
				return p5Error()
			}
			seen[fact.GUID] = true
			if fact.GUID != pet.GetGuid() || fact.Deferred == 0 {
				continue
			}
			var receipt db.P9PetDeferredConsumption
			err := tx.First(&receipt, "player_id=? AND source_key=? AND guid=?", id, row.SourceKey, pet.GetGuid()).Error
			if err == nil {
				if receipt.Applied < 0 || receipt.Capped < 0 || receipt.Applied+receipt.Capped != int64(fact.Deferred) || receipt.PolicySource == "" {
					return p5Error()
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			grown, consumed, err := s.p9CopyIntimacyGainAllocated(pet, fact.Deferred, func(candidate *pb.Pet, count int) error { return s.p9AllocateIntimacy(tx, id, candidate, count) })
			if err != nil {
				return err
			}
			if consumed.Deferred != 0 {
				return nil
			}
			if err = p9SavePet(tx, id, g, grown); err != nil {
				return err
			}
			proto.Reset(pet)
			proto.Merge(pet, grown)
			receipt = db.P9PetDeferredConsumption{PlayerID: id, SourceKey: row.SourceKey, GUID: pet.GetGuid(), Applied: int64(consumed.Applied), Capped: int64(consumed.Capped), PolicySource: constUtil.P9IntimacyCompatibilitySource}
			if err = tx.Create(&receipt).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// ApplyP9CopyIntimacy runs inside the caller's locked profile settlement. Root
// hooks call only successful completed story missions or paid original sweeps.
// The original ten-sweep UI issues ten distinct single requests; each receipt
// freezes one actual completion and earns the original two points.
// intermediate tasks/losses have no verified award rule and remain outside it.
func (s *GameService) ApplyP9CopyIntimacy(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, source string) ([]*pb.PetAddIntimacy, error) {
	if tx == nil || p == nil || g == nil || p.GetPlayerId() != id || source == "copy:" || source == "sweep:" || len(source) > 191 || strings.TrimSpace(source) != source || strings.ContainsAny(source, "\r\n\x00") || (!strings.HasPrefix(source, "copy:") && !strings.HasPrefix(source, "sweep:")) {
		return nil, p5Error()
	}
	var saved db.P9PetCopyGrant
	err := tx.First(&saved, "player_id=? AND source_key=?", id, source).Error
	if err == nil {
		envelope := &pb.SCCopySweep{}
		if saved.Source == "" || saved.CompletedCount != 1 {
			return nil, p5Error()
		}
		if err = proto.Unmarshal(saved.Response, envelope); err != nil {
			return nil, err
		}
		return envelope.GetPetAdds(), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	earned := int32(tableNumber(s.table("GameConfigure", "StaticName", "PetCopyIntimacy"), "NameValue"))
	if earned != 2 {
		return nil, p5Error()
	}
	infos := []*pb.PetAddIntimacy{}
	facts := []P9CopyIntimacyFact{}
	seen := map[string]bool{}
	for _, camp := range g.Formation {
		if formationHero(camp) == nil {
			continue
		}
		for _, thing := range camp.GetThings() {
			if thing.GetPos() != 7 || seen[thing.GetGuid()] {
				continue
			}
			seen[thing.GetGuid()] = true
			pet, err := p9OwnedPet(tx, id, thing.GetGuid())
			if err != nil {
				return nil, err
			}
			if pet.GetId() != thing.GetId() {
				return nil, p5Error()
			}
			var allocate func(*pb.Pet, int) error
			if s.P9IntimacyReady {
				allocate = func(candidate *pb.Pet, count int) error { return s.p9AllocateIntimacy(tx, id, candidate, count) }
			}
			grown, fact, err := s.p9CopyIntimacyGainAllocated(pet, earned, allocate)
			if err != nil {
				return nil, err
			}
			if fact.Applied > 0 {
				if err = p9SavePet(tx, id, g, grown); err != nil {
					return nil, err
				}
				infos = append(infos, &pb.PetAddIntimacy{Guid: grown.Guid, AddExp: proto.Int32(fact.Applied), CurExp: grown.GetEat().Exp, Pet: proto.Clone(grown).(*pb.Pet)})
			}
			facts = append(facts, fact)
		}
	}
	if len(infos) > 0 {
		power, err := s.formationFightPower(p, g)
		if err != nil {
			return nil, err
		}
		p.FightPower = proto.Int32(power)
	}
	data, err := proto.Marshal(&pb.SCCopySweep{PetAdds: infos})
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = []byte{}
	}
	frozen, err := json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	saved = db.P9PetCopyGrant{PlayerID: id, SourceKey: source, CompletedCount: 1, Response: data, Facts: frozen, CreatedAt: time.Now().UnixMilli(), Source: "GameConfigure.PetCopyIntimacy=2；原SCCopyFightEnd13/SCCopySweep5；已完成成功故事副本/实际付费扫荡；未知随机解锁点数完整留账不阻断通关"}
	if err = tx.Create(&saved).Error; err != nil {
		return nil, err
	}
	return infos, nil
}
