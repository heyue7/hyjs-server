package service

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

// Level100/20 are finite prerequisites. The real original047 arena calculation
// produces rank and proof; no title, winner, GUID, BestWonRank or effect is seeded.
func TestP5OriginalArenaWinnerTitlesWearAndCold(t *testing.T) {
	f, challenger := p11ArenaScopedSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, challenger, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 901, now); err != nil {
		t.Fatal(err)
	}
	entries := func(id int64) map[string]*pb.TitleInfo {
		t.Helper()
		out, err := f.s.p5Titles(f.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]*pb.TitleInfo{}
		for _, v := range out[0].(*pb.SCTitleList).GetTitleList() {
			m[v.GetTitleId()] = v
		}
		return m
	}
	wanted := []string{"title_022", "title_023", "title_024", "title_025", "title_026"}
	for _, id := range []int64{f.id, challenger} {
		for _, title := range wanted {
			if entries(id)[title].GetState() != 0 {
				t.Fatal("initial arena position granted title", id, title)
			}
		}
	}
	response, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}, 902, now)
	if err != nil {
		t.Fatal(err)
	}
	if response[0].(*pb.SCPvpFightResult).GetWinPlayerId() != challenger {
		t.Fatal("real strong attacker did not win")
	}
	for _, title := range wanted {
		v := entries(challenger)[title]
		if v.GetState() != 1 || v.GetCurrent() != 1 {
			t.Fatal("real rank1 title missing", title, v)
		}
		if entries(f.id)[title].GetState() != 0 {
			t.Fatal("loser inherited winner title", title)
		}
	}
	capture := func() *pb.BattlePlayer {
		t.Helper()
		p, g := p10LuckyProfile(t, challenger)
		b, err := f.s.battlePlayer(p, g)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	base := capture()
	props := []string{"hit", "dodge", "antiCri", "cri", "damageReduce"}
	amounts := []float64{2, 2, 2, 2, 3}
	for i, title := range wanted {
		if _, err = f.s.p5ChangeTitle(f.ctx, challenger, &pb.CSChangeTitle{TitleId: proto.String(title)}, int64(1000+i)); err != nil {
			t.Fatal(err)
		}
		current := capture()
		expected := proto.Clone(base).(*pb.BattlePlayer)
		prop := expected.Formation[0].Prop
		field := prop.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(props[i]))
		original := prop.ProtoReflect().Get(field).Float()
		prop.ProtoReflect().Set(field, protoreflect.ValueOfFloat64(original+amounts[i]))
		if !proto.Equal(expected, current) {
			t.Fatal("original title effect mismatch", title, expected, current)
		}
	}
	_, game := p10LuckyProfile(t, challenger)
	for _, title := range wanted {
		if !strings.Contains(p5KV(game, "p5.title:"+title), "proof=") {
			t.Fatal("verified source missing", title)
		}
	}
	current := capture()
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if !proto.Equal(current, capture()) {
		t.Fatal("cold title attributes changed")
	}
	for _, title := range wanted {
		if entries(challenger)[title].GetState() != 1 {
			t.Fatal("cold title missing", title)
		}
	}
	if _, err = f.s.p5ChangeTitle(f.ctx, challenger, &pb.CSChangeTitle{TitleId: proto.String("")}, 2000); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(base, capture()) {
		t.Fatal("clearing title did not restore actual base")
	}
}
