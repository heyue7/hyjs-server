package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// Each list revalidates one actual accepted winner, then shares that transaction
// snapshot across all five original rank conditions and any new title sources.
func TestP5ArenaTitleListOneWinnerReadPerRequest(t *testing.T) {
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
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 901, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}, 902, now); err != nil {
		t.Fatal(err)
	}
	reads := 0
	callback := "p5_arena_title_request_read_count"
	if err := db.GetIns().Callback().Query().Before("gorm:query").Register(callback, func(q *gorm.DB) {
		if q.Statement.Table == "p11_arena_members" {
			reads++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Query().Remove(callback) })
	for phase := 0; phase < 2; phase++ {
		reads = 0
		out, err := f.s.p5Titles(f.ctx, challenger)
		if err != nil {
			t.Fatal(err)
		}
		if reads != 1 {
			t.Fatal("winner proof was not read exactly once for this list", phase, reads)
		}
		rows := map[string]*pb.TitleInfo{}
		for _, v := range out[0].(*pb.SCTitleList).GetTitleList() {
			rows[v.GetTitleId()] = v
		}
		for _, id := range []string{"title_022", "title_023", "title_024", "title_025", "title_026"} {
			if rows[id].GetState() != 1 || rows[id].GetCurrent() != 1 {
				t.Fatal("rank/proof reuse changed original qualifications", phase, id, rows[id])
			}
		}
	}
}
