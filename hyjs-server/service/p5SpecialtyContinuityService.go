package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// specialtyInitializeTracked keeps a legitimately invested warrior usable
// when reaching the original specialty gate creates its free default records.
// Existing investment must match the exact before state; nil costs neither
// manufacture refundable inputs nor create a ledger for a pristine warrior.
func (s *GameService) specialtyInitializeTracked(g *model.GameState, w *pb.Warrior, rule *db.SkillSpecialtyRule) error {
	before := proto.Clone(w).(*pb.Warrior)
	if err := s.specialtyInitialize(w, rule); err != nil {
		return err
	}
	if proto.Equal(before, w) {
		return nil
	}
	return RecordP5Investment(g, w.GetGuid(), before, w, nil)
}
