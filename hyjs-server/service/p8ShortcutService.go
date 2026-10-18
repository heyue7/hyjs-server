package service

import (
	"context"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) p8FoodPercent(id string) (float64, error) {
	var value string
	if strings.HasPrefix(id, "FishItem_") {
		row := s.table("FishSkill", "FishItemID", id)
		if row["Type"] != "dish" || row["ValueType"] != "onetimeRecover" {
			return 0, p8GatherError()
		}
		value = row["Value"]
	} else {
		if s.table("Item", "itemId", id)["category"] != "16" {
			return 0, p8GatherError()
		}
		for _, row := range s.Tables["ItemUse"] {
			if row["itemId"] != id {
				continue
			}
			parts := strings.Split(row["result"], ",")
			if value != "" || len(parts) != 2 || parts[0] != "perhp" || row["category"] != "16" || row["odds"] != "100" {
				return 0, p8GatherError()
			}
			value = parts[1]
		}
	}
	percent, err := strconv.ParseFloat(value, 64)
	if err != nil || percent <= 0 || percent > 100 {
		return 0, p8GatherError()
	}
	return percent, nil
}

// Original UIFieldSetHpPage includes unowned recoverables as selectable entries;
// configuring a valid recipe never grants or consumes inventory.
func (s *GameService) p8Shortcut(ctx context.Context, id int64, b *pb.CSSideSetShortCut, stamp int64) ([]proto.Message, error) {
	return s.p8LifeMutation(ctx, id, constUtil.P8ShortcutOperation, stamp, b, &pb.SCSideSetShortCut{}, func(tx *gorm.DB, p *pb.Player, g *model.GameState, now int64) error {
		if b.GetPosition() < 1 || b.GetPosition() > 2 || len(b.GetId()) > 64 || strings.TrimSpace(b.GetId()) != b.GetId() {
			return p8GatherError()
		}
		if b.GetId() != "" {
			if _, err := s.p8FoodPercent(b.GetId()); err != nil {
				return err
			}
		}
		state, err := p8LifeState(tx, id)
		if err != nil {
			return err
		}
		if b.GetPosition() == 1 {
			state.Shortcut1 = b.GetId()
		} else {
			state.Shortcut2 = b.GetId()
		}
		return tx.Save(state).Error
	})
}

func (s *GameService) p8GemCookingList(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCGemAndCookingList{}
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		state, err := p8LifeState(tx, id)
		if err != nil {
			return err
		}
		out.GemList, out.CookingList = g.Gems, g.Cooking
		out.ShortCut1, out.ShortCut2 = proto.String(state.Shortcut1), proto.String(state.Shortcut2)
		// Original GCListCallback compares nextUseTime with CurrentUTCJavaTime.
		out.NextUseTime = proto.Int64(state.FoodReadyAt)
		return nil
	})
	return []proto.Message{out}, err
}
