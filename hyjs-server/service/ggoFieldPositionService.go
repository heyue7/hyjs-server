package service

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
	"hyjs-server/model"
)

func ggoPositionBump(game *model.GameState) error {
	if game == nil || game.GGOField == nil || game.GGOField.PositionRevision < 0 || game.GGOField.PositionRevision == math.MaxInt64 {
		return errors.New("沙漠位置版本无效")
	}
	game.GGOField.PositionRevision++
	game.GGOField.PositionConfirmed = 0
	game.GGOField.PositionSession = ""
	return nil
}

// GGOFieldPositionStatus is a recovery read after the original position frame.
// It does not acknowledge a handshake or move the character by itself.
func (s *GameService) GGOFieldPositionStatus(ctx context.Context, auth model.GGOFieldAuth, session []byte) (*model.GGOFieldPositionStatusResponse, error) {
	var out model.GGOFieldPositionStatusResponse
	err := s.ggoWithProfile(ctx, auth, session, false, false, func(_ *gorm.DB, a *ggoFieldActor) error {
		if a.Game.Position == nil || a.Game.Position.Current == nil || a.Game.GGOField == nil {
			return errors.New("沙漠位置尚未初始化")
		}
		position := a.Game.Position.Current
		x, y, z := float64(position.GetX()), float64(position.GetY()), float64(position.GetZ())
		out = model.GGOFieldPositionStatusResponse{Scene: a.Game.Position.GetScene(), Position: &model.AdminFieldGGOPosition{X: &x, Y: &y, Z: &z}, PositionRevision: a.Game.GGOField.PositionRevision, Loading: a.Game.PositionLoading}
		if a.Game.GGOField.PositionSession == clientUpdateSessionHash(session) {
			out.ConfirmedRevision = a.Game.GGOField.PositionConfirmed
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
