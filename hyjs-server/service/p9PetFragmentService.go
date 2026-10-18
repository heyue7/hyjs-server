package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) p9TakeFragment(g *model.GameState, key string, num int32) error {
	row := s.table("Frag", "id", key)
	if row == nil || !strings.HasPrefix(key, "frag_pet_") || !strings.HasPrefix(row["target"], "pet_") || s.table("Pet", "id", row["target"]) == nil || num < 1 {
		return p5Error()
	}
	var owned *pb.SimpleItem
	for _, item := range g.Items {
		if item.GetId() == key {
			if owned != nil {
				return p5Error()
			}
			owned = item
		}
	}
	if owned == nil || owned.GetNum() < num {
		return errors.New("原精灵碎片数量不足")
	}
	owned.Num = proto.Int32(owned.GetNum() - num)
	return nil
}

// BagMemberDetail uses CSCombine.targetId; native TableFrag.num consumes one
// original fragment type and creates exactly one new owned GUID.
func (s *GameService) p9PetCombine(ctx context.Context, id int64, b *pb.CSCombine, stamp int64) ([]proto.Message, error) {
	out := &pb.SCCombine{Get: &pb.ResultObject{}}
	return s.p9Mutation(ctx, id, "pet-fragment-combine", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if s.table("Pet", "id", b.GetTargetId()) == nil {
			return p5Error()
		}
		var found map[string]string
		for _, row := range s.Tables["Frag"] {
			if row["target"] == b.GetTargetId() {
				if found != nil {
					return p5Error()
				}
				found = row
			}
		}
		if found == nil {
			return p5Error()
		}
		n, err := strconv.ParseInt(found["num"], 10, 32)
		if err != nil || n < 1 {
			return p5Error()
		}
		if err = s.p9TakeFragment(g, found["id"], int32(n)); err != nil {
			return err
		}
		pets, err := s.P9GrantPet(tx, p, b.GetTargetId(), 1)
		if err != nil {
			return err
		}
		source := "TableFrag." + found["id"] + "=" + strconv.FormatInt(n, 10) + "；原CSCombine21041单次合成；实际原碎片扣费"
		if err = tx.Model(&db.P9PetInstance{}).Where("player_id=? AND guid=?", id, pets[0].GetGuid()).Update("source", source).Error; err != nil {
			return err
		}
		out.Get.Pets = pets
		out.Cost = []*pb.BaseResource{{Id: proto.String(found["id"]), Num: proto.Int32(int32(n))}}
		return s.RecordP1Event(tx, id, "p9:fragment-combine:"+strconv.FormatInt(stamp, 10), "pet_fragment_consumed", n, LocalPlayNow(ctx))
	})
}

// Original BagSellPage Pet-fragment preview labels coin, not Player.coinpet.
func (s *GameService) p9PetFragSell(ctx context.Context, id int64, b *pb.CSPetFragSell, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetFragSell{Get: &pb.ResultObject{}}
	return s.p9Mutation(ctx, id, "pet-fragment-sell", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if len(b.GetSells()) < 1 || len(b.GetSells()) > 100 {
			return p5Error()
		}
		seen := map[string]bool{}
		var coin, total int64
		for _, cost := range b.GetSells() {
			if cost.GetGuid() != "" || seen[cost.GetId()] || cost.GetNum() < 1 {
				return p5Error()
			}
			seen[cost.GetId()] = true
			row := s.table("Frag", "id", cost.GetId())
			price := int64(tableNumber(row, "price"))
			if row == nil || price < 1 || price > math.MaxInt32/int64(cost.GetNum()) || coin > math.MaxInt32-price*int64(cost.GetNum()) {
				return p5Error()
			}
			if err := s.p9TakeFragment(g, cost.GetId(), cost.GetNum()); err != nil {
				return err
			}
			coin += price * int64(cost.GetNum())
			total += int64(cost.GetNum())
			out.Cost = append(out.Cost, proto.Clone(cost).(*pb.BaseResource))
		}
		gain, err := s.GrantFixedReward(p, g, fmt.Sprintf("coin,%d", coin))
		if err != nil {
			return err
		}
		out.Get = gain
		return s.RecordP1Event(tx, id, "p9:fragment-sell:"+strconv.FormatInt(stamp, 10), "pet_fragment_consumed", total, LocalPlayNow(ctx))
	})
}

func (s *GameService) DispatchP9PetFragments(ctx context.Context, id int64, b proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request := b.(type) {
	case *pb.CSCombine:
		if !strings.HasPrefix(request.GetTargetId(), "pet_") {
			return nil, false, nil
		}
		out, err := s.p9PetCombine(ctx, id, request, stamp)
		return out, true, err
	case *pb.CSPetFragSell:
		out, err := s.p9PetFragSell(ctx, id, request, stamp)
		return out, true, err
	default:
		return nil, false, nil
	}
}
