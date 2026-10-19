package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestFormationSideHPRemapPreservesDeadLegacyAndEmpty(t *testing.T) {
	hero := func(camp, board int32) *pb.FormationItem {
		return &pb.FormationItem{FormatPos: proto.Int32(camp), BattlePos: proto.Int32(board), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String("hero_020"), Guid: proto.String("owned")}}}
	}
	for _, dead := range []bool{false, true} {
		g := &model.GameState{Formation: []*pb.FormationItem{hero(1, 3), hero(2, 7)}, Field: &model.FieldState{Side: &pb.PlayerSide{CurHp: proto.Float64(0.3), Anger: proto.Float64(9), State: proto.Int32(0), BattlePosHps: []*pb.BattlePosHp{{BattlePos: proto.Int32(3), HpPercent: proto.Float64(0)}, {BattlePos: proto.Int32(7), HpPercent: proto.Float64(0.6)}}}}}
		if dead {
			g.Field.Side.CurHp = proto.Float64(0)
			g.Field.Side.State = proto.Int32(5)
			g.Field.Side.BattlePosHps[1].HpPercent = proto.Float64(0)
		}
		old := proto.Clone(g.Field.Side).(*pb.PlayerSide)
		if err := syncFormationSideHP(g, map[int32]int32{1: 7, 2: 3}); err != nil {
			t.Fatal(err)
		}
		if g.Field.Side.GetBattlePos7() != 0 || g.Field.Side.GetBattlePos3() != old.BattlePosHps[1].GetHpPercent() || g.Field.Side.GetCurHp() != old.GetCurHp() || g.Field.Side.GetState() != old.GetState() || g.Field.Side.GetAnger() != old.GetAnger() {
			t.Fatal("formation remap changed owned life or revived a fallen hero", g.Field.Side)
		}
		g.Formation[0].BattlePos, g.Formation[1].BattlePos = proto.Int32(7), proto.Int32(3)
		already := proto.Clone(g.Field.Side)
		if err := syncFormationSideHP(g, map[int32]int32{1: 7, 2: 3}); err != nil || !proto.Equal(already, g.Field.Side) {
			t.Fatal("repeated profile preparation changed HP", err)
		}
	}
	legacy := &model.GameState{Formation: []*pb.FormationItem{hero(1, 3)}, Field: &model.FieldState{Side: &pb.PlayerSide{CurHp: proto.Float64(0.25)}}}
	if err := syncFormationSideHP(legacy, map[int32]int32{1: 13}); err != nil || legacy.Field.Side.GetBattlePos13() != 0.25 || legacy.Field.Side.GetCurHp() != 0.25 {
		t.Fatal("legacy aggregate life was not retained", err)
	}
	if err := syncFormationSideHP(&model.GameState{}, nil); err != nil {
		t.Fatal("no field side requires no fabricated HP", err)
	}
	if raw, err := model.EncodeGameResponses(nil); err != nil || len(raw) != 0 {
		t.Fatal("original one-way town notification must encode an empty transport write", err)
	}
}
