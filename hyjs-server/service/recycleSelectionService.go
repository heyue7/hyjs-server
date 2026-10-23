package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type recycleSelectionCandidate struct {
	guid    string
	quality int
}

// RecycleSelection locks the original profile and rechecks current authorization.
// Only detached snapshots are mutated while validating the original decomposition rules.
func (s *GameService) RecycleSelection(ctx context.Context, id int64, session []byte, kind string, limit int) (*model.RecycleSelectionResponse, error) {
	out := &model.RecycleSelectionResponse{GUIDs: []string{}}
	if limit < 1 || limit > constUtil.RecycleDecomposeMaxBatch || (kind != "hero" && kind != "equipment" && kind != "pet") {
		return out, model.NewErrParamsError()
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		if len(session) != 16 || subtle.ConstantTimeCompare(profile.Session, session) != 1 {
			return model.NewErrBizCode("登录已失效，请重新登录")
		}
		if profile.AccountID > 0 {
			account, err := AccountForProfile(tx, &profile)
			if err != nil {
				return err
			}
			if !account.Enabled || profile.AccountAuthRevision != account.AuthRevision {
				return model.NewErrBizCode("账号权限已变更，请重新登录")
			}
		}
		p, g, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		gate := int32(tableNumber(s.table("GameConfigure", "StaticName", "RecycleRoomLevel"), "NameValue"))
		if gate < 1 || p.GetLevel() < gate {
			return model.NewErrBizCode("角色等级尚未达到回收屋要求")
		}
		candidates := []recycleSelectionCandidate{}
		holds := map[string]bool{}
		var pets map[string]*pb.Pet
		var petConfig *p9MarketConfig
		var petCount int64
		switch kind {
		case "hero":
			var rows []db.P7ExpeditionHero
			if err := tx.Where("player_id=?", id).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				holds[row.GUID] = true
			}
			for _, w := range g.Warriors {
				q, _ := strconv.Atoi(s.table("Hero", "id", w.GetId())["quality"])
				if q == 12 || q == 13 || q == 18 {
					candidates = append(candidates, recycleSelectionCandidate{w.GetGuid(), q})
				}
			}
		case "equipment":
			for _, e := range g.Equipment {
				q, _ := strconv.Atoi(s.table("Equip", "id", e.GetEquipId())["quality"])
				if q == 12 || q == 13 {
					candidates = append(candidates, recycleSelectionCandidate{e.GetGuid(), q})
				}
			}
		case "pet":
			if g.Run != nil && !g.Run.Complete || g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled {
				return model.NewErrBizCode("战斗中不能修改精灵守护或成长")
			}
			if tx.Migrator().HasTable(&db.DailyCopyFight{}) {
				if err := s.ensureNoActiveDailyCopy(tx, id, time.Now()); err != nil {
					return err
				}
			}
			owned, err := P9PetList(tx, id)
			if err != nil {
				return err
			}
			pets = map[string]*pb.Pet{}
			if err := tx.Model(&db.P9PetInstance{}).Where("player_id=?", id).Count(&petCount).Error; err != nil {
				return err
			}
			var rule db.P9PetMarketRule
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&rule, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
				return err
			}
			petConfig = &p9MarketConfig{}
			if !rule.Enabled || strings.TrimSpace(rule.Source) == "" || json.Unmarshal(rule.Input, petConfig) != nil || s.validateP9MarketConfig(petConfig) != nil {
				return model.NewErrBizCode(constUtil.P9MarketError)
			}
			// Validate an existing same-week frozen catalogue without creating or refreshing it.
			var state db.P9PetMarketState
			stateErr := tx.First(&state, "player_id=?", id).Error
			if stateErr != nil && !errors.Is(stateErr, gorm.ErrRecordNotFound) {
				return stateErr
			}
			week := p9MarketWeek(LocalPlayAt(ctx, time.Now()))
			if stateErr == nil && state.WeekKey > week {
				return model.NewErrBizCode(constUtil.P9MarketError)
			}
			if stateErr == nil && state.WeekKey == week {
				var frozen p9MarketConfig
				if state.InputHash != p6Digest(state.Input) || json.Unmarshal(state.Input, &frozen) != nil || s.validateP9MarketConfig(&frozen) != nil || state.Generation < 1 || state.RefreshUsed < 0 || state.RefreshUsed > petConfig.RefreshCap {
					return model.NewErrBizCode(constUtil.P9MarketError)
				}
				if _, _, err := s.p9MarketInventory(&state, &frozen); err != nil {
					return err
				}
			}
			for _, pet := range owned {
				q, _ := strconv.Atoi(s.table("Pet", "id", pet.GetId())["quality"])
				if q < 23 && q > 0 && pet.GetLevel() == 1 && pet.GetBreached().GetLevel() == 0 && pet.GetEat().GetExp() == 0 {
					pets[pet.GetGuid()] = pet
					candidates = append(candidates, recycleSelectionCandidate{pet.GetGuid(), q})
				}
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].quality != candidates[j].quality {
				return candidates[i].quality < candidates[j].quality
			}
			return candidates[i].guid < candidates[j].guid
		})
		for _, candidate := range candidates {
			if len(out.GUIDs) >= limit {
				break
			}
			// Reject existing read-only guards before cloning a large inventory.
			// The detached trial still runs all original checks and refund simulation.
			if holds[candidate.guid] || P5GuardInstance(p, g, candidate.guid) != nil || kind == "pet" && p9Guarded(g, candidate.guid) {
				out.Skipped++
				continue
			}
			trialP, trialG, err := recycleSelectionClone(p, g)
			if err != nil {
				return err
			}
			eligible := false
			switch kind {
			case "hero":
				eligible = s.recycleSelectionHero(trialP, trialG, candidate.guid, holds) == nil
			case "equipment":
				eligible = s.recycleSelectionEquipment(trialP, trialG, candidate.guid) == nil
			case "pet":
				eligible, err = s.recycleSelectionPet(tx, id, trialP, trialG, pets[candidate.guid], petConfig, &petCount)
				if err != nil {
					return err
				}
			}
			if !eligible {
				out.Skipped++
				continue
			}
			out.GUIDs = append(out.GUIDs, candidate.guid)
			p, g = trialP, trialG
		}
		return nil
	})
	return out, err
}
func recycleSelectionClone(p *pb.Player, g *model.GameState) (*pb.Player, *model.GameState, error) {
	raw, err := json.Marshal(g)
	if err != nil {
		return nil, nil, err
	}
	game := &model.GameState{}
	if err = json.Unmarshal(raw, game); err != nil {
		return nil, nil, err
	}
	return proto.Clone(p).(*pb.Player), game, nil
}
func (s *GameService) recycleSelectionHero(p *pb.Player, g *model.GameState, guid string, holds map[string]bool) error {
	if holds[guid] {
		return p5Error()
	}
	if err := P5GuardInstance(p, g, guid); err != nil {
		return err
	}
	w, err := p5Warrior(g, guid)
	if err != nil {
		return err
	}
	class := s.table("Hero", "id", w.GetId())["heroClass"]
	if w.GetId() == "hero_039" || class == "99" || class == "100" || class == "101" {
		return p5Error()
	}
	if p5KV(g, "p5.invest:"+guid) != "" {
		if _, err = s.p5Refund(p, g, guid, w, true); err != nil {
			return err
		}
		w, err = p5Warrior(g, guid)
		if err != nil {
			return err
		}
	} else if p5WarriorProgressed(w) {
		return p5Error()
	}
	if p5WarriorProgressed(w) {
		return p5Error()
	}
	rewards, err := s.p5WarriorDecomposeReward(w.GetId(), 1)
	if err != nil {
		return err
	}
	remaining := g.Warriors[:0]
	for _, owned := range g.Warriors {
		if owned.GetGuid() != guid {
			remaining = append(remaining, owned)
		}
	}
	g.Warriors = remaining
	_, err = s.p5GrantTrackedRefund(p, g, rewards)
	return err
}
func (s *GameService) recycleSelectionEquipment(p *pb.Player, g *model.GameState, guid string) error {
	if err := P5GuardInstance(p, g, guid); err != nil {
		return err
	}
	e, err := s.equipmentRecord(g, guid)
	if err != nil {
		return err
	}
	if p5EquipmentProgressed(e) {
		if _, err = s.p5Refund(p, g, guid, e); err != nil {
			return err
		}
		e, err = s.equipmentRecord(g, guid)
		if err != nil || p5EquipmentProgressed(e) {
			return p5Error()
		}
	}
	if _, err = s.p8RecycleEquipmentGems(p, g, e); err != nil {
		return err
	}
	_, _, err = s.equipmentDecomposeApply(p, g, &pb.CSEquipDecompose{Guid: []string{guid}}, constUtil.EquipmentRefundBaseOnly)
	return err
}
func (s *GameService) recycleSelectionPet(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, pet *pb.Pet, cfg *p9MarketConfig, petCount *int64) (bool, error) {
	if p9Guarded(g, pet.GetGuid()) || P5GuardInstance(p, g, pet.GetGuid()) != nil {
		return false, nil
	}
	// Reload the owned row and investment from the same locked transaction.
	owned, err := p9OwnedPet(tx, id, pet.GetGuid())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == p5Error().Error() {
			return false, nil
		}
		return false, err
	}
	costs, donors, err := s.p9DecomposeRefund(tx, id, owned)
	if err != nil {
		if err.Error() == constUtil.P9PetResetInvestmentMissing || err.Error() == constUtil.P9PetResetInvestmentInvalid {
			return false, nil
		}
		return false, err
	}
	value := int64(cfg.Decompose[s.table("Pet", "id", owned.GetId())["quality"]])
	if value < 1 || p.GetCoinpet() < 0 || p.GetCoinpet() > math.MaxInt64-value {
		return false, nil
	}
	if len(donors) > 0 {
		// The actual refund creates fresh base pets, so validate its catalogue before selecting.
		row := s.table("Pet", "id", owned.GetId())
		baseFound := false
		for _, talent := range s.Tables["PetTalent"] {
			baseFound = baseFound || talent["petTalentId"] == row["defaultTalent"] && talent["talentLevel"] == "0"
		}
		if row["defaultTalent"] == "" || !baseFound {
			return false, nil
		}
		for _, talent := range s.Tables["PetTalentSkill"] {
			if talent["petId"] != owned.GetId() {
				continue
			}
			level, err := strconv.ParseInt(talent["talentLevel"], 10, 32)
			if err != nil || level < 1 || talent["talent"] == "" || talent["odd"] != "100" {
				return false, nil
			}
		}
	}
	if len(donors) > 0 && (p.GetPetBagSize() < 1 || *petCount-1+int64(len(donors)) > int64(p.GetPetBagSize())) {
		return false, nil
	}
	for key, n := range costs {
		if key == "coinpet" {
			if p.GetCoinpet() > math.MaxInt64-n-value {
				return false, nil
			}
			p.Coinpet = proto.Int64(p.GetCoinpet() + n)
			continue
		}
		if _, err := s.GrantFixedReward(p, g, key+","+strconv.FormatInt(n, 10)); err != nil {
			return false, nil
		}
	}
	p.Coinpet = proto.Int64(p.GetCoinpet() + value)
	*petCount += int64(len(donors)) - 1
	return true, nil
}
