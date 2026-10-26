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
	if len(rewards) > 0 {
		switch s.pointShop75Quality(rewards[0].GetId(), map[string]bool{}) {
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

// Resolve original fragments and original-format selector boxes to their equipment tab.
func (s *GameService) pointShop75Quality(id string, seen map[string]bool) int64 {
	if seen[id] || len(seen) > 32 {
		return 0
	}
	seen[id] = true
	if strings.HasPrefix(id, "frag_") {
		return s.pointShop75Quality(s.table("Frag", "id", id)["target"], seen)
	}
	for _, table := range []string{"Equip", "Treasure"} {
		if r := s.table(table, "id", id); r != nil {
			return int64(tableNumber(r, "quality"))
		}
	}
	if !strings.HasPrefix(id, "item_") {
		return 0
	}
	quality := int64(0)
	for _, r := range s.Tables["ItemUse"] {
		if r["itemId"] != id {
			continue
		}
		for _, part := range strings.Split(r["result"], ";") {
			// Each alternative is evaluated independently, including repeated item IDs.
			path := map[string]bool{}
			for k, v := range seen {
				path[k] = v
			}
			q := s.pointShop75Quality(strings.Split(part, ",")[0], path)
			if q == 0 || quality != 0 && q != quality {
				return 0
			}
			quality = q
		}
	}
	return quality
}
