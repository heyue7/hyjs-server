package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10ProfileCall(t *testing.T, f *p1Fixture, id, target int64) *pb.SCFindPlayerInfo {
	t.Helper()
	r, handled, err := f.s.DispatchP10Profile(f.ctx, id, &pb.CSFindPlayerInfo{PlayerId: proto.Int64(target), ServerId: proto.Int32(1)})
	if err != nil || !handled {
		t.Fatal(err)
	}
	return r[0].(*pb.SCFindPlayerInfo)
}
func TestP10ProfileActualFormationEquipmentAndTreasureProjection(t *testing.T) {
	f, b, _ := p10Setup(t)
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error {
		_, err := f.s.GrantFixedReward(p, g, "equip_013,1;trea_001,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var game *model.GameState
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { game = g; return nil }); err != nil {
		t.Fatal(err)
	}
	equip := game.Equipment[0]
	if _, err := f.s.equipmentFormationOn(f.ctx, b, &pb.CSFormationOn{Id: equip.EquipId, Guid: equip.Guid, FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	treasure := game.Treasures[0]
	if _, err := f.s.treasureFormationOn(f.ctx, b, &pb.CSFormationOn{Id: treasure.TreasureId, Guid: treasure.Guid, FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	before := p10ProfileCall(t, f, f.id, b)
	info := before.GetFormationInfo()
	if info.GetName() != "社交乙" || info.GetFightPower() <= 0 || len(info.GetFormationItemInfoList()) != 1 {
		t.Fatal("actual profile formation", info)
	}
	slot := info.GetFormationItemInfoList()[0]
	if slot.GetWarInfo().GetHeroId() != "hero_047" || slot.GetWarInfo().GetLevel() != 20 || slot.GetWarInfo().GetBaseProp().GetAtt() <= 0 || len(slot.GetEquipInfoList()) != 1 || slot.GetEquipInfoList()[0].GetEquipId() != equip.GetEquipId() || len(slot.GetTreaInfoList()) != 1 || slot.GetTreaInfoList()[0].GetTreasureId() != treasure.GetTreasureId() {
		t.Fatal("actual owned mounted instances", slot)
	}
	f.restart(t)
	if err := f.s.InitializeP10Social(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, p10ProfileCall(t, f, f.id, b)) {
		t.Fatal("profile changed after reconnect")
	}
}
func TestP10ProfileBlacklistCrossServerAndForeignInstanceGate(t *testing.T) {
	f, b, c := p10Setup(t)
	now := time.Now()
	p10Call(t, f, b, &pb.CSSendToBlackList{FriendId: proto.Int64(f.id)}, 1, now)
	if _, handled, err := f.s.DispatchP10Profile(f.ctx, f.id, &pb.CSFindPlayerInfo{PlayerId: proto.Int64(b), ServerId: proto.Int32(1)}); !handled || err == nil {
		t.Fatal("blocked detail visible")
	}
	if _, _, err := f.s.DispatchP10Profile(f.ctx, c, &pb.CSFindPlayerInfo{PlayerId: proto.Int64(b), ServerId: proto.Int32(2)}); err == nil {
		t.Fatal("foreign server")
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error {
		g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Pos: proto.Int32(1), Id: proto.String("equip_013"), Guid: proto.String("foreign-equipment-guid")})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.DispatchP10Profile(f.ctx, c, &pb.CSFindPlayerInfo{PlayerId: proto.Int64(b), ServerId: proto.Int32(1)}); err == nil {
		t.Fatal("foreign/unowned mounted equipment projected")
	}
}

func TestP10ProfileProtocolSeed(t *testing.T) {
	f, _, _ := p10Setup(t)
	a, err := LoginAccount(f.ctx, "p10-native-profile", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, a.Id, "原真实阵容资料", "hero_047"); err != nil {
		t.Fatal(err)
	}
	var game *model.GameState
	if err = updateProfile(f.ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		_, err := f.s.GrantFixedReward(p, g, "equip_013,1;trea_001,1")
		game = g
		return err
	}); err != nil {
		t.Fatal(err)
	}
	equip := game.Equipment[0]
	treasure := game.Treasures[0]
	if _, err = f.s.equipmentFormationOn(f.ctx, a.Id, &pb.CSFormationOn{Id: equip.EquipId, Guid: equip.Guid, FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.treasureFormationOn(f.ctx, a.Id, &pb.CSFormationOn{Id: treasure.TreasureId, Guid: treasure.Guid, FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
}

func TestP10SearchSkipsCorruptedCandidateWithoutFakePower(t *testing.T) {
	f, b, c := p10Setup(t)
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error {
		g.Formation[0].Things = append(g.Formation[0].Things, &pb.FormationThing{Pos: proto.Int32(1), Id: proto.String("equip_013"), Guid: proto.String("not-owned")})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, c, func(p *pb.Player, g *model.GameState) error { p.Name = proto.String("社交乙"); return nil }); err != nil {
		t.Fatal(err)
	}
	r := p10Call(t, f, f.id, &pb.CSSearchPlayer{IdOrName: proto.String("社交乙")}, 0, time.Now()).(*pb.SCSearchPlayer)
	found := false
	for _, info := range r.GetPlayer() {
		if info.GetPlayerId() == b {
			t.Fatal("corrupt candidate advertised valid power")
		}
		if info.GetPlayerId() == c && info.GetFightPower() > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("corrupt candidate prevented healthy real search")
	}
}
