package service

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
)

// p5BasicItemUse 将原表明确的基础消耗品纳入玩家行锁及原帧回执，重试保留首笔增量。
func (s *GameService) p5BasicItemUse(ctx context.Context, id int64, b *pb.CSItemUse, stamp int64) ([]proto.Message, error) {
	out := &pb.SCItemUse{}
	return s.p5Mutation(ctx, id, "item-basic", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		cost := b.GetCostItem()
		if cost.GetGuid() != "" || cost.GetNum() < 1 || cost.GetNum() > 100 || b.GetGetItemIndex() != 0 {
			return p5Error()
		}
		row := s.table("Item", "itemId", cost.GetId())
		if row == nil || row["category"] != "1" {
			return p5Error()
		}
		var rewards []*pb.BaseResource
		for _, key := range []string{"energy", "stamina", "coin", "diamond"} {
			if row[key] == "" {
				continue
			}
			unit, err := strconv.ParseInt(row[key], 10, 64)
			if err != nil || unit < 1 || unit > math.MaxInt32/int64(cost.GetNum()) {
				return p5Error()
			}
			rewards = append(rewards, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(int32(unit * int64(cost.GetNum())))})
		}
		if len(rewards) == 0 || row["exp"] != "" {
			return p5Error()
		}
		if err := deductPoolCost(p, g, cost); err != nil {
			return err
		}
		gain, err := s.GrantFixedReward(p, g, p5ResourceSpec(rewards))
		if err != nil {
			return err
		}
		out.Cost = proto.Clone(cost).(*pb.BaseResource)
		out.Get = gain
		return nil
	})
}

// p5BoxUse uses original ItemUse weights and selection indexes, each bundle independently.
func (s *GameService) p5BoxUse(ctx context.Context, id int64, b *pb.CSItemUse, stamp int64) ([]proto.Message, error) {
	out := &pb.SCItemUse{}
	return s.p5Mutation(ctx, id, "item-box", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		cost := b.GetCostItem()
		if cost.GetGuid() != "" || cost.GetNum() < 1 || cost.GetNum() > 100 {
			return p5Error()
		}
		item := s.table("Item", "itemId", cost.GetId())
		if item == nil {
			return p5Error()
		}
		options := []map[string]string{}
		for _, r := range s.Tables["ItemUse"] {
			if r["itemId"] == cost.GetId() {
				options = append(options, r)
			}
		}
		if len(options) == 0 {
			return p5Error()
		}
		if err := deductPoolCost(p, g, cost); err != nil {
			return err
		}
		out.Get = &pb.ResultObject{}
		out.Cost = proto.Clone(cost).(*pb.BaseResource)
		for n := int32(0); n < cost.GetNum(); n++ {
			var selected map[string]string
			if item["category"] == "5" {
				for _, r := range options {
					index, err := strconv.ParseInt(r["odds"], 10, 32)
					if err != nil {
						return err
					}
					if int32(index) == b.GetGetItemIndex() {
						if selected != nil {
							return p5Error()
						}
						selected = r
					}
				}
			} else if item["category"] == "2" {
				if b.GetGetItemIndex() != 0 {
					return p5Error()
				}
				weights := []db.GameplayPoolReward{}
				for i, r := range options {
					weight, err := strconv.ParseInt(r["odds"], 10, 64)
					if err != nil || weight < 1 || weight > 1000000000000 {
						return p5Error()
					}
					weights = append(weights, db.GameplayPoolReward{ID: int32(i), Weight: weight, Amount: 1})
				}
				picked, err := choosePoolReward(weights, false)
				if err != nil {
					return err
				}
				selected = options[picked.ID]
			}
			if selected == nil {
				return p5Error()
			}
			var (
				gain *pb.ResultObject
				err  error
			)
			if selected["result"] == constUtil.P5WantedItemReward {
				// Original item004 restores one paid wanted attempt; the natural recovery cap does not truncate it.
				if cost.GetId() != constUtil.P5WantedItemID || item["category"] != "2" || len(options) != 1 || p.GetWantedAttTimes() < 0 || p.GetWantedAttTimes() == math.MaxInt32 {
					return p5Error()
				}
				p.WantedAttTimes = proto.Int32(p.GetWantedAttTimes() + 1)
				gain = &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("wantedAttTimes"), Num: proto.Int32(1)}}}
			} else {
				gain, err = s.GrantFixedReward(p, g, selected["result"])
			}
			if err != nil {
				return err
			}
			redMergeGain(out.Get, gain)
		}
		return nil
	})
}

func (s *GameService) p5EquipmentFragSell(ctx context.Context, id int64, b *pb.CSEquipmentFragSell, stamp int64) ([]proto.Message, error) {
	out := &pb.SCEquipmentFragSell{}
	return s.p5Mutation(ctx, id, "equipment-fragment-sale", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if len(b.GetCost()) < 1 || len(b.GetCost()) > 100 {
			return p5Error()
		}
		seen := map[string]bool{}
		var coin int64
		for _, r := range b.GetCost() {
			row := s.table("Frag", "id", r.GetId())
			if row == nil || row["target"][:min(6, len(row["target"]))] != "equip_" || r.GetGuid() != "" || r.GetNum() < 1 || seen[r.GetId()] {
				return p5Error()
			}
			seen[r.GetId()] = true
			price, err := strconv.ParseInt(row["price"], 10, 64)
			if err != nil || price < 0 || price > math.MaxInt32/int64(r.GetNum()) || coin > math.MaxInt32-price*int64(r.GetNum()) {
				return p5Error()
			}
			coin += price * int64(r.GetNum())
			if err = deductPoolCost(p, g, r); err != nil {
				return err
			}
			out.Cost = append(out.Cost, proto.Clone(r).(*pb.BaseResource))
		}
		if coin <= 0 {
			return p5Error()
		}
		// BagSellPage's original equipment-fragment preview uses strategy_score (point).
		gain, err := s.GrantFixedReward(p, g, fmt.Sprintf("point,%d", coin))
		out.Get = gain
		return err
	})
}

func (s *GameService) p5WarriorFragSell(ctx context.Context, id int64, b *pb.CSWarriorFragSell, stamp int64) ([]proto.Message, error) {
	out := &pb.SCWarriorFragSell{}
	return s.p5Mutation(ctx, id, "warrior-fragment-sale", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if len(b.GetSell()) < 1 || len(b.GetSell()) > 100 {
			return p5Error()
		}
		seen := map[string]bool{}
		var soul int64
		for _, cost := range b.GetSell() {
			row := s.table("Frag", "id", cost.GetId())
			target := row["target"]
			if row == nil || len(target) < 5 || target[:5] != "hero_" || cost.GetGuid() != "" || cost.GetNum() < 1 || seen[cost.GetId()] {
				return p5Error()
			}
			seen[cost.GetId()] = true
			price := int64(tableNumber(row, "price"))
			if (price < 1 && !p15GreenFragmentZeroSale(row)) || price > math.MaxInt32/int64(cost.GetNum()) || soul > math.MaxInt32-price*int64(cost.GetNum()) {
				return p5Error()
			}
			soul += price * int64(cost.GetNum())
			if err := deductPoolCost(p, g, cost); err != nil {
				return err
			}
			out.Cost = append(out.Cost, proto.Clone(cost).(*pb.BaseResource))
		}
		// 六绿零卖价仍扣除实际碎片，以空增量回包，避免零数量奖励被误判。
		if soul == 0 {
			out.Get = &pb.ResultObject{}
			return nil
		}
		// Original BagSellPage names item_033 (将魂) for the hero-fragment price.
		gain, err := s.GrantFixedReward(p, g, fmt.Sprintf("item_033,%d", soul))
		out.Get = gain
		return err
	})
}
