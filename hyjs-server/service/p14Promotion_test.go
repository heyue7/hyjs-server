package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p14PromotionSetup 真实本服报名/cutoff/auto资格，隔离服务时钟和原阶段日历明确放在历史日期。
func p14PromotionSetup(t *testing.T) (*p1Fixture, []int64, time.Time, time.Time) {
	t.Helper()
	f, ids, _, _ := p14Setup(t)
	location := time.FixedZone("UTC+8", 8*3600)
	today := time.Now().In(location)
	base := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -2)
	start := base.Add(6 * time.Hour)
	end := start.AddDate(0, 0, 4)
	for stage := int32(1); stage <= 19; stage++ {
		from, to := base.Add(time.Duration(stage-1)*time.Hour), base.Add(time.Duration(stage)*time.Hour)
		if stage == constUtil.P14PromotionStage {
			from, to = start, end
		} else if stage > constUtil.P14PromotionStage {
			from = end.Add(time.Duration(stage-constUtil.P14PromotionStage-1) * time.Hour)
			to = from.Add(time.Hour)
		}
		if err := db.GetIns().Create(&db.P14TimeSlot{SeasonIndex: 7, Stage: stage, StartAt: from.UnixMilli(), EndAt: to.UnixMilli(), Description: fmt.Sprintf("原阶段%d", stage), Source: "isolated historical explicit promotion clock"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"register_start": base.Add(time.Hour).UnixMilli(), "register_end": base.Add(2 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range ids[:3] {
		if _, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterGame{}, int64(100+i), base.Add(90*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP14Season(f.ctx, base.Add(3*time.Hour+time.Minute)); err != nil {
		t.Fatal(err)
	}
	return f, ids, start, end
}
func p14PromotionInfo(t *testing.T, f *p1Fixture, id int64, at time.Time) *pb.SCKFOpenFrightServerInfo {
	t.Helper()
	out, err := f.s.p14Query(f.ctx, id, &pb.CSKFOpenFrightServerInfo{}, at)
	if err != nil {
		t.Fatal(err)
	}
	return out[0].(*pb.SCKFOpenFrightServerInfo)
}

func TestP14PromotionActualQualificationFirstNoonConcurrentCapCrossDayColdAndEnd(t *testing.T) {
	f, ids, start, end := p14PromotionSetup(t)
	id := ids[0]
	at := start.Add(5*time.Hour + 59*time.Minute)
	info := p14PromotionInfo(t, f, id, at)
	if info.GetTypeStage() != constUtil.P14PromotionStage || info.GetIsEnterCurStage() != 0 || info.GetMateTime() != 0 {
		t.Fatal("default promotion gate")
	}
	var count int64
	db.GetIns().Model(&db.P14PromotionState{}).Count(&count)
	if count != 0 {
		t.Fatal("default created state")
	}
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	info = p14PromotionInfo(t, f, id, at)
	if info.GetIsEnterCurStage() != 0 || info.GetMateTime() != 0 {
		t.Fatal("first noon fabricated early points")
	}
	outsider := p14PromotionInfo(t, f, ids[4], at)
	if outsider.GetIsEnterCurStage() != 0 || outsider.GetMateTime() != 0 {
		t.Fatal("unregistered source entered promotion")
	}
	noon := start.Add(6 * time.Hour)
	workers := 1
	if f.mysql.Database != "" {
		workers = 8
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.p14Query(f.ctx, id, &pb.CSKFOpenFrightServerInfo{}, noon)
			if err == nil {
				value := out[0].(*pb.SCKFOpenFrightServerInfo)
				if value.GetIsEnterCurStage() != 0 || value.GetMateTime() != 20 {
					t.Error("actual original point frame")
				}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	db.GetIns().Model(&db.P14PromotionGrant{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate actual noon grant", count)
	}
	var state db.P14PromotionState
	if err := db.GetIns().First(&state).Error; err != nil || state.Score != 1 || state.Points != 20 {
		t.Fatal("original initial score/actual point cap", err)
	}
	if got := p14PromotionInfo(t, f, id, noon.Add(time.Hour)); got.GetMateTime() != 20 {
		t.Fatal("same noon changed points")
	}
	var repeated db.P14PromotionState
	db.GetIns().First(&repeated)
	if state.SourceHash != repeated.SourceHash {
		t.Fatal("same grant was rewritten")
	}
	next := noon.AddDate(0, 0, 1)
	if got := p14PromotionInfo(t, f, id, next); got.GetMateTime() != 20 {
		t.Fatal("next day exceeded original cap")
	}
	db.GetIns().Model(&db.P14PromotionGrant{}).Count(&count)
	if count != 2 {
		t.Fatal("next noon source omitted")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.InitializeP14Cross(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := p14PromotionInfo(t, f, id, next.Add(time.Hour)); got.GetMateTime() != 20 || got.GetIsEnterCurStage() != 0 {
		t.Fatal("cold point/eligibility changed")
	}
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if got := p14PromotionInfo(t, f, id, next.Add(2*time.Hour)); got.GetIsEnterCurStage() != 0 || got.GetMateTime() != 0 {
		t.Fatal("closed point gate exposed active state")
	}
	db.GetIns().Model(&db.P14PromotionGrant{}).Count(&count)
	if count != 2 {
		t.Fatal("closed policy granted")
	}
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	f.s.ServerID = 2
	if got := p14PromotionInfo(t, f, id, next.Add(time.Hour)); got.GetIsEnterCurStage() != 0 || got.GetMateTime() != 0 {
		t.Fatal("same localID borrowed foreign qualification")
	}
	f.s.ServerID = 1
	if _, err := f.s.p14Query(f.ctx, id, &pb.CSKFOpenFrightServerInfo{}, end); err == nil {
		t.Fatal("unverified finals emitted a success or fake failure stage")
	}
	db.GetIns().Model(&db.P14PromotionGrant{}).Count(&count)
	if count != 2 {
		t.Fatal("end retroactively granted beyond active query")
	}
}

func TestP14PromotionActualLedgerMissingOrRetaggedBalanceFailsWithoutRepair(t *testing.T) {
	f, ids, start, _ := p14PromotionSetup(t)
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	at := start.Add(6 * time.Hour)
	p14PromotionInfo(t, f, ids[0], at)
	var state db.P14PromotionState
	db.GetIns().First(&state)
	state.Points = 0
	state.SourceHash = p14PromotionStateHash(&state)
	db.GetIns().Save(&state)
	if _, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFOpenFrightServerInfo{}, at.Add(time.Hour)); err == nil {
		t.Fatal("retagged zero balance concealed missing consumed source")
	}
	state.Points = 20
	state.SourceHash = p14PromotionStateHash(&state)
	db.GetIns().Save(&state)
	if err := db.GetIns().Where("season_index=? AND server_id=? AND player_id=?", state.SeasonIndex, state.ServerID, state.PlayerID).Delete(&db.P14PromotionGrant{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFOpenFrightServerInfo{}, at.AddDate(0, 0, 1)); err == nil {
		t.Fatal("missing actual noon source silently repaired")
	}
	var count int64
	db.GetIns().Model(&db.P14PromotionGrant{}).Count(&count)
	if count != 0 {
		t.Fatal("failed source wrote replacement grant")
	}
}
