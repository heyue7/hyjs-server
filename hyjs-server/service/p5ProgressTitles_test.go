package service

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP5OriginalProgressTitles covers original level and craft milestones without competitive results.
func TestP5OriginalProgressTitles(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	titles := func() map[string]*pb.TitleInfo {
		t.Helper()
		out, err := f.s.p5Titles(f.ctx, f.id)
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]*pb.TitleInfo{}
		for _, info := range out[0].(*pb.SCTitleList).GetTitleList() {
			result[info.GetTitleId()] = info
		}
		return result
	}
	fresh := titles()
	if fresh["title_009"].GetState() != 0 || fresh["title_009"].GetCurrent() != 1 || fresh["title_015"].GetCurrent() != 1 || fresh["title_017"].GetCurrent() != 1 {
		t.Fatal("fresh actual levels or title threshold differs")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(25); return nil }); err != nil {
		t.Fatal(err)
	}
	current := titles()
	if current["title_009"].GetState() != 1 || current["title_009"].GetTotal() != 25 || current["title_010"].GetState() != 0 {
		t.Fatal("original25 title qualification missing or50 granted early")
	}
	p, g := f.profile(t)
	before, err := f.s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p5ChangeTitle(f.ctx, f.id, &pb.CSChangeTitle{TitleId: proto.String("title_009")}, 100); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	after, err := f.s.battlePlayer(p, g)
	if err != nil || after.GetFormation()[0].GetProp().GetPdef()-before.GetFormation()[0].GetProp().GetPdef() != 10 || after.GetFormation()[0].GetProp().GetMdef()-before.GetFormation()[0].GetProp().GetMdef() != 10 {
		t.Fatal("original title009 actual battle defenses missing", err)
	}
	for _, state := range []any{&db.GemCraftState{PlayerID: f.id, Level: 5}, &db.CookingCraftState{PlayerID: f.id, Level: 3}} {
		if err = db.GetIns().Save(state).Error; err != nil {
			t.Fatal(err)
		}
	}
	current = titles()
	if current["title_015"].GetState() != 1 || current["title_016"].GetState() != 0 || current["title_017"].GetState() != 1 || current["title_018"].GetState() != 0 {
		t.Fatal("original jewelry5/cooking3 thresholds missing or highest granted early")
	}
	f.restart(t)
	current = titles()
	if current["title_009"].GetState() != 1 || current["title_015"].GetState() != 1 || current["title_017"].GetState() != 1 {
		t.Fatal("cold milestone title lost")
	}
	if _, err = f.s.p5ChangeTitle(f.ctx, f.id, &pb.CSChangeTitle{TitleId: proto.String("")}, 101); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	reset, err := f.s.battlePlayer(p, g)
	if err != nil || !proto.Equal(reset, before) {
		t.Fatal("clearing title failed to restore original actor", err)
	}
}
