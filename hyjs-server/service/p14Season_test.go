package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestP14SeasonActual 截止本服名册冻结原阵容版本，排队上传/重启/换季不改变海选自然来源或冒充晋级。
func TestP14SeasonActualLocalCutoffAllStagesFrozenRosterLateUploadColdAndNextSeason(t *testing.T) {
	f, ids, _, now := p14Setup(t)
	base := now.Add(-90 * time.Minute)
	for stage := int32(1); stage <= 19; stage++ {
		start := base.Add(time.Duration(stage-1) * time.Hour)
		if err := db.GetIns().Create(&db.P14TimeSlot{SeasonIndex: 7, Stage: stage, StartAt: start.UnixMilli(), EndAt: start.Add(time.Hour).UnixMilli(), Description: fmt.Sprintf("完整原阶段%d", stage), Source: "隔离原全部19阶段明确日历"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	end := base.Add(2 * time.Hour)
	if err := db.GetIns().Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"register_start": base.Add(time.Hour).UnixMilli(), "register_end": end.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range ids[:3] {
		if _, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterGame{}, 10+int64(i), now); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, ids[0], func(p *pb.Player, g *model.GameState) error {
		p.Name = proto.String("真实截止前最后阵容")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 20, end.Add(-time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP14Season(f.ctx, end.Add(-time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P14AuditionBoundary{}).Count(&count)
	if count != 0 {
		t.Fatal("froze before exact original cutoff")
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- f.s.TickP14Season(f.ctx, end) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	} else {
		for i := 0; i < 8; i++ {
			if err := f.s.TickP14Season(f.ctx, end); err != nil {
				t.Fatal(err)
			}
		}
	}
	boundary, roster, err := p14AuditionRead(db.GetIns(), 7, 1)
	if err != nil || len(roster) != 3 || boundary.Count != 3 {
		t.Fatal("true local complete cutoff", err)
	}
	first := roster[0]
	formation := &pb.FormationSimpleInfo{}
	if proto.Unmarshal(first.Snapshot, formation) != nil || formation.GetName() != "真实截止前最后阵容" || first.Revision != 2 {
		t.Fatal("last actual snapshot revision before cutoff")
	}
	for _, id := range []int64{ids[0], ids[3]} {
		reply, err := f.s.p14Query(f.ctx, id, &pb.CSKFOpenFrightServerInfo{}, end)
		if err != nil {
			t.Fatal(err)
		}
		// Wire0 is not a membership boolean; real Login and frozen roster carry that fact.
		wanted := int32(0)
		if reply[0].(*pb.SCKFOpenFrightServerInfo).GetTypeStage() != 3 || reply[0].(*pb.SCKFOpenFrightServerInfo).GetIsEnterCurStage() != wanted {
			t.Fatal("real frozen audition membership", reply)
		}
	}
	if _, err = f.s.p14Register(f.ctx, ids[3], &pb.CSKFEnterGame{}, 40, end.Add(-time.Millisecond)); err == nil {
		t.Fatal("queued request with pre-cutoff internal clock added member after boundary seal")
	}
	if _, err = f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 41, end.Add(-time.Millisecond)); err == nil {
		t.Fatal("queued old-clock upload changed immutable audition")
	}
	if _, err = f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 20, end.Add(time.Hour)); err != nil {
		t.Fatal("old actual exact upload receipt not recovered", err)
	}
	audition := base.Add(3*time.Hour + time.Minute)
	reply, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFOpenFrightServerInfo{}, audition)
	if err != nil || reply[0].(*pb.SCKFOpenFrightServerInfo).GetIsEnterCurStage() != 0 {
		t.Fatal("original audition4 admission", err)
	}
	for stage := int32(5); stage <= 19; stage++ {
		reply, err = f.s.p14Query(f.ctx, ids[0], &pb.CSKFOpenFrightServerInfo{}, base.Add(time.Duration(stage-1)*time.Hour+time.Minute))
		if err == nil {
			t.Fatal("unverified promotion/finals inferred from signup", stage)
		}
	}
	// 原服2同本地ID可以有自己的名册；联合身份不能借用原服1已冻结的三人。
	f.s.ServerID = 2
	if err = f.s.TickP14Season(f.ctx, end); err != nil {
		t.Fatal(err)
	}
	second, other, err := p14AuditionRead(db.GetIns(), 7, 2)
	if err != nil || second.Count != 0 || len(other) != 0 {
		t.Fatal("empty actual second realm invented first-realm players", err)
	}
	f.s.ServerID = 1
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP14Cross(f.ctx); err != nil {
		t.Fatal(err)
	}
	old, restored, err := p14AuditionRead(db.GetIns(), 7, 1)
	if err != nil || old.SourceHash != boundary.SourceHash || restored[0].SourceHash != first.SourceHash {
		t.Fatal("cold changed complete cutoff snapshot", err)
	}
	if err = db.GetIns().Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"season_index": 8, "register_start": now.Add(24 * time.Hour).UnixMilli(), "register_end": now.Add(25 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickP14Season(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	old, _, err = p14AuditionRead(db.GetIns(), 7, 1)
	if err != nil || old.SourceHash != boundary.SourceHash {
		t.Fatal("new season polluted historical audition", err)
	}
	// 注册截止即使改一毫秒也必须被来源SHA发现，不能改变旧名册资格。
	if err = db.GetIns().Model(&db.P14AuditionBoundary{}).Where("season_index=7 AND server_id=1").Update("register_end", boundary.RegisterEnd+1).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = p14AuditionRead(db.GetIns(), 7, 1); err == nil {
		t.Fatal("tampered sealed deadline accepted")
	}
}
