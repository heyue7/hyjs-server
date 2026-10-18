package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"time"
)

// syncP12FieldPVPPenaltyForRead uses the caller's locked, decoded objects. Most
// installations are default-off and white: their ordinary reads do no PK query.
// Persistent jail/red facts still recover even after operators disable new PK.
func (s *GameService) syncP12FieldPVPPenaltyForRead(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time) (bool, error) {
	if !s.P12FieldPVPReady || p == nil || g == nil {
		return false, nil
	}
	if p.GetRedeemTime() == 0 && (g.Field == nil || g.Field.Side.GetKills() == 0) {
		return false, nil
	}
	beforeRedeem, beforeLoading := p.GetRedeemTime(), g.PositionLoading
	var beforePosition *pb.Position
	var beforeSide *pb.PlayerSide
	if g.Position != nil {
		beforePosition = proto.Clone(g.Position).(*pb.Position)
	}
	if g.Field != nil && g.Field.Side != nil {
		beforeSide = proto.Clone(g.Field.Side).(*pb.PlayerSide)
	}
	if err := s.SyncP12FieldPVPPenalty(tx, p, g, now); err != nil {
		return false, err
	}
	var afterSide *pb.PlayerSide
	if g.Field != nil {
		afterSide = g.Field.Side
	}
	return beforeRedeem != p.GetRedeemTime() || beforeLoading != g.PositionLoading || !proto.Equal(beforePosition, g.Position) || !proto.Equal(beforeSide, afterSide), nil
}
func (s *GameService) pauseP12FieldPVPSurvival(tx *gorm.DB, id int64, now time.Time) (bool, error) {
	if !s.P12FieldPVPReady {
		return false, nil
	}
	err := P12FieldPVPRequireIdle(tx, id, now)
	if err == nil {
		return false, nil
	}
	if err.Error() == constUtil.P12FieldPVPBusy || err.Error() == constUtil.P12FieldPVPJailed {
		return true, nil
	}
	return false, err
}
