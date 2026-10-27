package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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

type p9RecruitOption struct {
	PetID     string `json:"petId"`
	Weight    int64  `json:"weight"`
	Guarantee bool   `json:"guarantee"`
}
type p9RecruitInput struct {
	Pool           []p9RecruitOption `json:"pool"`
	Source         string            `json:"source"`
	SingleCost     string            `json:"singleCost"`
	FiveCost       string            `json:"fiveCost"`
	FreeCap        int32             `json:"freeCap"`
	FreeIntervalMS int64             `json:"freeIntervalMs"`
	GuaranteeEvery int32             `json:"guaranteeEvery"`
	CoinPerDraw    int32             `json:"coinPerDraw"`
}

// initializeP9Recruit 默认池与原首领能力独立，不覆盖运营编辑或原历史存档。
func (s *GameService) initializeP9Recruit(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&db.P9PetRecruitRule{}, &db.P9PetRecruitProgress{}); err != nil {
		return err
	}
	counts := map[string]int64{}
	for _, pet := range s.Tables["Pet"] {
		if pet["quality"] == "18" || pet["quality"] == "20" || pet["quality"] == "23" {
			counts[pet["quality"]]++
		}
	}
	if counts["18"] != 21 || counts["20"] != 38 || counts["23"] != 19 {
		return errors.New(constUtil.P9RecruitConfigurationError)
	}
	scale := counts["18"] * counts["20"] * counts["23"]
	group := map[string]int64{"18": 70, "20": 29, "23": 1}
	var pool []p9RecruitOption
	for _, pet := range s.Tables["Pet"] {
		quality := pet["quality"]
		if group[quality] == 0 {
			continue
		}
		pool = append(pool, p9RecruitOption{PetID: pet["id"], Weight: group[quality] * scale / counts[quality], Guarantee: quality == "20" || quality == "23"})
	}
	raw, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	rule := db.P9PetRecruitRule{ID: constUtil.P9RecruitRuleID, Enabled: true, Pool: raw, Source: constUtil.P9RecruitSource}
	if _, err := s.p9RecruitInput(rule); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error
}

func (s *GameService) p9RecruitInput(rule db.P9PetRecruitRule) (*p9RecruitInput, error) {
	input := &p9RecruitInput{Source: rule.Source}
	if len(rule.Pool) > 65536 || json.Unmarshal(rule.Pool, &input.Pool) != nil {
		return nil, errors.New(constUtil.P9RecruitConfigurationError)
	}
	value := func(name string) string { return s.table("GameConfigure", "StaticName", name)["NameValue"] }
	input.SingleCost = value("PetRecruitCostOnce")
	input.FiveCost = value("PetRecruitCostFive")
	free, e1 := strconv.ParseInt(value("PetRecruitFreeTimes"), 10, 32)
	hours, e2 := strconv.ParseFloat(value("PetRecruitFreeTimeInterval"), 64)
	must, e3 := strconv.ParseInt(value("PetRecruitMustHitTimes"), 10, 32)
	coin, e4 := strconv.ParseInt(value("PetRecruitCostOnceCoin"), 10, 32)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || free != 1 || hours != 24 || must != 9 || coin != 2000 {
		return nil, errors.New(constUtil.P9RecruitConfigurationError)
	}
	input.FreeCap = int32(free)
	input.FreeIntervalMS = int64(hours * 3600000)
	input.GuaranteeEvery = int32(must + 1)
	input.CoinPerDraw = int32(coin)
	return input, s.validateP9RecruitInput(input)
}

func (s *GameService) validateP9RecruitInput(input *p9RecruitInput) error {
	if input == nil || strings.TrimSpace(input.Source) == "" || len(input.Pool) < 1 || len(input.Pool) > 78 || input.SingleCost != "diamond,150" || input.FiveCost != "diamond,700" || input.FreeCap != 1 || input.FreeIntervalMS != 86400000 || input.GuaranteeEvery != 10 || input.CoinPerDraw != 2000 {
		return errors.New(constUtil.P9RecruitConfigurationError)
	}
	seen := map[string]bool{}
	guarantee := false
	for _, option := range input.Pool {
		pet := s.table("Pet", "id", option.PetID)
		if pet == nil || seen[option.PetID] || option.Weight < 1 || option.Weight > 1000000000000 || pet["defaultTalent"] == "" || (pet["quality"] != "18" && pet["quality"] != "20" && pet["quality"] != "23") || option.Guarantee != (pet["quality"] == "20" || pet["quality"] == "23") {
			return errors.New(constUtil.P9RecruitConfigurationError)
		}
		seen[option.PetID] = true
		guarantee = guarantee || option.Guarantee
	}
	if !guarantee {
		return errors.New(constUtil.P9RecruitConfigurationError)
	}
	return nil
}

// p9RecruitProgress 由持有Profile行锁的调用方冻结规则与周期；免费余额封顶为原1次。
func (s *GameService) p9RecruitProgress(tx *gorm.DB, id int64, state *db.P9PetState, now int64) (*db.P9PetRecruitProgress, *p9RecruitInput, error) {
	var live db.P9PetRecruitRule
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&live, "id=?", constUtil.P9RecruitRuleID).Error; err != nil {
		return nil, nil, err
	}
	if !live.Enabled {
		return nil, nil, errors.New(constUtil.P9RecruitDisabled)
	}
	var progress db.P9PetRecruitProgress
	err := tx.First(&progress, "player_id=?", id).Error
	var input *p9RecruitInput
	if errors.Is(err, gorm.ErrRecordNotFound) {
		input, err = s.p9RecruitInput(live)
		if err != nil {
			return nil, nil, err
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, nil, err
		}
		progress = db.P9PetRecruitProgress{PlayerID: id, Input: raw, InputHash: p6Digest(raw), InitializedAt: now}
		state.FreeRemaining = input.FreeCap
		state.NextFree = now + input.FreeIntervalMS
		state.MustTimes = input.GuaranteeEvery - 1
		if err := tx.Create(&progress).Error; err != nil {
			return nil, nil, err
		}
	} else {
		if err != nil {
			return nil, nil, err
		}
		input = &p9RecruitInput{}
		if progress.InputHash != p6Digest(progress.Input) || json.Unmarshal(progress.Input, input) != nil || s.validateP9RecruitInput(input) != nil || progress.SinceGuarantee < 0 || progress.SinceGuarantee >= input.GuaranteeEvery || progress.Draws < 0 || progress.InitializedAt <= 0 {
			return nil, nil, errors.New(constUtil.P9RecruitConfigurationError)
		}
		if state.NextFree <= 0 || state.FreeRemaining < 0 || state.FreeRemaining > input.FreeCap || state.MustTimes != input.GuaranteeEvery-1-progress.SinceGuarantee {
			return nil, nil, errors.New(constUtil.P9RecruitConfigurationError)
		}
		if now >= state.NextFree {
			cycles := (now-state.NextFree)/input.FreeIntervalMS + 1
			if cycles > (math.MaxInt64-state.NextFree)/input.FreeIntervalMS {
				return nil, nil, errors.New(constUtil.P9RecruitConfigurationError)
			}
			state.NextFree += cycles * input.FreeIntervalMS
			state.FreeRemaining = input.FreeCap
		}
	}
	if err := tx.Save(state).Error; err != nil {
		return nil, nil, err
	}
	return &progress, input, nil
}

// p9PetRecruit 扣费、完整实例、保底、免费游标与原P9回执同事务提交。
func (s *GameService) p9PetRecruit(ctx context.Context, id int64, request *pb.CSPetRecruit, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetRecruit{Get: &pb.ResultObject{}}
	return s.p9Mutation(ctx, id, constUtil.P9RecruitOperation, stamp, request, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if (request.GetFree() != 0 && request.GetFree() != 1) || (request.GetNumType() != 1 && request.GetNumType() != 2) || request.GetFree() == 1 && request.GetNumType() != 1 {
			return p5Error()
		}
		state, err := p9State(tx, id)
		if err != nil {
			return err
		}
		if !state.FirstGranted {
			return errors.New("请先领取原首次精灵")
		}
		now := time.Now().UnixMilli()
		progress, input, err := s.p9RecruitProgress(tx, id, state, now)
		if err != nil {
			return err
		}
		count := int32(1)
		price := input.SingleCost
		if request.GetNumType() == 2 {
			count = 5
			price = input.FiveCost
		}
		if progress.Draws > math.MaxInt64-int64(count) || p.GetCoin() > math.MaxInt64-int64(input.CoinPerDraw)*int64(count) || p.GetCoin() < 0 {
			return p5Error()
		}
		var owned int64
		if err := tx.Model(&db.P9PetInstance{}).Where("player_id=?", id).Count(&owned).Error; err != nil {
			return err
		}
		if owned+int64(count) > int64(p.GetPetBagSize()) {
			return errors.New("精灵背包容量不足")
		}
		if request.GetFree() == 1 {
			if state.FreeRemaining < 1 {
				return errors.New("免费精灵招募次数不足")
			}
			state.FreeRemaining--
		} else {
			cost, err := parsePoolCost(price)
			if err != nil {
				return err
			}
			if err := deductPoolCost(p, g, cost); err != nil {
				return err
			}
			out.Cost = []*pb.BaseResource{cost}
		}
		options := make([]db.GameplayPoolReward, 0, len(input.Pool))
		for index, option := range input.Pool {
			options = append(options, db.GameplayPoolReward{ID: int32(index), ResourceID: option.PetID, Amount: 1, Weight: option.Weight, Guaranteed: option.Guarantee})
		}
		for i := int32(0); i < count; i++ {
			forced := progress.SinceGuarantee == input.GuaranteeEvery-1
			selected, err := choosePoolReward(options, forced)
			if err != nil {
				return err
			}
			pets, err := s.P9GrantPet(tx, p, selected.ResourceID, 1)
			if err != nil {
				return err
			}
			out.Get.Pets = append(out.Get.Pets, pets...)
			out.GetList = append(out.GetList, &pb.BaseResource{Id: pets[0].Id, Guid: pets[0].Guid, Num: proto.Int32(1)})
			if selected.Guaranteed {
				progress.SinceGuarantee = 0
			} else {
				progress.SinceGuarantee++
			}
			progress.Draws++
		}
		coin := input.CoinPerDraw * count
		p.Coin = proto.Int64(p.GetCoin() + int64(coin))
		out.Get.Simps = []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(coin)}}
		state.MustTimes = input.GuaranteeEvery - 1 - progress.SinceGuarantee
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		if err := tx.Save(progress).Error; err != nil {
			return err
		}
		if s.P1Ready {
			return s.RecordP1Event(tx, id, "p9:recruit:"+strconv.FormatInt(stamp, 10), constUtil.P9RecruitEvent, int64(count), LocalPlayAt(ctx, time.UnixMilli(now)))
		}
		return nil
	})
}
