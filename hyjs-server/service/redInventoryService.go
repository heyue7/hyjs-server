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
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func redChoiceBox(key string) bool {
	if growth74BoxCategories[key] != "" {
		return true
	}
	switch key {
	case "item_067", "item_339", "item_340", "item_023", "item_053", "item_054", "item_118", "item_119", "item_120", "item_121", "item_122", "item_156", "item_157", "item_347", "item_348", "item_361", "item_362", "item_363":
		return true
	}
	return false
}

// redBoxUse 使用原 ItemUse 选项编号与数量；每份原箱子消耗、生成实例和回执一起提交。
func (s *GameService) redBoxUse(ctx context.Context, id int64, b *pb.CSItemUse, stamp int64) ([]proto.Message, error) {
	result := &pb.SCItemUse{}
	err := s.redP2Mutation(ctx, id, constUtil.P2BoxOperation, stamp, b, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		cost := b.GetCostItem()
		if cost.GetNum() < 1 || cost.GetNum() > 100 || !redChoiceBox(cost.GetId()) {
			return errors.New("红箱数量或类型无效")
		}
		item := s.table("Item", "itemId", cost.GetId())
		if item == nil {
			return errors.New("红箱原表缺失")
		}
		var options []map[string]string
		for _, row := range s.Tables["ItemUse"] {
			if row["itemId"] == cost.GetId() {
				options = append(options, row)
			}
		}
		if len(options) == 0 {
			return errors.New("红箱选项配置缺失")
		}
		if growth74BoxCategories[cost.GetId()] != "" {
			var maximum int64
			for _, option := range options {
				var instances int64
				for _, part := range strings.Split(option["result"], ";") {
					pair := strings.Split(part, ",")
					if len(pair) == 2 && ((strings.HasPrefix(pair[0], "hero_") && !strings.HasPrefix(pair[0], "hero_exp_")) || strings.HasPrefix(pair[0], "equip_") || strings.HasPrefix(pair[0], "trea_") || strings.HasPrefix(pair[0], "pet_")) {
						n, _ := strconv.ParseInt(pair[1], 10, 64)
						instances += n
					}
				}
				if instances > maximum {
					maximum = instances
				}
			}
			if maximum > 0 && int64(cost.GetNum()) > 100/maximum {
				return fmt.Errorf("此礼包每次最多打开%d个，请减少数量", 100/maximum)
			}
		}
		if err := deductPoolCost(p, g, &pb.BaseResource{Id: proto.String(cost.GetId()), Num: proto.Int32(cost.GetNum())}); err != nil {
			return err
		}
		result.Cost = &pb.BaseResource{Id: proto.String(cost.GetId()), Num: proto.Int32(cost.GetNum())}
		result.Get = &pb.ResultObject{}
		for n := int32(0); n < cost.GetNum(); n++ {
			var chosen map[string]string
			if item["category"] == "5" {
				for _, row := range options {
					if int32(tableNumber(row, "odds")) == b.GetGetItemIndex() {
						chosen = row
						break
					}
				}
			} else if item["category"] == "2" && growth74BoxCategories[cost.GetId()] == "2" {
				if len(options) != 1 || options[0]["odds"] != "100" {
					return errors.New("养成礼包固定奖励配置无效")
				}
				chosen = options[0]
			} else if item["category"] == "2" {
				var configured []db.RedP2BoxReward
				if e := tx.Where("box_id=?", cost.GetId()).Order("resource_id").Find(&configured).Error; e != nil {
					return e
				}
				var rewards []db.GameplayPoolReward
				for index, row := range configured {
					allowed := false
					for _, option := range options {
						for _, entry := range strings.Split(option["result"], ";") {
							allowed = allowed || entry == fmt.Sprintf("%s,%d", row.ResourceID, row.Amount)
						}
					}
					if !allowed || row.Weight < 1 || row.Weight > 1000000000000 || row.Amount != 1 {
						return errors.New("随机红箱兼容规则无效")
					}
					rewards = append(rewards, db.GameplayPoolReward{ID: int32(index), ResourceID: row.ResourceID, Amount: row.Amount, Weight: row.Weight})
				}
				picked, e := choosePoolReward(rewards, false)
				if e != nil {
					return e
				}
				chosen = map[string]string{"result": fmt.Sprintf("%s,%d", picked.ResourceID, picked.Amount)}
			}
			if chosen == nil {
				return errors.New("红箱选择不在原选项列表")
			}
			gain, e := s.GrantFixedReward(p, g, chosen["result"])
			if e != nil {
				return e
			}
			redMergeGain(result.Get, gain)
		}
		return nil
	})
	return []proto.Message{result}, err
}

func redMergeGain(target, gain *pb.ResultObject) {
	target.Simps = append(target.Simps, gain.Simps...)
	target.Wars = append(target.Wars, gain.Wars...)
	target.Equips = append(target.Equips, gain.Equips...)
	target.Treas = append(target.Treas, gain.Treas...)
	target.Pets = append(target.Pets, gain.Pets...)
}

func (s *GameService) redCombine(ctx context.Context, id int64, b *pb.CSCombine, stamp int64) ([]proto.Message, error) {
	result := &pb.SCCombine{}
	err := s.redP2Mutation(ctx, id, constUtil.P2CombineOperation, stamp, b, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		row := s.table("Frag", "target", b.GetTargetId())
		if row == nil {
			return errors.New("碎片合成目标无效")
		}
		count, e := strconv.ParseInt(row["num"], 10, 32)
		if e != nil || count < 1 {
			return errors.New("碎片合成数量无效")
		}
		cost := &pb.BaseResource{Id: proto.String(row["id"]), Num: proto.Int32(int32(count))}
		if e = deductPoolCost(p, g, cost); e != nil {
			return e
		}
		gain, e := s.GrantFixedReward(p, g, b.GetTargetId()+",1")
		if e != nil {
			return e
		}
		result.Get = gain
		result.Cost = []*pb.BaseResource{cost}
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) redFragGet(ctx context.Context, id int64, b *pb.CSWarriorFragGet) ([]proto.Message, error) {
	var row db.Profile
	if e := db.GetIns().WithContext(ctx).First(&row, id).Error; e != nil {
		return nil, e
	}
	_, g, e := decodeProfile(&row)
	if e != nil {
		return nil, e
	}
	wanted := map[string]bool{}
	for _, key := range b.GetId() {
		wanted[key] = true
	}
	result := &pb.SCWarriorFragGet{}
	for _, item := range g.Items {
		if strings.HasPrefix(item.GetId(), "frag_hero_") && (len(wanted) == 0 || wanted[item.GetId()]) {
			result.WarriorFrag = append(result.WarriorFrag, proto.Clone(item).(*pb.SimpleItem))
		}
	}
	return []proto.Message{result}, nil
}

func (s *GameService) redFragSell(ctx context.Context, id int64, b *pb.CSWarriorFragSell, stamp int64) ([]proto.Message, error) {
	result := &pb.SCWarriorFragSell{}
	err := s.redP2Mutation(ctx, id, constUtil.P2FragSellOperation, stamp, b, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		seen := map[string]bool{}
		var coins int64
		if len(b.GetSell()) < 1 || len(b.GetSell()) > 100 {
			return errors.New("碎片出售数量无效")
		}
		for _, cost := range b.GetSell() {
			row := s.table("Frag", "id", cost.GetId())
			if row == nil || !strings.HasPrefix(cost.GetId(), "frag_hero_") || cost.GetNum() < 1 || seen[cost.GetId()] {
				return errors.New("碎片出售参数无效")
			}
			seen[cost.GetId()] = true
			price, e := strconv.ParseInt(row["price"], 10, 32)
			if e != nil || price < 0 || coins > math.MaxInt32-price*int64(cost.GetNum()) {
				return errors.New("碎片出售将魂无效")
			}
			coins += price * int64(cost.GetNum())
			if e = deductPoolCost(p, g, cost); e != nil {
				return e
			}
			result.Cost = append(result.Cost, proto.Clone(cost).(*pb.BaseResource))
		}
		if coins > 0 {
			// Original BagSellPage hero-fragment preview names item_033 (将魂).
			gain, e := s.GrantFixedReward(p, g, fmt.Sprintf("item_033,%d", coins))
			if e != nil {
				return e
			}
			result.Get = gain
		} else {
			result.Get = &pb.ResultObject{}
		}
		return nil
	})
	return []proto.Message{result}, err
}
