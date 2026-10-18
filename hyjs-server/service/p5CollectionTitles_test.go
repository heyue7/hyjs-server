package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The original visible handbook defines achievable world/color title cohorts.
// A real guide recruitment supplies the last purple/ALO card; other histories
// are finite qualification fixtures and preserve their first acquisition dates.
func TestP5OriginalCollectionTitles(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, hero := range f.s.Tables["Hero"] {
		if hero["id"] == "hero_025" {
			continue
		}
		if err := db.GetIns().Create(&db.P5Collection{PlayerID: f.id, HeroID: hero["id"], AcquiredAt: 12345}).Error; err != nil {
			t.Fatal(err)
		}
	}
	titles := func() map[string]*pb.TitleInfo {
		t.Helper()
		out, err := f.s.p5Titles(f.ctx, f.id)
		if err != nil {
			t.Fatal(err)
		}
		rows := map[string]*pb.TitleInfo{}
		for _, row := range out[0].(*pb.SCTitleList).GetTitleList() {
			rows[row.GetTitleId()] = row
		}
		return rows
	}
	before := titles()
	expected := map[string]int32{"title_002": 21, "title_003": 19, "title_004": 22, "title_005": 10, "title_006": 13, "title_007": 26, "title_008": 16}
	for id, total := range expected {
		current := total
		state := int32(1)
		if id == "title_003" || id == "title_006" {
			current--
			state = 0
		}
		if r := before[id]; r.GetTotal() != total || r.GetCurrent() != current || r.GetState() != state {
			t.Fatalf("cohort %s: %v", id, r)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { g.Quests = append(g.Quests, "quest_1"); return nil }); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.recruitGuide(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	actual := out[0].(*pb.SCWarriorRecruitGuide).GetGet().GetWars()
	if len(actual) != 1 || actual[0].GetId() != "hero_025" {
		t.Fatal("original guide card missing")
	}
	after := titles()
	for id, total := range expected {
		if r := after[id]; r.GetTotal() != total || r.GetCurrent() != total || r.GetState() != 1 {
			t.Fatalf("actual last card %s: %v", id, r)
		}
	}
	p, g := f.profile(t)
	base, err := f.s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	for i, option := range []struct {
		id      string
		att, hp float64
	}{{"title_005", 0, 100}, {"title_006", 100, 0}, {"title_003", 300, 0}} {
		if _, err = f.s.p5ChangeTitle(f.ctx, f.id, &pb.CSChangeTitle{TitleId: proto.String(option.id)}, int64(100+i)); err != nil {
			t.Fatal(err)
		}
		p, g = f.profile(t)
		actor, e := f.s.battlePlayer(p, g)
		if e != nil {
			t.Fatal(e)
		}
		a, b := actor.GetFormation()[0].GetProp(), base.GetFormation()[0].GetProp()
		if a.GetAtt()-b.GetAtt() != option.att || a.GetHp()-b.GetHp() != option.hp {
			t.Fatal("original title battle effects differ", option.id, a, b)
		}
	}
	f.restart(t)
	after = titles()
	if after["title_003"].GetState() != 1 || after["title_006"].GetState() != 1 {
		t.Fatal("cold collection title lost")
	}
	var old db.P5Collection
	if err = db.GetIns().First(&old, "player_id=? AND hero_id=?", f.id, "hero_011").Error; err != nil || old.AcquiredAt != 12345 {
		t.Fatal("historical collection rewritten", err)
	}
}
