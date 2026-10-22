package service

import (
	"strings"

	pb "hyjs-server/model/protocol"
)

// pointShop48Priority maps original TabPointShop's thousand-wide tabs to the displayed reward.
// Materials use 商品; equipment uses its real purple/orange/red quality. The final
// three digits preserve operator ordering without changing the stored offer or receipt.
func (s *GameService) pointShop48Priority(priority int32, rewards []*pb.BaseResource) int32 {
	group := int32(1)
	if len(rewards) > 0 && strings.HasPrefix(rewards[0].GetId(), "equip_") {
		row := s.table("Equip", "id", rewards[0].GetId())
		switch tableNumber(row, "quality") {
		case 18:
			group = 2
		case 20:
			group = 3
		case 23:
			group = 4
		}
	}
	order := priority % 1000
	if order < 0 {
		order = 0
	}
	return group*1000 + order
}
