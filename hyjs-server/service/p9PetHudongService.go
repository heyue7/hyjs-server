package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p9HudongPreference struct {
	Type   int32 `json:"type"`
	Weight int64 `json:"weight"`
}
type p9HudongConfig struct {
	WindowHours     int64                `json:"windowHours"`
	CooldownSeconds int64                `json:"cooldownSeconds"`
	RewardCalls     int32                `json:"rewardCalls"`
	MaximumCalls    int32                `json:"maximumCalls"`
	Experience      int32                `json:"experience"`
	Multiplier      int32                `json:"multiplier"`
	Preferences     []p9HudongPreference `json:"preferences"`
	Source          string               `json:"source"`
}

func (s *GameService) initializeP9Hudong(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&db.P9PetHudongRule{}, &db.P9PetHudongState{}); err != nil {
		return err
	}
	cfg := p9HudongConfig{WindowHours: 6, CooldownSeconds: 5, RewardCalls: 5, MaximumCalls: 10, Experience: 3, Multiplier: 2, Preferences: []p9HudongPreference{{0, 1}, {1, 1}, {2, 1}}, Source: constUtil.P9HudongSource}
	if err := s.validateP9Hudong(&cfg); err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P9PetHudongRule{ID: constUtil.P9HudongRuleID, Enabled: true, Input: raw, Source: cfg.Source}).Error
}
func (s *GameService) validateP9Hudong(cfg *p9HudongConfig) error {
	if cfg == nil || cfg.Source == "" || cfg.WindowHours != 6 || cfg.CooldownSeconds != 5 || cfg.RewardCalls != 5 || cfg.MaximumCalls != 10 || cfg.Experience != 3 || cfg.Multiplier != 2 || len(cfg.Preferences) != 3 {
		return errors.New(constUtil.P9HudongError)
	}
	for _, pair := range []struct {
		key   string
		value int64
	}{{"PetHudongUpdateTime", cfg.WindowHours}, {"HudongCDTime", cfg.CooldownSeconds}, {"PetMaxHudongTimes", int64(cfg.RewardCalls)}, {"PetMaxHudongExp", int64(cfg.Experience)}} {
		if tableNumber(s.table("GameConfigure", "StaticName", pair.key), "NameValue") != float64(pair.value) {
			return errors.New(constUtil.P9HudongError)
		}
	}
	seen := map[int32]bool{}
	for _, p := range cfg.Preferences {
		if p.Type < 0 || p.Type > 2 || seen[p.Type] || p.Weight < 1 || p.Weight > 1000000000000 {
			return errors.New(constUtil.P9HudongError)
		}
		seen[p.Type] = true
	}
	return nil
}
func p9HudongWindow(now time.Time, hours int64) int64 {
	shifted := now.UnixMilli() + 8*3600000
	size := hours * 3600000
	return shifted - shifted%size - 8*3600000
}

// p9HudongState freezes preference per real GUID and preserves legacy counters on first adoption.
func (s *GameService) p9HudongState(tx *gorm.DB, id int64, pet *pb.Pet, now time.Time) (*db.P9PetHudongState, *p9HudongConfig, error) {
	if pet.Eat == nil {
		pet.Eat = &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0)}
	}
	var live db.P9PetHudongRule
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&live, "id=?", constUtil.P9HudongRuleID).Error; err != nil {
		return nil, nil, err
	}
	if !live.Enabled {
		return nil, nil, errors.New(constUtil.P9HudongDisabled)
	}
	var state db.P9PetHudongState
	err := tx.First(&state, "guid=?", pet.GetGuid()).Error
	first := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !first {
		return nil, nil, err
	}
	cfg := &p9HudongConfig{}
	window := p9HudongWindow(now, 6)
	if !first && state.PlayerID != id {
		return nil, nil, errors.New(constUtil.P9HudongError)
	}
	if state.WindowAt > window || state.LastAt > now.UnixMilli() {
		return nil, nil, errors.New(constUtil.P9HudongError)
	}
	if first || state.WindowAt != window {
		if len(live.Input) > 65536 || live.Source == "" || json.Unmarshal(live.Input, cfg) != nil || s.validateP9Hudong(cfg) != nil {
			return nil, nil, errors.New(constUtil.P9HudongError)
		}
		counts := []int32{0, 0, 0}
		if first && len(pet.GetEat().GetHdTimeList()) != 0 {
			if len(pet.GetEat().GetHdTimeList()) != 3 {
				return nil, nil, errors.New(constUtil.P9HudongError)
			}
			counts = append([]int32(nil), pet.GetEat().GetHdTimeList()...)
		}
		for _, n := range counts {
			if n < 0 || n > 1000000 {
				return nil, nil, errors.New(constUtil.P9HudongError)
			}
		}
		choices := []db.GameplayPoolReward{}
		for _, pref := range cfg.Preferences {
			choices = append(choices, db.GameplayPoolReward{ID: pref.Type, Weight: pref.Weight, Amount: 1})
		}
		chosen, err := choosePoolReward(choices, false)
		if err != nil {
			return nil, nil, err
		}
		state.GUID = pet.GetGuid()
		state.PlayerID = id
		state.WindowAt = window
		state.Input = append([]byte(nil), live.Input...)
		state.InputHash = p6Digest(state.Input)
		state.Favorite = chosen.ID
		// Existing explicit imported mood is preserved for the first six-hour window.
		if first && pet.GetEat().CritTrend != nil && pet.GetEat().GetCritTrend() >= 0 && pet.GetEat().GetCritTrend() <= 3 {
			state.Favorite = pet.GetEat().GetCritTrend()
		}
		state.Counts, err = json.Marshal(counts)
		if err != nil {
			return nil, nil, err
		}
		if err := tx.Save(&state).Error; err != nil {
			return nil, nil, err
		}
	} else {
		if state.PlayerID != id || state.InputHash != p6Digest(state.Input) || json.Unmarshal(state.Input, cfg) != nil || s.validateP9Hudong(cfg) != nil {
			return nil, nil, errors.New(constUtil.P9HudongError)
		}
	}
	counts := []int32{}
	if state.PlayerID != id || state.Favorite < 0 || state.Favorite > 3 || json.Unmarshal(state.Counts, &counts) != nil || len(counts) != 3 || state.Earned < 0 || state.Applied < 0 || state.Capped < 0 || state.Applied > state.Earned || state.Capped != state.Earned-state.Applied {
		return nil, nil, errors.New(constUtil.P9HudongError)
	}
	for _, n := range counts {
		if n < 0 || n > 1000000 {
			return nil, nil, errors.New(constUtil.P9HudongError)
		}
	}
	if pet.Eat == nil {
		pet.Eat = &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0)}
	}
	pet.Eat.CritTrend = proto.Int32(state.Favorite)
	pet.Eat.HdTimeList = counts
	return &state, cfg, nil
}

// p9HudongGain applies free intimacy with original layer thresholds and existing attribute policy.
func (s *GameService) p9HudongGain(tx *gorm.DB, id int64, pet *pb.Pet, gain int32) (int32, int32, error) {
	if pet.Eat == nil || gain < 0 || gain > 6 || pet.GetEat().GetLevel() < 0 || pet.GetEat().GetLevel() >= 7 || pet.GetEat().GetExp() < 0 {
		return 0, 0, p5Error()
	}
	row := s.table("Pet", "id", pet.GetId())
	if row == nil {
		return 0, 0, p5Error()
	}
	left := gain
	for left > 0 {
		level := pet.GetEat().GetLevel()
		if level == 7 {
			return gain - left, left, nil
		}
		current := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(level)))
		next := s.p9TablePair("PetIntimacy", "quality", row["quality"], "intimacyLevel", strconv.Itoa(int(level+1)))
		need := int32(tableNumber(current, "intimacy"))
		if current == nil || next == nil || need < 1 || pet.GetEat().GetExp() >= need {
			return 0, 0, p5Error()
		}
		room := need - pet.GetEat().GetExp()
		if left < room {
			pet.Eat.Exp = proto.Int32(pet.GetEat().GetExp() + left)
			left = 0
			break
		}
		unlocked := int(tableNumber(next, "additionalTalent"))
		if unlocked > len(pet.GetRandPropList()) {
			if err := s.p9AllocateIntimacy(tx, id, pet, unlocked); err != nil {
				return 0, 0, err
			}
		}
		pet.Eat.Level = proto.Int32(level + 1)
		pet.Eat.Exp = proto.Int32(0)
		left -= room
	}
	return gain - left, left, nil
}

func (s *GameService) p9PetHudongAt(ctx context.Context, id int64, request *pb.CSPetHudong, stamp int64, now time.Time) ([]proto.Message, error) {
	if !s.P9IntimacyReady {
		return nil, errors.New(constUtil.P9HudongDisabled)
	}
	out := &pb.SCPetHudong{}
	return s.p9NextMutation(ctx, id, constUtil.P9HudongOperation, stamp, request, out, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		if request.GetType() < 0 || request.GetType() > 2 {
			return p5Error()
		}
		pet, err := p9OwnedPet(tx, id, request.GetGuid())
		if err != nil {
			return err
		}
		if err := s.p9DrainDeferredIntimacy(tx, id, g, pet); err != nil {
			return err
		}
		if pet.GetEat().GetLevel() >= 7 {
			return errors.New("精灵亲密度已经达到原上限")
		}
		state, cfg, err := s.p9HudongState(tx, id, pet, now)
		if err != nil {
			return err
		}
		if state.LastAt > 0 && now.UnixMilli()-state.LastAt < cfg.CooldownSeconds*1000 {
			return errors.New("精灵互动仍在原5秒冷却中")
		}
		kind := request.GetType()
		if pet.Eat.HdTimeList[kind] >= cfg.MaximumCalls {
			return errors.New("本窗口该类精灵互动次数已用完")
		}
		gain := int32(0)
		if pet.Eat.HdTimeList[kind] < cfg.RewardCalls {
			gain = cfg.Experience
			if kind == state.Favorite {
				gain *= cfg.Multiplier
			}
		}
		// Only a verified untouched base creates a zero-cost investment ledger.
		// Existing paid ledgers remain exact; unknown old growth keeps its protection.
		if pet.GetEat().GetLevel() == 0 && pet.GetEat().GetExp() == 0 && len(pet.GetRandPropList()) == 0 && p9BasicDonor(pet) {
			if err := p9Invest(tx, id, pet.GetGuid(), nil, nil); err != nil {
				return err
			}
		}
		applied, capped, err := s.p9HudongGain(tx, id, pet, gain)
		if err != nil {
			return err
		}
		if state.Earned > math.MaxInt64-int64(gain) || state.Applied > math.MaxInt64-int64(applied) || state.Capped > math.MaxInt64-int64(capped) {
			return p5Error()
		}
		pet.Eat.HdTimeList[kind]++
		pet.Eat.CritedValue = proto.Int32(applied)
		state.Counts, err = json.Marshal(pet.Eat.HdTimeList)
		if err != nil {
			return err
		}
		state.LastAt = now.UnixMilli()
		state.Earned += int64(gain)
		state.Applied += int64(applied)
		state.Capped += int64(capped)
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		if err := p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		out.Pet = proto.Clone(pet).(*pb.Pet)
		if s.P1Ready {
			return s.RecordP1Event(tx, id, "p9:hudong:"+strconv.FormatInt(stamp, 10), "pet_hudong", 1, LocalPlayAt(ctx, now))
		}
		return nil
	})
}
