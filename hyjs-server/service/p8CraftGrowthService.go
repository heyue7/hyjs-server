package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
)

// P8RefreshCraftPower persists the same original party calculation used by
// native panels and battle snapshots after equipment gems or permanent food.
func (s *GameService) P8RefreshCraftPower(p *pb.Player, g *model.GameState) error {
	power, err := s.formationFightPower(p, g)
	if err != nil {
		return err
	}
	p.FightPower = proto.Int32(power)
	return nil
}

// p8CookingRefundCosts follows original TableText.recycle_desc_text's 50% dish
// refund. Native resources are integer units: odd cumulative units round down,
// an explicit compatibility choice where the original server rounding is absent.
// Other verified investments keep their existing exact refund rule.
func (s *GameService) p8CookingRefundCosts(costs []*pb.BaseResource) ([]*pb.BaseResource, error) {
	var result []*pb.BaseResource
	for _, cost := range costs {
		if cost == nil || cost.GetNum() < 1 || cost.GetGuid() != "" {
			return nil, p5Error()
		}
		count := cost.GetNum()
		if dish := s.table("FishSkill", "FishItemID", cost.GetId()); dish != nil {
			if dish["Type"] != "dish" || !strings.HasPrefix(dish["ValueType"], "permanent") {
				return nil, p5Error()
			}
			count /= 2
		}
		if count > 0 {
			result = append(result, &pb.BaseResource{Id: cost.Id, Num: proto.Int32(count)})
		}
	}
	return result, nil
}

// p8SeparateCookingCosts keeps verified permanent dishes in their dedicated
// inventory path; existing GrantFixedReward does not handle FishSkill resources.
func (s *GameService) p8SeparateCookingCosts(costs []*pb.BaseResource) ([]*pb.BaseResource, error) {
	var regular []*pb.BaseResource
	for _, cost := range costs {
		if dish := s.table("FishSkill", "FishItemID", cost.GetId()); dish != nil {
			if dish["Type"] != "dish" || !strings.HasPrefix(dish["ValueType"], "permanent") {
				return nil, p5Error()
			}
			continue
		}
		regular = append(regular, cost)
	}
	return regular, nil
}

// p8GrantCookingRefund returns actual dishes through cooking inventory checks
// while preserving the original fixed-reward path for all other investments.
func (s *GameService) p8GrantCookingRefund(p *pb.Player, g *model.GameState, costs []*pb.BaseResource) (*pb.ResultObject, error) {
	regular, err := s.p8SeparateCookingCosts(costs)
	if err != nil {
		return nil, err
	}
	gain := &pb.ResultObject{}
	if len(regular) > 0 {
		gain, err = s.GrantFixedReward(p, g, p5ResourceSpec(regular))
		if err != nil {
			return nil, err
		}
	}
	food := map[string]int64{}
	for _, cost := range costs {
		if s.table("FishSkill", "FishItemID", cost.GetId()) != nil {
			food[cost.GetId()] += int64(cost.GetNum())
		}
	}
	if len(food) > 0 {
		if err = s.cookingInventoryDelta(p, g, food); err != nil {
			return nil, err
		}
		redMergeGain(gain, gemRewards(food))
	}
	return gain, nil
}
