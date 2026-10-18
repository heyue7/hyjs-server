package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Chapter/trial titles read completed original progress without producing wins.
// All saved chapter/stage prerequisites in this test are finite historical fixtures.
func TestP5OriginalJourneyTitles(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	titles := func() map[string]*pb.TitleInfo {
		t.Helper()
		out, err := f.s.p5Titles(f.ctx, f.id)
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]*pb.TitleInfo{}
		for _, r := range out[0].(*pb.SCTitleList).GetTitleList() {
			result[r.GetTitleId()] = r
		}
		return result
	}
	if row := titles()["title_019"]; row.GetCurrent() != 0 || row.GetTotal() != 30 || row.GetState() != 0 {
		t.Fatal("absent trial schema manufactured qualification")
	}
	var missions []*pb.CopySimpleInfo
	for _, r := range f.s.Tables["CopyMission"] {
		if r["ChapterId"] == "10" {
			missions = append(missions, &pb.CopySimpleInfo{ChapterId: proto.String("10"), MissionId: proto.String(r["MissionId"]), Star: proto.Int32(1)})
		}
	}
	if len(missions) != 6 {
		t.Fatal("original chapter10 mission count changed")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Copy = &pb.SCCopyPassedInfo{CurrentChapterId: proto.String("20"), PassedChapterInfo: []*pb.CopyChapterInfo{{ChapterId: proto.String("20"), Star: proto.Int32(25)}}, PassedCopyInfo: missions[:len(missions)-1]}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if row := titles()["title_013"]; row.GetState() != 0 || row.GetCurrent() != 0 {
		t.Fatal("open chapter or summary stars used as completed chapter", row)
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, missions[len(missions)-1])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	qualified := titles()
	if qualified["title_013"].GetCurrent() != 10 || qualified["title_013"].GetState() != 1 || qualified["title_014"].GetState() != 0 {
		t.Fatal("complete original10 title missing or20 granted early")
	}
	if err := db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	state := db.P7RushState{PlayerID: f.id, Day: "2026-10-15", RunID: 1, Floor: 30, HighFloor: 29}
	if err := db.GetIns().Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if row := titles()["title_019"]; row.GetCurrent() != 29 || row.GetState() != 0 {
		t.Fatal("current uncompleted floor confused with highest completed floor", row)
	}
	state.HighFloor = 30
	state.Floor = 1
	if err := db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	qualified = titles()
	if qualified["title_019"].GetCurrent() != 30 || qualified["title_019"].GetState() != 1 || qualified["title_020"].GetState() != 0 || qualified["title_021"].GetState() != 0 {
		t.Fatal("original highest30 qualification or60/100 guard differs")
	}
	p, g := f.profile(t)
	before, err := f.s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	for i, choice := range []struct {
		id      string
		hp, att float64
	}{{"title_013", 500, 0}, {"title_019", 0, 50}} {
		if _, err = f.s.p5ChangeTitle(f.ctx, f.id, &pb.CSChangeTitle{TitleId: proto.String(choice.id)}, int64(200+i)); err != nil {
			t.Fatal(err)
		}
		p, g = f.profile(t)
		after, e := f.s.battlePlayer(p, g)
		if e != nil {
			t.Fatal(e)
		}
		a, b := after.GetFormation()[0].GetProp(), before.GetFormation()[0].GetProp()
		if a.GetHp()-b.GetHp() != choice.hp || a.GetAtt()-b.GetAtt() != choice.att {
			t.Fatal("original milestone effect differs", choice.id)
		}
	}
	f.restart(t)
	qualified = titles()
	if qualified["title_013"].GetState() != 1 || qualified["title_019"].GetState() != 1 {
		t.Fatal("cold journey titles missing")
	}
	var after db.P7RushState
	if err = db.GetIns().First(&after, "player_id=?", f.id).Error; err != nil || after != state {
		t.Fatal("title query changed owner trial state", err)
	}
	// Reading qualification directly keeps the owner's reset/current stage untouched.
	tx := db.GetIns().WithContext(f.ctx)
	p, g = f.profile(t)
	beforeKV := len(g.KV)
	if _, err = f.s.p5TitleListTx(tx, p, g, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(g.KV) != beforeKV {
		t.Fatal("repeat list duplicated title state")
	}
}
