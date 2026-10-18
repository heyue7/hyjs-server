package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14AuditionMatchSetup(t *testing.T, count int) (*p1Fixture, time.Time) {
	t.Helper()
	f, ids, _, now := p14Setup(t)
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("source=?", constUtil.P14AuditionBattleSource).Delete(&db.P6BattleRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	for len(ids) < count {
		login, err := LoginAccount(f.ctx, fmt.Sprintf("p14-match-%s-%d-%d", t.Name(), len(ids), time.Now().UnixNano()), "isolated-local-pass")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CreatePlayer(f.ctx, login.Id, fmt.Sprintf("海选真实%d", len(ids)), "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err := updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(70); return nil }); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, login.Id)
	}
	base := now.Add(-90 * time.Minute)
	for stage := int32(1); stage <= 19; stage++ {
		start := base.Add(time.Duration(stage-1) * time.Hour)
		if err := db.GetIns().Create(&db.P14TimeSlot{SeasonIndex: 7, Stage: stage, StartAt: start.UnixMilli(), EndAt: start.Add(time.Hour).UnixMilli(), Description: fmt.Sprintf("原阶段%d", stage), Source: "隔离真实海选19阶段发布时间"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	end := base.Add(2 * time.Hour)
	if err := db.GetIns().Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"register_start": base.Add(time.Hour).UnixMilli(), "register_end": end.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range ids[:count] {
		if _, err := f.s.p14Register(f.ctx, id, &pb.CSKFEnterGame{}, int64(100+i), now); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.TickP14Season(f.ctx, end); err != nil {
		t.Fatal(err)
	}
	return f, base.Add(3*time.Hour + time.Minute)
}

// TestP14AuditionActual 验证11个真实角色随机无重复配对、实际整场计算/重放和严格权威消费门禁。
func TestP14AuditionActualRandomElevenOriginalP6ComputedAuthorityGateColdAndCorruption(t *testing.T) {
	f, now := p14AuditionMatchSetup(t, 11)
	if _, err := f.s.PrepareP14AuditionRound(f.ctx, now); err == nil {
		t.Fatal("default matching gate open")
	}
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var results []*db.P14AuditionRound
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		var mu sync.Mutex
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := f.s.PrepareP14AuditionRound(f.ctx, now)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
				errs <- e
			}()
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
			r, err := f.s.PrepareP14AuditionRound(f.ctx, now)
			if err != nil {
				t.Fatal(err)
			}
			results = append(results, r)
		}
	}
	for _, round := range results {
		if round == nil || round.RoundIndex != 1 || round.SourceHash != results[0].SourceHash {
			t.Fatal("concurrent duplicate random rounds")
		}
	}
	var ids []int64
	if json.Unmarshal(results[0].Participants, &ids) != nil || len(ids) != 11 {
		t.Fatal("missing odd participant")
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatal("duplicate random member")
		}
		seen[id] = true
	}
	boundary, roster, err := p14AuditionRead(db.GetIns(), 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := f.s.p14AuditionRoundRead(f.ctx, db.GetIns(), results[0], boundary, roster)
	if err != nil || len(matches) != 5 {
		t.Fatal("actual whole pair source", err)
	}
	first := matches[0]
	outcome, err := f.s.ComputeP14AuditionMatch(f.ctx, first.ID)
	if err != nil || outcome == nil || outcome.Authoritative || outcome.Winner < 0 || outcome.Winner > 1 {
		t.Fatal("actual diagnostic computation without authority", err)
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", first.ReplayID).Error; err != nil {
		t.Fatal(err)
	}
	if record.Status != constUtil.P6BattleComputed || record.Authoritative || len(record.Operates) == 0 || record.InputHash != first.InputHash {
		t.Fatal("actual persisted P6 calculation")
	}
	if err := db.GetIns().First(&first, "id=?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.Applied || first.WinnerID != 0 || first.OutcomeHash != "" {
		t.Fatal("diagnostic result applied loss or winner")
	}
	retry, err := f.s.ComputeP14AuditionMatch(f.ctx, first.ID)
	if err != nil || !proto.Equal(retry.Operates, outcome.Operates) || retry.Winner != outcome.Winner {
		t.Fatal("diagnostic retry changed original result", err)
	}
	// 实际P14 worker处理剩余四场，全部保留诊断录像而不产生淘汰或晋级。
	if err := f.s.TickP14Season(f.ctx, now); err != nil {
		t.Fatal("actual P14 lifecycle", err)
	}
	var computed int64
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("source=? AND status=?", constUtil.P14AuditionBattleSource, constUtil.P6BattleComputed).Count(&computed).Error; err != nil || computed != 5 {
		t.Fatal("lifecycle omitted actual pending matches", computed, err)
	}
	var qualifications int64
	if err := db.GetIns().Model(&db.P14AuditionQualification{}).Count(&qualifications).Error; err != nil || qualifications != 0 {
		t.Fatal("diagnostic matches granted qualification", err)
	}
	if _, err := f.s.PrepareP14AuditionRound(f.ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P14AuditionRound{}).Count(&count)
	if count != 1 {
		t.Fatal("non-authoritative round started next round")
	}
	f.s.ServerID = 2
	if _, err := f.s.ComputeP14AuditionMatch(f.ctx, first.ID); err == nil {
		t.Fatal("foreign realm accepted local match")
	}
	f.s.ServerID = 1
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.InitializeP14Cross(f.ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.PrepareP14AuditionRound(f.ctx, now)
	if err != nil || restored.SourceHash != results[0].SourceHash {
		t.Fatal("cold changed frozen cryptographic random pairing", err)
	}
	retry, err = f.s.ComputeP14AuditionMatch(f.ctx, first.ID)
	if err != nil || !proto.Equal(retry.Operates, outcome.Operates) {
		t.Fatal("cold changed actual replay", err)
	}
	if err := db.GetIns().Model(&db.P14AuditionMatch{}).Where("id=?", first.ID).Updates(map[string]any{"applied": true, "winner_id": first.AttackerID, "outcome_hash": record.OutcomeHash}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.PrepareP14AuditionRound(f.ctx, now); err == nil {
		t.Fatal("manual diagnostic result bypassed authority gate")
	}
	if err := db.GetIns().Model(&db.P14AuditionMatch{}).Where("id=?", first.ID).Updates(map[string]any{"applied": false, "winner_id": 0, "outcome_hash": "", "seed": first.Seed + 1}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ComputeP14AuditionMatch(f.ctx, first.ID); err == nil {
		t.Fatal("tampered original pairing seed accepted")
	}
}

func TestP14AuditionActualSmallRosterOriginalAutoQualificationNoFakeByeOrVictory(t *testing.T) {
	f, now := p14AuditionMatchSetup(t, 3)
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	round, err := f.s.PrepareP14AuditionRound(f.ctx, now)
	if err != nil || round != nil {
		t.Fatal("small roster fabricated match", err)
	}
	var count int64
	db.GetIns().Model(&db.P14AuditionMatch{}).Count(&count)
	if count != 0 {
		t.Fatal("small roster fake opponent")
	}
	qualified, err := f.s.p14AuditionQualificationRead(f.ctx, db.GetIns(), 7, 1)
	if err != nil || qualified.RoundCount != 0 {
		t.Fatal("original auto promotion not sealed", err)
	}
	var players []int64
	if json.Unmarshal(qualified.Players, &players) != nil || len(players) != 3 {
		t.Fatal("auto promotion not actual local cutoff list")
	}
	for _, id := range players {
		replies, err := f.s.p14AuditionResult(f.ctx, id, now)
		if err != nil {
			t.Fatal(err)
		}
		reply := replies[0].(*pb.SCKuaFuMatchResult)
		if reply.GetResult() != 1 || reply.GetMatchResult().GetTypeStage() != 1 || reply.GetMatchResult().GetWinTimes() != 0 || reply.GetMatchResult().GetFailTimes() != 0 {
			t.Fatal("automatic promotion manufactured wins")
		}
		for _, sub := range []int32{1, 2} {
			replays, err := f.s.p14AuditionAutoReplayInfo(f.ctx, id, &pb.CSKFReplayInfo{Type: proto.Int32(1), SubType: proto.Int32(sub)}, now)
			if err != nil {
				t.Fatal(err)
			}
			info := replays[0].(*pb.SCKFReplayInfo)
			if len(info.Replays) != 0 || info.GetPageNum() != 1 || info.GetSubType() != sub {
				t.Fatal("original auto pass invented replay or invalid empty paging")
			}
		}
		// Actual stage6 now reaches the independent promotion point gate.
		if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
		for _, step := range []time.Duration{time.Hour, 2 * time.Hour} {
			messages, err := f.s.p14Query(f.ctx, id, &pb.CSKFOpenFrightServerInfo{}, now.Add(step))
			if err != nil || messages[0].(*pb.SCKFOpenFrightServerInfo).GetIsEnterCurStage() != 0 {
				t.Fatal("actual auto promotion lacks original stage5/6 qualification", err)
			}
		}
	}
	if err := f.s.TickP14Season(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	var repeated db.P14AuditionQualification
	if err := db.GetIns().First(&repeated, "season_index=7 AND server_id=1").Error; err != nil || repeated.SourceHash != qualified.SourceHash {
		t.Fatal("worker reissued automatic promotion", err)
	}
	f.s.ServerID = 2
	if err := f.s.TickP14Season(f.ctx, now); err != nil {
		t.Fatal(err)
	}
	var empty int64
	db.GetIns().Model(&db.P14AuditionQualification{}).Where("server_id=2").Count(&empty)
	if empty != 0 {
		t.Fatal("zero real second-realm roster granted borrowed auto promotion")
	}
	f.s.ServerID = 1
	if err := db.GetIns().Model(&db.P14AuditionQualification{}).Where("season_index=7 AND server_id=1").Update("round_count", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p14AuditionResult(f.ctx, players[0], now); err == nil {
		t.Fatal("tampered automatic promotion source accepted")
	}
}
