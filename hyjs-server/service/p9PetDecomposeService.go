package service

import (
	"context"
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

// p9DecomposeRefund 与原重置保持同一实际投入边界，旧档未知培养不估算资源。
func (s *GameService) p9DecomposeRefund(tx *gorm.DB, id int64, pet *pb.Pet) (map[string]int64, []*pb.Pet, error) {
	var investment db.P9PetInvestment
	pool := map[string]int64{}
	donors := []*pb.Pet{}
	err := tx.First(&investment, "guid=? AND player_id=?", pet.GetGuid(), id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		base := proto.Clone(pet).(*pb.Pet)
		base.Eat = &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0)}
		if !p9BasicDonor(base) || len(pet.GetRandPropList()) != 0 {
			return nil, nil, errors.New(constUtil.P9PetResetInvestmentMissing)
		}
		return pool, donors, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if investment.Source == "" || json.Unmarshal(investment.Costs, &pool) != nil || pool == nil || json.Unmarshal(investment.Donors, &donors) != nil || donors == nil || len(donors) >= 100 {
		return nil, nil, errors.New(constUtil.P9PetResetInvestmentInvalid)
	}
	for key, n := range pool {
		if key == "" || strings.ContainsAny(key, ",;\r\n") || strings.HasPrefix(key, "pet_") || n < 1 || n > math.MaxInt32 {
			return nil, nil, errors.New(constUtil.P9PetResetInvestmentInvalid)
		}
	}
	seen := map[string]bool{pet.GetGuid(): true}
	for _, donor := range donors {
		if donor == nil || donor.GetId() != pet.GetId() || donor.GetGuid() == "" || seen[donor.GetGuid()] || !p9BasicDonor(donor) || len(donor.GetRandPropList()) != 0 {
			return nil, nil, errors.New(constUtil.P9PetResetInvestmentInvalid)
		}
		seen[donor.GetGuid()] = true
		var remains int64
		if err := tx.Model(&db.P9PetInstance{}).Where("guid=?", donor.GetGuid()).Count(&remains).Error; err != nil {
			return nil, nil, err
		}
		if remains != 0 {
			return nil, nil, errors.New(constUtil.P9PetResetInvestmentInvalid)
		}
	}
	return pool, donors, nil
}

// p9PetDecompose 以原coinpet返精灵本身价值，培养资源和捐献原实例全量同事务返还。
func (s *GameService) p9PetDecompose(ctx context.Context, id int64, request *pb.CSPetDecompose, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetDecompose{Get: &pb.ResultObject{}}
	return s.p9NextMutation(ctx, id, constUtil.P9DecomposeOperation, stamp, request, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if len(request.GetGuids()) < 1 || len(request.GetGuids()) > 100 || p.GetCoinpet() < 0 {
			return p5Error()
		}
		if float64(p.GetLevel()) < tableNumber(s.table("GameConfigure", "StaticName", "RecycleRoomLevel"), "NameValue") {
			return p5Error()
		}
		_, cfg, err := s.p9MarketState(tx, id, time.Now())
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		total := int64(0)
		for _, guid := range request.GetGuids() {
			if guid == "" || strings.TrimSpace(guid) != guid || seen[guid] {
				return p5Error()
			}
			seen[guid] = true
			pet, err := p9OwnedPet(tx, id, guid)
			if err != nil {
				return err
			}
			if p9Guarded(g, guid) {
				return errors.New("守护中的精灵不能分解")
			}
			value := cfg.Decompose[s.table("Pet", "id", pet.GetId())["quality"]]
			if value < 1 || total > math.MaxInt32-int64(value) {
				return p5Error()
			}
			total += int64(value)
			costs, donors, err := s.p9DecomposeRefund(tx, id, pet)
			if err != nil {
				return err
			}
			data, err := proto.Marshal(pet)
			if err != nil {
				return err
			}
			if err := tx.Create(&db.P9PetRetired{GUID: guid, PlayerID: id, PetID: pet.GetId(), Data: data, Operation: constUtil.P9DecomposeOperation, Timestamp: stamp, Coinpet: int64(value), Source: cfg.Source}).Error; err != nil {
				return err
			}
			removed := tx.Delete(&db.P9PetInstance{}, "player_id=? AND guid=?", id, guid)
			if removed.Error != nil {
				return removed.Error
			}
			if removed.RowsAffected != 1 {
				return p5Error()
			}
			p5SetKV(g, "p9.pet:"+guid, "")
			keys := make([]string, 0, len(costs))
			for key := range costs {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if key == "coinpet" {
					if p.GetCoinpet() > math.MaxInt64-costs[key] {
						return p5Error()
					}
					p.Coinpet = proto.Int64(p.GetCoinpet() + costs[key])
					out.Get.Simps = append(out.Get.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(int32(costs[key]))})
					continue
				}
				gain, err := s.GrantFixedReward(p, g, key+","+strconv.FormatInt(costs[key], 10))
				if err != nil {
					return err
				}
				p1AppendGain(out.Get, gain)
			}
			if len(donors) > 0 {
				refunded, err := s.P9GrantPet(tx, p, pet.GetId(), int32(len(donors)))
				if err != nil {
					return err
				}
				out.Get.Pets = append(out.Get.Pets, refunded...)
			}
			out.Cost = append(out.Cost, &pb.BaseResource{Id: pet.Id, Guid: pet.Guid, Num: proto.Int32(1)})
		}
		if p.GetCoinpet() > math.MaxInt64-total {
			return p5Error()
		}
		p.Coinpet = proto.Int64(p.GetCoinpet() + total)
		out.Get.Simps = append(out.Get.Simps, &pb.SimpleItem{Id: proto.String("coinpet"), Num: proto.Int32(int32(total))})
		if s.P1Ready {
			return s.RecordP1Event(tx, id, "p9:decompose:"+strconv.FormatInt(stamp, 10), "pet_decompose", int64(len(request.GetGuids())), LocalPlayNow(ctx))
		}
		return nil
	})
}
