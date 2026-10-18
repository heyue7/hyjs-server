package service

import (
	"context"
	"errors"
	"math"
	"strconv"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// itemUse 按 TableItem 的基础资源奖励使用已有道具，消耗和奖励在同一事务保存。
func (s *GameService) itemUse(ctx context.Context, id int64, request *pb.CSItemUse, timestamps ...int64) ([]proto.Message, error) {
	if redChoiceBox(request.GetCostItem().GetId()) {
		var stamp int64
		if len(timestamps) > 0 {
			stamp = timestamps[0]
		}
		return s.redBoxUse(ctx, id, request, stamp)
	}
	switch request.GetCostItem().GetId() {
	case "item_022", "item_024", "item_025", "item_115", "item_116", "item_117":
		var stamp int64
		if len(timestamps) > 0 {
			stamp = timestamps[0]
		}
		return s.treasureBoxUse(ctx, id, request, stamp)
	}
	var result *pb.SCItemUse
	err := updateProfile(ctx, id, func(player *pb.Player, game *model.GameState) error {
		var (
			cost     = request.GetCostItem()
			resource string
			item     *pb.SimpleItem
		)
		// 三项 category=1 道具的奖励直接来自 TableItem，礼包概率规则另行恢复。
		switch cost.GetId() {
		case "item_001":
			resource = "energy"
		case "item_002":
			resource = "stamina"
		case "item_062":
			resource = "coin"
		default:
			return errors.New("当前道具的使用规则尚未恢复")
		}
		row := s.table("Item", "itemId", cost.GetId())
		if row == nil || row["category"] != "1" {
			return errors.New("Missing item use configuration")
		}
		unit, parseErr := strconv.ParseInt(row[resource], 10, 32)
		if parseErr != nil || unit <= 0 {
			return errors.New("Invalid item use reward")
		}
		for _, candidate := range game.Items {
			if candidate.GetId() == cost.GetId() {
				if item != nil {
					return errors.New("道具存档包含重复记录")
				}
				item = candidate
			}
		}
		if item == nil || item.GetNum() < cost.GetNum() {
			return errors.New("道具数量不足")
		}
		gain := unit * int64(cost.GetNum())
		// ResultObject.simps.num 沿用 APK int32，批量奖励与资源余额均须可完整表示。
		if gain <= 0 || gain > math.MaxInt32 {
			return errors.New("道具奖励数量超出协议范围")
		}
		switch resource {
		case "energy":
			if player.GetEnergy() < 0 || int64(player.GetEnergy()) > math.MaxInt32-gain {
				return errors.New("耐力余额超出协议范围")
			}
			player.Energy = proto.Int32(player.GetEnergy() + int32(gain))
		case "stamina":
			if player.GetStamina() < 0 || int64(player.GetStamina()) > math.MaxInt32-gain {
				return errors.New("体力余额超出协议范围")
			}
			player.Stamina = proto.Int32(player.GetStamina() + int32(gain))
		case "coin":
			if player.GetCoin() < 0 || player.GetCoin() > math.MaxInt64-gain {
				return errors.New("金币余额超出协议范围")
			}
			player.Coin = proto.Int64(player.GetCoin() + gain)
		}
		item.Num = proto.Int32(item.GetNum() - cost.GetNum())
		if item.GetNum() == 0 {
			for index, candidate := range game.Items {
				if candidate == item {
					game.Items = append(game.Items[:index], game.Items[index+1:]...)
					break
				}
			}
		}
		result = &pb.SCItemUse{
			Cost: &pb.BaseResource{Id: proto.String(cost.GetId()), Num: proto.Int32(cost.GetNum())},
			Get:  &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String(resource), Num: proto.Int32(int32(gain))}}},
		}
		return nil
	})
	// UI 回调先 Delete(cost)，再 AddResultObj(get)，以原始增量响应更新本地背包。
	return []proto.Message{result}, err
}
