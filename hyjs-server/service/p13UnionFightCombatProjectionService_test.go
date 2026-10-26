package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p13CombatProjectionTrace struct {
	logger.Interface
	SQL  []string
	Rows []int64
}

func (trace *p13CombatProjectionTrace) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	trace.SQL = append(trace.SQL, sql)
	trace.Rows = append(trace.Rows, rows)
	trace.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

// These are display/countdown metadata fixtures, not fabricated P6 results or
// authority proofs. No battle outcome, winner, P6 record, wallet or reward is
// inserted. Real producer/terminal coverage remains in the lifecycle suites.
func p13CombatProjectionSetup(t *testing.T, scope string) (*p1Fixture, *db.P13UnionFightPlayerBattleOffer) {
	t.Helper()
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.P13UnionFightPlayerBattleOffer{}, &db.P13UnionFightPlayerBattleAttempt{}, &db.P13UnionFightGuardBattleAttempt{}); err != nil {
		t.Fatal(err)
	}
	rules := p13UFPlayerBattle172Rules()
	rules.MatchID = scope
	raw, _ := json.Marshal(rules)
	offer := &db.P13UnionFightPlayerBattleOffer{MatchID: scope, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: 1}
	if err := db.GetIns().Create(offer).Error; err != nil {
		t.Fatal(err)
	}
	return f, offer
}

func p13CombatProjectionFrame(id int64, state, deaths int32) *pb.InstUFPlayer {
	hp := float64(10)
	if state == 3 {
		hp = 0
	}
	return &pb.InstUFPlayer{Guid: proto.Int64(id), Camp: proto.Int32(0), State: proto.Int32(state), DeadNum: proto.Int32(deaths), LeftTime: proto.Float32(77), TotalTime: proto.Float32(99), FightInfo: &pb.InstFightable{Guid: proto.Int64(id), Index: []int32{3}, HpPos: []float64{hp}, MaxHpPos: []float64{10}, Anger: proto.Float64(float64(deaths))}}
}

func p13CombatProjectionOrdinary(t *testing.T, offer *db.P13UnionFightPlayerBattleOffer, frame *pb.InstUFPlayer, created int64, pending bool) *db.P13UnionFightPlayerBattleAttempt {
	t.Helper()
	peer := proto.Clone(frame).(*pb.InstUFPlayer)
	peer.Guid = proto.Int64(frame.GetGuid() + 100000)
	peer.Camp = proto.Int32(1)
	peer.FightInfo.Guid = peer.Guid
	peer.State = proto.Int32(0)
	peer.DeadNum = proto.Int32(0)
	peer.FightInfo.HpPos[0] = 10
	peer.FightInfo.Anger = proto.Float64(0)
	row := &db.P13UnionFightPlayerBattleAttempt{ID: p6Digest([]byte(fmt.Sprintf("projection-ordinary/%d/%d", frame.GetGuid(), created))), MatchID: offer.MatchID, PlayerID: frame.GetGuid(), TargetID: peer.GetGuid(), Stamp: created + frame.GetGuid(), P6RecordID: p6Digest([]byte(fmt.Sprintf("projection-only-no-p6/%d/%d", frame.GetGuid(), created))), OfferHash: offer.SourceHash, StartPB: []byte{}, AppliedProperties: []byte{}, Before: []byte{}, Terminal: []byte{}, CreatedAt: created, DueAt: created + 5000, Status: constUtil.P13UFPlayerBattlePending}
	row.FightID = "projection-only/" + row.ID
	if pending {
		left, _ := proto.Marshal(frame)
		right, _ := proto.Marshal(peer)
		row.Before, _ = json.Marshal([]p13UFPlayerBattleSide{{PlayerID: frame.GetGuid(), Camp: 0, Frame: left}, {PlayerID: peer.GetGuid(), Camp: 1, Frame: right}})
	} else {
		row.Status = constUtil.P13UFPlayerBattleSettled
		row.DeadPlayerID = frame.GetGuid()
		row.SettledAt = row.DueAt
		row.ReviveSeconds = int64(frame.GetDeadNum()) * 5
		row.After, _ = json.Marshal([]*pb.InstUFPlayer{frame, peer})
	}
	row.SourceHash = p13UFPlayerBattleHash(row)
	if err := db.GetIns().Create(row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func p13CombatProjectionGuard(t *testing.T, scope string, frame *pb.InstUFPlayer, created int64, pending bool) *db.P13UnionFightGuardBattleAttempt {
	t.Helper()
	rules := p13UFGuardBattleRules{Policy: db.P13UnionFightGuardBattlePolicy{ID: 1, Enabled: true, Version: "projection-only-metadata", Source: "countdown fixture; no combat authority or result", PublishedAt: 1, StartsAt: 1, EndsAt: 1 << 62}, LocalRule: constUtil.P13UFGuardLocalRule, BuildingKind: "tower", DurationMillis: 5000, RecoverySeconds: 120, ReviveSeconds: []int64{5, 10, 15, 20, 25, 30, 35, 40}}
	raw, _ := json.Marshal(rules)
	row := &db.P13UnionFightGuardBattleAttempt{ID: p6Digest([]byte(fmt.Sprintf("projection-guard/%d/%d", frame.GetGuid(), created))), MatchID: scope, PlayerID: frame.GetGuid(), GuardGUID: frame.GetGuid() + 200000, Stamp: created + frame.GetGuid(), P6RecordID: p6Digest([]byte(fmt.Sprintf("guard-projection-only-no-p6/%d/%d", frame.GetGuid(), created))), Rules: raw, PlayerBefore: []byte{}, GuardBefore: []byte{}, CreatedAt: created, DueAt: created + 5000, Status: constUtil.P13UFGuardPending, CalculationStage: constUtil.P13UFGuardCaptured}
	if pending {
		row.PlayerBefore, _ = proto.Marshal(frame)
	} else {
		row.Status = constUtil.P13UFGuardSettled
		row.CalculationStage = constUtil.P13UFGuardComputed
		row.PlayerDied = true
		row.SettledAt = row.DueAt
		row.ReviveSeconds = int64(frame.GetDeadNum()) * 5
		row.PlayerAfter, _ = proto.Marshal(frame)
	}
	row.SourceHash = p13UFGuardBattleHash(row)
	if err := db.GetIns().Create(row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestP13UFCombatProjectBatchFiftyPendingQueriesAndNoRecord(t *testing.T) {
	for _, projection := range []string{"", constUtil.P13UnionFightMapProjectionReFlat} {
		t.Run("projection="+projection, func(t *testing.T) {
			f, offer := p13CombatProjectionSetup(t, "pending-projection-"+projection)
			actors := []db.P13UnionFightFieldPlayer{}
			frames := []*pb.InstUFPlayer{}
			for i := int64(1); i <= 50; i++ {
				frame := p13CombatProjectionFrame(1000+i, 1, 0)
				before := proto.Clone(frame).(*pb.InstUFPlayer)
				before.State = proto.Int32(0)
				if i%2 == 0 {
					p13CombatProjectionOrdinary(t, offer, before, 1000, true)
				} else {
					p13CombatProjectionGuard(t, offer.MatchID, before, 1000, true)
				}
				actors = append(actors, db.P13UnionFightFieldPlayer{MatchID: offer.MatchID, PlayerID: frame.GetGuid(), MapProjection: projection})
				frames = append(frames, frame)
			}
			// Preserve a legacy state1 frame without any journal; do not invent a timer.
			actors = append(actors, db.P13UnionFightFieldPlayer{MatchID: offer.MatchID, PlayerID: 9999, MapProjection: projection})
			frames = append(frames, p13CombatProjectionFrame(9999, 1, 0))
			counter := &p13CombatProjectionTrace{Interface: logger.Default.LogMode(logger.Silent)}
			if err := p13UFFieldCombatProjectBatch(db.GetIns().WithContext(f.ctx).Session(&gorm.Session{Logger: counter}), actors, frames, time.UnixMilli(2000)); err != nil {
				t.Fatal(err)
			}
			if len(counter.SQL) != 3 {
				t.Fatal("50 peers caused per-player pending/offer reads", len(counter.SQL))
			}
			for _, frame := range frames[:50] {
				if frame.GetLeftTime() != 4 || frame.GetTotalTime() != 5 {
					t.Fatal("frozen pending duration differs", frame)
				}
			}
			if frames[50].GetLeftTime() != 77 || frames[50].GetTotalTime() != 99 {
				t.Fatal("missing journal fabricated countdown")
			}
		})
	}
}

func TestP13UFCombatProjectBatchMixedDeathLatestBoundedAndCold(t *testing.T) {
	f, offer := p13CombatProjectionSetup(t, "mixed-death-projection")
	actors := []db.P13UnionFightFieldPlayer{}
	frames := []*pb.InstUFPlayer{}
	for i := int64(1); i <= 50; i++ {
		id := 1000 + i
		old := p13CombatProjectionFrame(id, 3, 1)
		current := p13CombatProjectionFrame(id, 3, 2)
		// Large historical display journals must be visited only by the lightweight
		// window query, never loaded as full blobs into the projection batch.
		for j := int64(0); j < 4; j++ {
			row := p13CombatProjectionOrdinary(t, offer, old, 1000+j, false)
			row.Terminal = []byte(strings.Repeat("history-not-a-combat-result", 80))
			row.SourceHash = p13UFPlayerBattleHash(row)
			if err := db.GetIns().Save(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if i%2 == 0 {
			p13CombatProjectionGuard(t, offer.MatchID, old, 2000, false)
			p13CombatProjectionOrdinary(t, offer, current, 4000, false)
		} else {
			p13CombatProjectionOrdinary(t, offer, old, 2000, false)
			p13CombatProjectionGuard(t, offer.MatchID, current, 4000, false)
		}
		projection := ""
		if i%2 == 0 {
			projection = constUtil.P13UnionFightMapProjectionReFlat
		}
		actors = append(actors, db.P13UnionFightFieldPlayer{MatchID: offer.MatchID, PlayerID: id, MapProjection: projection})
		frames = append(frames, current)
	}
	counter := &p13CombatProjectionTrace{Interface: logger.Default.LogMode(logger.Silent)}
	if err := p13UFFieldCombatProjectBatch(db.GetIns().WithContext(f.ctx).Session(&gorm.Session{Logger: counter}), actors, frames, time.UnixMilli(10000)); err != nil {
		t.Fatal(err)
	}
	if len(counter.SQL) > 7 {
		t.Fatal("mixed deaths caused N+1 queries", len(counter.SQL))
	}
	window := 0
	for i, sql := range counter.SQL {
		if strings.Contains(sql, "ROW_NUMBER") {
			window++
			if strings.Contains(sql, "SELECT *") {
				t.Fatal("window query selected historical blobs", sql)
			}
		}
		if counter.Rows[i] > 50 {
			t.Fatal("query loaded unbounded full death history", counter.Rows[i], sql)
		}
	}
	if window != 2 {
		t.Fatal("latest death selection was not windowed", window)
	}
	for _, frame := range frames {
		if frame.GetLeftTime() != 9 || frame.GetTotalTime() != 10 {
			t.Fatal("older other-kind death overwrote current ordinal", frame)
		}
	}
	sqlDB, _ := db.GetIns().DB()
	sqlDB.Close()
	f.open(t)
	for _, frame := range frames {
		frame.LeftTime = proto.Float32(77)
	}
	if err := p13UFFieldCombatProjectBatch(db.GetIns().WithContext(f.ctx), actors, frames, time.UnixMilli(11000)); err != nil {
		t.Fatal("cold countdown", err)
	}
	for _, frame := range frames {
		if frame.GetLeftTime() != 8 || frame.GetTotalTime() != 10 {
			t.Fatal("cold projection restarted timer", frame)
		}
	}
	// A newer death that does not match the actual current HP/death count must
	// not fall back to an older matching journal and manufacture old time.
	frames[0].DeadNum = proto.Int32(1)
	frames[0].FightInfo = p13CombatProjectionFrame(frames[0].GetGuid(), 3, 1).FightInfo
	frames[0].LeftTime = proto.Float32(77)
	if err := p13UFFieldCombatProjectBatch(db.GetIns(), actors[:1], frames[:1], time.UnixMilli(11000)); err != nil {
		t.Fatal(err)
	}
	if frames[0].GetLeftTime() != 77 {
		t.Fatal("obsolete matching death was used after newer terminal")
	}
}

func TestP13UFCombatProjectBatchCorruptionAndChunkBound(t *testing.T) {
	f, offer := p13CombatProjectionSetup(t, "projection-corruption")
	frame := p13CombatProjectionFrame(42, 1, 0)
	before := proto.Clone(frame).(*pb.InstUFPlayer)
	before.State = proto.Int32(0)
	row := p13CombatProjectionOrdinary(t, offer, before, 1000, true)
	actors := []db.P13UnionFightFieldPlayer{{MatchID: offer.MatchID, PlayerID: 42}}
	validRowHash, validOfferHash := row.SourceHash, offer.SourceHash
	if err := db.GetIns().Model(row).Update("source_hash", "damaged").Error; err != nil {
		t.Fatal(err)
	}
	if err := p13UFFieldCombatProjectBatch(db.GetIns(), actors, []*pb.InstUFPlayer{frame}, time.UnixMilli(2000)); err == nil || frame.GetLeftTime() != 77 {
		t.Fatal("corrupt journal fabricated time", err)
	}
	if err := db.GetIns().Model(row).Update("source_hash", validRowHash).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(offer).Update("source_hash", "damaged-offer").Error; err != nil {
		t.Fatal(err)
	}
	if err := p13UFFieldCombatProjectBatch(db.GetIns(), actors, []*pb.InstUFPlayer{frame}, time.UnixMilli(2000)); err == nil {
		t.Fatal("corrupt frozen rule was accepted")
	}
	if err := db.GetIns().Model(offer).Update("source_hash", validOfferHash).Error; err != nil {
		t.Fatal(err)
	}
	guardFrame := p13CombatProjectionFrame(43, 1, 0)
	guardBefore := proto.Clone(guardFrame).(*pb.InstUFPlayer)
	guardBefore.State = proto.Int32(0)
	guard := p13CombatProjectionGuard(t, offer.MatchID, guardBefore, 1000, true)
	validGuardHash := guard.SourceHash
	if err := db.GetIns().Model(guard).Update("source_hash", "damaged-guard").Error; err != nil {
		t.Fatal(err)
	}
	if err := p13UFFieldCombatProjectBatch(db.GetIns(), []db.P13UnionFightFieldPlayer{{MatchID: offer.MatchID, PlayerID: 43}}, []*pb.InstUFPlayer{guardFrame}, time.UnixMilli(2000)); err == nil || guardFrame.GetLeftTime() != 77 {
		t.Fatal("corrupt guard journal fabricated time", err)
	}
	if err := db.GetIns().Model(guard).Update("source_hash", validGuardHash).Error; err != nil {
		t.Fatal(err)
	}
	// 129 requested peers form two bounded chunks; missing-record views retain
	// their exact old state and require a constant two pending reads per chunk.
	actors = nil
	frames := []*pb.InstUFPlayer{}
	for i := int64(1); i <= 129; i++ {
		actors = append(actors, db.P13UnionFightFieldPlayer{MatchID: offer.MatchID, PlayerID: 10000 + i})
		frames = append(frames, p13CombatProjectionFrame(10000+i, 1, 0))
	}
	counter := &p13CombatProjectionTrace{Interface: logger.Default.LogMode(logger.Silent)}
	if err := p13UFFieldCombatProjectBatch(db.GetIns().WithContext(f.ctx).Session(&gorm.Session{Logger: counter}), actors, frames, time.UnixMilli(2000)); err != nil || len(counter.SQL) != 4 {
		t.Fatal("projection chunk bound", len(counter.SQL), err)
	}
	for _, frame := range frames {
		frame.State = proto.Int32(0)
	}
	counter.SQL = nil
	counter.Rows = nil
	if err := p13UFFieldCombatProjectBatch(db.GetIns().Session(&gorm.Session{Logger: counter}), actors, frames, time.UnixMilli(2000)); err != nil || len(counter.SQL) != 0 {
		t.Fatal("idle peers read combat journals", len(counter.SQL), err)
	}
}
