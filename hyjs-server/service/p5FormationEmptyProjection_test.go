package service

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原UI只初始化有角色的阵位；真实卸阵、重连和再次上阵保留库存及内部空槽。
func TestP5FormationListOriginalEmptySlotOffAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	// 有限等级/伙伴只隔离此原回包契约，不声称本例证明自然招募来源。
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		_, err := f.s.GrantFixedReward(p, g, "hero_020,1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	var partner *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_020" {
			partner = w
		}
	}
	if partner == nil {
		t.Fatal("fixture partner missing")
	}
	on := &pb.CSFormationOn{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.formationOn(f.ctx, f.id, on); err != nil {
		t.Fatal(err)
	}
	off := &pb.CSFormationOff{Id: partner.Id, Guid: partner.Guid, FormatPos: proto.Int32(2)}
	if _, err := f.s.formationOff(f.ctx, f.id, off); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		_, current := f.profile(t)
		before, err := json.Marshal(current)
		if err != nil {
			t.Fatal(err)
		}
		out, err := f.s.formationList(f.ctx, f.id)
		if err != nil {
			t.Fatal(err)
		}
		view := out[0].(*pb.SCFormationList)
		if len(view.GetFormationList()) != 1 || formationHero(view.FormationList[0]).GetGuid() != p.GetHeroWarId() {
			t.Fatal("original20111 projection contains an empty or wrong hero cell")
		}
		raw, err := proto.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		decoded := &pb.SCFormationList{}
		if err = proto.Unmarshal(raw, decoded); err != nil || !proto.Equal(decoded, view) {
			t.Fatal("original20111 roundtrip", err)
		}
		_, after := f.profile(t)
		afterRaw, _ := json.Marshal(after)
		if string(before) != string(afterRaw) || len(after.Formation) != 2 {
			t.Fatal("projection changed inventory or removed the internal empty slot")
		}
	}
	check()
	f.restart(t)
	check()
	if _, err := f.s.formationOff(f.ctx, f.id, off); err != nil {
		t.Fatal("retry unloaded same owned hero", err)
	}
	check()
	if _, err := f.s.formationOff(f.ctx, f.id, &pb.CSFormationOff{Id: proto.String("hero_043"), Guid: p.HeroWarId, FormatPos: proto.Int32(1)}); err == nil {
		t.Fatal("main hero removal accepted")
	}
	if _, err := f.s.formationOn(f.ctx, f.id, on); err != nil {
		t.Fatal("real reformation after cold", err)
	}
	out, err := f.s.formationList(f.ctx, f.id)
	if err != nil || len(out[0].(*pb.SCFormationList).FormationList) != 2 {
		t.Fatal("real nonempty slot did not return", err)
	}
}
