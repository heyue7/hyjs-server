package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Pending receipt fixtures exercise admission only and never assert a P6 result.
func p13UFBaseBarrierFixture(scope string, ordinal int64, due int64) db.P13UnionFightGuardBattleAttempt {
	rules := p13UFGuardBattleRules{Policy: db.P13UnionFightGuardBattlePolicy{ID: 1, Enabled: true, Version: "barrier-fixture", Source: "pending base metadata fixture only", PublishedAt: 1, StartsAt: 1, EndsAt: 1000000}, LocalRule: constUtil.P13UFGuardLocalRule, BuildingKind: "base", GateProof: p6Digest([]byte("admission fixture")), DurationMillis: 5000, ReviveSeconds: []int64{5}}
	raw, _ := json.Marshal(rules)
	guard, _ := proto.Marshal(&pb.InstUFGuard{Guid: proto.Int64(ordinal), UFGuardId: proto.String("baseCamp_guard_001"), Camp: proto.Int32(0)})
	player, _ := proto.Marshal(&pb.InstUFPlayer{Guid: proto.Int64(100 + ordinal), Camp: proto.Int32(1)})
	id := p6Digest([]byte(fmt.Sprintf("barrier/%s/%d", scope, ordinal)))
	row := db.P13UnionFightGuardBattleAttempt{ID: id, MatchID: scope, BuildingKind: "base", GuardGUID: ordinal, PlayerID: 100 + ordinal, Stamp: ordinal, P6RecordID: id, Rules: raw, GuardBefore: guard, PlayerBefore: player, Status: constUtil.P13UFGuardPending, CalculationStage: constUtil.P13UFGuardCaptured, CreatedAt: due - 5000, DueAt: due}
	row.SourceHash = p13UFGuardBattleHash(&row)
	return row
}

func TestP13UFBaseBarrierBatchPendingScopeAndLegacy(t *testing.T) {
	f, _, _, _, _ := p13UFFieldSetup(t)
	if err := f.s.InitializeP13UnionFightGuardBattle(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(20000)
	check := func(scopes []string, want map[string]int64) {
		t.Helper()
		got, err := p13UFBaseBarrierBatch(db.GetIns(), scopes, now)
		if err != nil || len(got) != len(want) {
			t.Fatal(got, want, err)
		}
		for key, value := range want {
			if got[key] != value {
				t.Fatal(got, want)
			}
		}
	}
	put := func(row db.P13UnionFightGuardBattleAttempt) {
		t.Helper()
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	check([]string{"barrier-a"}, map[string]int64{})
	put(p13UFBaseBarrierFixture("barrier-a", 1, 19000))
	check([]string{"barrier-a", "barrier-a"}, map[string]int64{"barrier-a": 19000})
	put(p13UFBaseBarrierFixture("barrier-a", 2, 18000))
	check([]string{"barrier-a"}, map[string]int64{"barrier-a": 18000})
	put(p13UFBaseBarrierFixture("barrier-other", 3, 17000))
	check([]string{"barrier-a"}, map[string]int64{"barrier-a": 18000})
	future := p13UFBaseBarrierFixture("barrier-future", 4, 20001)
	put(future)
	settled := p13UFBaseBarrierFixture("barrier-settled", 5, 19000)
	settled.Status = constUtil.P13UFGuardSettled
	settled.SettledAt = 20000
	settled.SourceHash = p13UFGuardBattleHash(&settled)
	put(settled)
	legacy := p13UFBaseBarrierFixture("barrier-legacy", 6, 19000)
	legacy.BuildingKind = ""
	legacy.SourceHash = p13UFGuardBattleHash(&legacy)
	put(legacy)
	raw, _ := json.Marshal(legacy)
	if strings.Contains(string(raw), "building_kind") {
		t.Fatal("legacy omitted field changed hash surface")
	}
	check([]string{"barrier-future", "barrier-settled", "barrier-legacy"}, map[string]int64{"barrier-legacy": 19000})
	at, err := p13UFBaseBarrierAt(db.GetIns(), "barrier-legacy", now)
	if err != nil || at != 19000 {
		t.Fatal(at, err)
	}
	put(p13UFBaseBarrierFixture("barrier-a", 7, 19500))
	if _, err = p13UFBaseBarrierBatch(db.GetIns(), []string{"barrier-a"}, now); err == nil {
		t.Fatal("third pending base accepted")
	}
	bad := p13UFBaseBarrierFixture("barrier-bad", 8, 19000)
	bad.SourceHash = "corrupt"
	put(bad)
	if _, err = p13UFBaseBarrierBatch(db.GetIns(), []string{"barrier-bad"}, now); err == nil {
		t.Fatal("corrupt pending source accepted")
	}
	var rules p13UFGuardBattleRules
	mismatch := p13UFBaseBarrierFixture("barrier-mismatch", 9, 19000)
	_ = json.Unmarshal(mismatch.Rules, &rules)
	rules.BuildingKind = "tower"
	rules.RecoverySeconds = 120
	mismatch.Rules, _ = json.Marshal(rules)
	mismatch.SourceHash = p13UFGuardBattleHash(&mismatch)
	put(mismatch)
	if _, err = p13UFBaseBarrierBatch(db.GetIns(), []string{"barrier-mismatch"}, now); err == nil {
		t.Fatal("kind/rules mismatch accepted")
	}
}
