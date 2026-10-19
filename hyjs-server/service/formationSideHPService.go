package service

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// syncFormationSideHP moves each occupied camp's saved HP with its hero before
// authoritative board coordinates change. Client old/new strings never set HP.
func syncFormationSideHP(g *model.GameState, positions map[int32]int32) error {
	if g.Field == nil || g.Field.Side == nil {
		return nil
	}
	changed := false
	for _, slot := range g.Formation {
		if formationHero(slot) != nil && slot.GetBattlePos() != positions[slot.GetFormatPos()] {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	side := g.Field.Side
	if !p8SideValid(side) {
		return errors.New(constUtil.FormationSideHPInvalid)
	}
	previous := map[int32]float64{}
	for _, hp := range side.GetBattlePosHps() {
		previous[hp.GetBattlePos()] = hp.GetHpPercent()
	}
	values := []*pb.BattlePosHp{}
	for _, slot := range g.Formation {
		if formationHero(slot) == nil {
			continue
		}
		value, exists := previous[slot.GetBattlePos()]
		if !exists {
			// Legacy field profiles can contain only the aggregate HP; retain
			// that actual fraction when first creating the per-position list.
			value = side.GetCurHp()
		}
		values = append(values, &pb.BattlePosHp{BattlePos: proto.Int32(positions[slot.GetFormatPos()]), HpPercent: proto.Float64(value)})
	}
	side.BattlePosHps = values
	side.BattlePos3, side.BattlePos7, side.BattlePos9, side.BattlePos13 = proto.Float64(0), proto.Float64(0), proto.Float64(0), proto.Float64(0)
	for _, hp := range values {
		switch hp.GetBattlePos() {
		case 3:
			side.BattlePos3 = hp.HpPercent
		case 7:
			side.BattlePos7 = hp.HpPercent
		case 9:
			side.BattlePos9 = hp.HpPercent
		case 13:
			side.BattlePos13 = hp.HpPercent
		}
	}
	return nil
}

// formationSideHPRefresh consumes original one-way socket 30094 after 82003.
// The swap transaction already moved HP using server-owned old coordinates;
// delayed/replayed notifications project the latest side without moving it again.
func (s *GameService) formationSideHPRefresh(ctx context.Context, id int64, _ *pb.CSRefreshSideHp) ([]proto.Message, error) {
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	_, g, err := decodeProfile(&row)
	if err != nil {
		return nil, err
	}
	if g.Field == nil || g.Field.Side == nil {
		return nil, nil
	}
	if !p8SideValid(g.Field.Side) || g.Field.Side.GetPlayerId() != id {
		return nil, errors.New(constUtil.FormationSideHPInvalid)
	}
	return []proto.Message{&pb.SCFieldPlayerState{Player: []*pb.PlayerSide{proto.Clone(g.Field.Side).(*pb.PlayerSide)}}}, nil
}
