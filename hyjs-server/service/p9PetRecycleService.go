package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) InitializeP9PetRecycle(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P9PetRetired{}); err != nil {
		return err
	}
	return s.initializeP9Market(tx)
}

// p9PetReset 用真实投入账本退还培养成本，原回包删除旧GUID并获得初始实例。
// 原攻略亲密历史留在旧GUID，不变成可退材料；未知旧档投入拒绝猜算。
func (s *GameService) p9PetReset(ctx context.Context, id int64, b *pb.CSPetReset, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetReset{Get: &pb.ResultObject{}}
	return s.p9NextMutation(ctx, id, constUtil.P9PetResetOperation, stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetGuid())
		if err != nil {
			return err
		}
		if p9Guarded(g, pet.GetGuid()) {
			return errors.New(constUtil.P9PetResetGuarded)
		}
		if float64(p.GetLevel()) < tableNumber(s.table("GameConfigure", "StaticName", "RecycleRoomLevel"), "NameValue") {
			return p5Error()
		}
		var investment db.P9PetInvestment
		pool := map[string]int64{}
		donors := []*pb.Pet{}
		err = tx.First(&investment, "guid=? AND player_id=?", pet.GetGuid(), id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 攻略亲密属于不可退款来源；其余成长无账本时不制造返还。
			base := proto.Clone(pet).(*pb.Pet)
			base.Eat = &pb.PetEat{Level: proto.Int32(0), Exp: proto.Int32(0)}
			if !p9BasicDonor(base) || len(pet.GetRandPropList()) != 0 {
				return errors.New(constUtil.P9PetResetInvestmentMissing)
			}
		} else if err != nil {
			return err
		} else if investment.Source == "" || json.Unmarshal(investment.Costs, &pool) != nil || pool == nil || json.Unmarshal(investment.Donors, &donors) != nil || donors == nil || len(donors) >= 100 {
			return errors.New(constUtil.P9PetResetInvestmentInvalid)
		}
		keys := make([]string, 0, len(pool))
		for key, count := range pool {
			if key == "" || strings.ContainsAny(key, ",;\r\n") || count < 1 || count > math.MaxInt32 || strings.HasPrefix(key, "pet_") {
				return errors.New(constUtil.P9PetResetInvestmentInvalid)
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		seen := map[string]bool{pet.GetGuid(): true}
		for _, donor := range donors {
			if donor == nil || donor.GetGuid() == "" || donor.GetId() != pet.GetId() || seen[donor.GetGuid()] || !p9BasicDonor(donor) || len(donor.GetRandPropList()) != 0 {
				return errors.New(constUtil.P9PetResetInvestmentInvalid)
			}
			seen[donor.GetGuid()] = true
			var remaining int64
			if err := tx.Model(&db.P9PetInstance{}).Where("guid=?", donor.GetGuid()).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining != 0 {
				return errors.New(constUtil.P9PetResetInvestmentInvalid)
			}
		}
		// 先付原重置成本，再同事务恢复资源/初始精灵；容量或溢出失败全部回滚。
		feeSpec := s.table("GameConfigure", "StaticName", "PetRebuildCost")["NameValue"]
		if strings.TrimSpace(feeSpec) == "" {
			return p5Error()
		}
		fees, err := s.growthCost(p, g, feeSpec, 1)
		if err != nil {
			return err
		}
		original, err := proto.Marshal(pet)
		if err != nil {
			return err
		}
		if err := tx.Create(&db.P9PetRetired{GUID: pet.GetGuid(), PlayerID: id, PetID: pet.GetId(), Data: original, Operation: constUtil.P9PetResetOperation, Timestamp: stamp, Source: constUtil.P9PetResetSource}).Error; err != nil {
			return err
		}
		removed := tx.Delete(&db.P9PetInstance{}, "guid=? AND player_id=?", pet.GetGuid(), id)
		if removed.Error != nil {
			return removed.Error
		}
		if removed.RowsAffected != 1 {
			return p5Error()
		}
		p5SetKV(g, "p9.pet:"+pet.GetGuid(), "")
		for _, key := range keys {
			gain, err := s.GrantFixedReward(p, g, key+","+strconv.FormatInt(pool[key], 10))
			if err != nil {
				return err
			}
			p1AppendGain(out.Get, gain)
		}
		out.Get.Pets, err = s.P9GrantPet(tx, p, pet.GetId(), int32(1+len(donors)))
		if err != nil {
			return err
		}
		out.Cost = append(fees, &pb.BaseResource{Id: pet.Id, Guid: pet.Guid, Num: proto.Int32(1)})
		// 旧投入账本保留为已退休GUID的审计事实，旧回执重试不复活实例。
		return s.p9RecordCosts(tx, id, constUtil.P9PetResetOperation+":"+strconv.FormatInt(stamp, 10), fees, LocalPlayNow(ctx))
	})
}

// Original TablePet.selloutPrice and BagSellPage send/consume GUID lists.
// Destruction of trained instances stays gated until native refund rules are verified.
func (s *GameService) p9PetSell(ctx context.Context, id int64, b *pb.CSPetSell, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetSell{Get: &pb.ResultObject{}}
	return s.p9NextMutation(ctx, id, "pet-sell", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if len(b.GetGuids()) < 1 || len(b.GetGuids()) > 100 {
			return p5Error()
		}
		seen := map[string]bool{}
		var total int64
		for _, guid := range b.GetGuids() {
			if strings.TrimSpace(guid) != guid || seen[guid] {
				return p5Error()
			}
			seen[guid] = true
			pet, err := p9OwnedPet(tx, id, guid)
			if err != nil {
				return err
			}
			if p9Guarded(g, guid) {
				return errors.New("守护中的精灵不能出售")
			}
			if !p9BasicDonor(pet) || len(pet.GetRandPropList()) > 0 {
				return errors.New("培养精灵原返还规则未确认，保护实例与投入")
			}
			var investment int64
			if err = tx.Model(&db.P9PetInvestment{}).Where("player_id=? AND guid=?", id, guid).Count(&investment).Error; err != nil {
				return err
			}
			if investment != 0 {
				return errors.New("精灵存在投入账本，原出售返还规则未确认")
			}
			row := s.table("Pet", "id", pet.GetId())
			price := int64(tableNumber(row, "selloutPrice"))
			if row == nil || price <= 0 || price > math.MaxInt32-total {
				return p5Error()
			}
			data, err := proto.Marshal(pet)
			if err != nil {
				return err
			}
			retired := db.P9PetRetired{GUID: guid, PlayerID: id, PetID: pet.GetId(), Data: data, Operation: "native-pet-sell", Timestamp: stamp, Coin: price, Source: "TablePet.selloutPrice；UIPet.InitData/SellPrice；BagSellPage.DoSell/SellCallback；仅未培养未守护原实例"}
			if err = tx.Create(&retired).Error; err != nil {
				return err
			}
			deleted := tx.Delete(&db.P9PetInstance{}, "player_id=? AND guid=?", id, guid)
			if deleted.Error != nil {
				return deleted.Error
			}
			if deleted.RowsAffected != 1 {
				return p5Error()
			}
			p5SetKV(g, "p9.pet:"+guid, "")
			out.Cost = append(out.Cost, &pb.BaseResource{Id: proto.String(pet.GetId()), Guid: proto.String(guid), Num: proto.Int32(1)})
			total += price
		}
		result, err := s.GrantFixedReward(p, g, fmt.Sprintf("coin,%d", total))
		if err != nil {
			return err
		}
		out.Get = result
		return nil
	})
}

func (s *GameService) DispatchP9PetRecycle(ctx context.Context, id int64, b proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request := b.(type) {
	case *pb.CSPetSell:
		out, err := s.p9PetSell(ctx, id, request, stamp)
		return out, true, err
	case *pb.CSPetReset:
		out, err := s.p9PetReset(ctx, id, request, stamp)
		return out, true, err
	case *pb.CSPetShopProductList:
		out, err := s.p9PetShopList(ctx, id)
		return out, true, err
	case *pb.CSPetShopBuy:
		out, err := s.p9PetShopBuy(ctx, id, request, stamp)
		return out, true, err
	case *pb.CSRefreshPetShop:
		out, err := s.p9PetShopRefresh(ctx, id, request, stamp)
		return out, true, err
	case *pb.CSPetDecompose:
		out, err := s.p9PetDecompose(ctx, id, request, stamp)
		return out, true, err
	default:
		return nil, false, nil
	}
}
