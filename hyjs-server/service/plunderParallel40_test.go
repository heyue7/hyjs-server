package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestPlunderParallel40PublicationScopedAcceptedWitness exercises the activity
// boundary with finite accepted-settlement witnesses; it does not run the P6 engine.
func TestPlunderParallel40PublicationScopedAcceptedWitness(t *testing.T) {
	for _, proof := range []struct{ name, hash string }{
		{"native", constUtil.P6PlunderNativeProof},
		{"generic", constUtil.P11GenericPlunderCompatibilityProof},
	} {
		t.Run(proof.name, func(t *testing.T) {
			f, _ := adminModesFixture(t)
			f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
			if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().AutoMigrate(&db.P6BattleRecord{}); err != nil {
				t.Fatal(err)
			}
			var profile db.Profile
			if err := db.GetIns().First(&profile, f.id).Error; err != nil {
				t.Fatal(err)
			}
			bound, err := f.s.BindServerGameClockContext(f.ctx, &profile)
			if err != nil {
				t.Fatal(err)
			}
			now := LocalPlayNow(bound)
			calendar := db.PeriodInstance{ID: constUtil.P4PlunderActivityPrefix + "admin_parallel40", Enabled: true, Title: "并存夺宝有限夹具", Kind: constUtil.P4PlunderActivityKind, Mode: "cumulative", Goal: 2, Rewards: "coin,1", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(time.Hour).UnixMilli(), Source: "finite accepted witness activity fixture"}
			personal := calendar
			personal.ID = constUtil.P4PlunderActivityPrefix + "admin_character_parallel40"
			foreign := calendar
			foreign.ID = constUtil.P4PlunderActivityPrefix + "admin_character_foreign40"
			for _, cfg := range []db.PeriodInstance{calendar, personal, foreign} {
				if err := db.GetIns().Create(&cfg).Error; err != nil {
					t.Fatal(err)
				}
			}
			form := adminModeForm("plunder-count")
			raw, _ := json.Marshal(form)
			// Owned and foreign instances share an actual character-day source;
			// the visibility chain must reject the foreign consumer in this role's TX.
			source := db.AdminActivityOccurrence{ID: "parallel40-personal", ServerID: f.s.OriginalServerID(), TemplateID: "plunder-count-original", Timing: "character-day", Mode: "plunder-count", ConsumerKey: "parallel40-source", ConsumerID: "period:parallel40-source", Enabled: true, FormJSON: raw, StartAt: calendar.StartAt, EndAt: calendar.EndAt, ClaimEndAt: calendar.RewardEndAt}
			if err := db.GetIns().Create(&source).Error; err != nil {
				t.Fatal(err)
			}
			for _, issued := range []db.AdminCharacterActivityInstance{
				{PlayerID: f.id, OccurrenceID: source.ID, ServerID: source.ServerID, ConsumerID: "period:" + personal.ID, ConsumerKey: personal.ID, Mode: source.Mode, StartAt: personal.StartAt, EndAt: personal.EndAt, ClaimEndAt: personal.RewardEndAt, FormJSON: raw},
				{PlayerID: f.id + 999, OccurrenceID: source.ID, ServerID: source.ServerID, ConsumerID: "period:" + foreign.ID, ConsumerKey: foreign.ID, Mode: source.Mode, StartAt: foreign.StartAt, EndAt: foreign.EndAt, ClaimEndAt: foreign.RewardEndAt, FormJSON: raw},
			} {
				if err := db.GetIns().Create(&issued).Error; err != nil {
					t.Fatal(err)
				}
			}
			response := &pb.SCFightTreasureStartResult{FightId: proto.String("parallel40-" + proof.name), MaxRoundNum: proto.Int32(20), IsWin: proto.Int32(0)}
			wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			terminal := []byte("finite accepted settlement witness")
			row := db.P11PlunderFightSettlement{PlayerID: f.id, FightID: response.GetFightId(), P6RecordID: "parallel40-record-" + proof.name, ScopeProof: proof.hash, EnergySpent: 2, AttackerWon: false, CreatedAt: now.UnixMilli(), ResponseHash: p6Digest(wire), Terminal: terminal, TerminalHash: p6Digest(terminal)}
			witness := db.P6BattleRecord{ID: row.P6RecordID, PlayerID: f.id, Source: constUtil.P11PlunderFightSource, SourceFight: row.FightID, Status: constUtil.P6BattleComputed, Authoritative: true, FinishedAt: row.CreatedAt, Input: []byte("finite input witness")}
			if err := db.GetIns().Create(&witness).Error; err != nil {
				t.Fatal(err)
			}
			apply := func() error {
				return db.GetIns().WithContext(bound).Transaction(func(tx *gorm.DB) error {
					return f.s.recordP4VerifiedPlunderAttempt(tx, &row, response, now)
				})
			}
			for i := 0; i < 2; i++ {
				if err := apply(); err != nil {
					t.Fatal("accepted witness or receipt replay", err)
				}
			}
			for _, cfg := range []db.PeriodInstance{calendar, personal} {
				var progress db.PeriodProgress
				if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", cfg.ID, f.id).Error; err != nil || progress.Value != 1 {
					t.Fatal("one accepted attempt must count once per publication", cfg.ID, progress.Value, err)
				}
				if got := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, constUtil.P4PlunderActivityFactPrefix+cfg.ID+":"+row.FightID); got != 1 {
					t.Fatal("publication fact replayed", cfg.ID, got)
				}
			}
			if got := p1Count(t, &db.PeriodProgress{}, "instance_id=?", foreign.ID); got != 0 {
				t.Fatal("foreign character's period received progress", got)
			}
			if got := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4PlunderActivityKind); got != 2 {
				t.Fatal("unexpected publication fact count", got)
			}
			row.ScopeProof = "unknown-proof"
			if err := apply(); err == nil {
				t.Fatal("unknown accepted scope proof was permitted")
			}
			witness.Authoritative = false
			if err := db.GetIns().Save(&witness).Error; err != nil {
				t.Fatal(err)
			}
			row.ScopeProof = proof.hash
			if err := apply(); err == nil {
				t.Fatal("non-authoritative witness was permitted")
			}
		})
	}
}
