package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP5WorldHeroTransport(t *testing.T) {
	p := &pb.Player{HeroWarId: proto.String("original")}
	g := &model.GameState{Warriors: []*pb.Warrior{{Id: proto.String("hero_043"), Guid: proto.String("original")}, {Id: proto.String("hero_020"), Guid: proto.String("replacement")}}, Formation: []*pb.FormationItem{{FormatPos: proto.Int32(1), Things: []*pb.FormationThing{{Id: proto.String("hero_020"), Guid: proto.String("replacement"), Pos: proto.Int32(0)}}}}}
	if id, err := P5WorldHeroID(p, g); err != nil || id != "hero_020" || p.GetHeroWarId() != "original" {
		t.Fatal("camp avatar/identity", id, err)
	}
	g.Formation[0].Things[0].Guid = proto.String("foreign")
	if _, err := P5WorldHeroID(p, g); err == nil {
		t.Fatal("foreign avatar accepted")
	}
	g.Formation = nil
	if id, err := P5WorldHeroID(p, g); err != nil || id != "hero_043" {
		t.Fatal("legacy owned original", id, err)
	}
}
