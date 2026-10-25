package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP13UFGuardBattlePolicyDefaultsOffAndCannotCapture(t *testing.T) {
	f, ids, _, clock, _ := p13UFFieldSetup(t)
	if err := f.s.InitializeP13UnionFightGuardBattle(f.ctx); err != nil {
		t.Fatal(err)
	}
	var policy db.P13UnionFightGuardBattlePolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil || policy.Enabled {
		t.Fatal("new guard source must remain disabled", err)
	}
	if replies, handled, err := f.s.p13UFTryGuardBattle(f.ctx, ids[0], &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(12345)}, 0, time.UnixMilli(clock.Load())); err != nil || handled || len(replies) != 0 {
		t.Fatal("disabled independent guard source changed legacy routing", handled, err)
	}
	for _, table := range []any{&db.P13UnionFightGuardState{}, &db.P13UnionFightGuardBattleAttempt{}} {
		var count int64
		if err := db.GetIns().Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("disabled source created state or reservations", count, err)
		}
	}
}

// Actual exported diagnostics exercise pure mapping without fabricating a
// certified result. Compute/terminal authority remains explicitly unavailable.
func TestP13UFGuardBattleActualDiagnosticMappingAndProofRefusal(t *testing.T) {
	root := "../../analysis/p6/guild-guard-inputs-20261025-raw"
	metadata, err := os.ReadFile(filepath.Join(root, "source-export.json"))
	if os.IsNotExist(err) {
		t.Skip("explicit retained real producer inputs required")
	}
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []struct {
			GuardID string            `json:"guardID"`
			Prefix  string            `json:"prefix"`
			Status  string            `json:"status"`
			Files   map[string]string `json:"files"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(metadata, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Cases) != 10 {
		t.Fatal("the real producer ledger must retain all ten original guard IDs")
	}
	for _, row := range manifest.Cases {
		t.Run(row.GuardID, func(t *testing.T) {
			read := func(suffix string) []byte {
				t.Helper()
				name := row.Prefix + "." + suffix
				raw, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || p6Digest(raw) != row.Files[name] {
					t.Fatal("preserved producer file changed", name, err)
				}
				return raw
			}
			if row.Status != "awaiting-original-automatic-ai" {
				t.Fatal("producer source was blocked")
			}
			var input model.FrozenBattleInput
			var terminal model.CombatFinalHPOutcome
			var before pb.InstUFPlayer
			var guard pb.InstUFGuard
			if json.Unmarshal(read("frozen-input.json"), &input) != nil || json.Unmarshal(read("go-prospective-terminal.json"), &terminal) != nil || proto.Unmarshal(read("player-frame.pb"), &before) != nil || proto.Unmarshal(read("guard-frame.pb"), &guard) != nil {
				t.Fatal("actual diagnostic decode")
			}
			if terminal.Outcome == nil || terminal.Outcome.Authoritative || constUtil.P6UFGuardNativeProof != "" {
				t.Fatal("diagnostic cannot be treated as approved native proof")
			}
			if _, err := (&GameService{}).ComputeP6UFGuardSettlement(context.Background(), &input); err == nil {
				t.Fatal("guard calculation acquired authority without native approval")
			}
			if err := validateP6UFGuardTerminal(&input, &terminal); err == nil {
				t.Fatal("Go diagnostic terminal was accepted as certified")
			}
			kind := "tower"
			recovery := int64(120)
			if strings.HasPrefix(row.GuardID, "factory_") {
				kind = "factory"
				recovery = 300
			}
			if strings.HasPrefix(row.GuardID, "baseCamp_") {
				kind = "base"
				recovery = 0
			}
			rules := &p13UFGuardBattleRules{Policy: db.P13UnionFightGuardBattlePolicy{ID: 1, Enabled: true, Version: "diagnostic-map-only", Source: "existing explicitly approved zero guard rewards/ordinal revive contract", PublishedAt: 1, StartsAt: 1, EndsAt: 1 << 62}, LocalRule: constUtil.P13UFGuardLocalRule, BuildingKind: kind, GateProof: p6Digest([]byte("pure mapping fixture; not an accepted base gate certificate")), DurationMillis: 5000, RecoverySeconds: recovery, ReviveSeconds: []int64{5, 10, 15, 20, 25, 30, 35, 40}}
			if _, _, _, _, _, err := p13UFGuardBattleMap(&input, &terminal, &before, &guard, rules, 5000, 4999); err == nil {
				t.Fatal("mapped an early five-second effect")
			}
			after, guardAfter, dead, defeated, revive, err := p13UFGuardBattleMap(&input, &terminal, &before, &guard, rules, 5000, 6000)
			if err != nil {
				t.Fatal("actual terminal mapping", err)
			}
			if after.GetKillNum() != before.GetKillNum() || after.GetOwnScore() != before.GetOwnScore() || after.GetOwnToken() != before.GetOwnToken() {
				t.Fatal("guard defeat borrowed player kill rewards")
			}
			if after.FightInfo.GetAnger() != terminal.Outcome.Attacker.GetInst().GetAnger() || guardAfter.FightInfo.GetAnger() != terminal.Outcome.Monster.GetInst().GetAnger() || guardAfter.FightInfo.Index[0] != 0 {
				t.Fatal("Monster AP or field-index0 mapping changed")
			}
			for _, actor := range terminal.FinalActors {
				if actor.Camp == 0 && after.FightInfo.HpPos[0] != actor.CurrentHP || actor.Camp == 1 && guardAfter.FightInfo.HpPos[0] != actor.CurrentHP {
					t.Fatal("actual HP terminal changed")
				}
			}
			if dead != (after.FightInfo.HpPos[0] == 0) || defeated != (guardAfter.FightInfo.HpPos[0] == 0) || dead && defeated {
				t.Fatal("winner inferred an artificial death")
			}
			if dead && (after.GetDeadNum() != before.GetDeadNum()+1 || revive != 5 || after.GetState() != 3 || after.GetLeftTime() != 4) {
				t.Fatal("guard death did not retain original ordinal clock")
			}
			bad := terminal
			bad.FinalActors = append([]model.CombatFinalActorHP(nil), terminal.FinalActors...)
			bad.FinalActors[0].GUID = "different-identity"
			if _, _, _, _, _, err := p13UFGuardBattleMap(&input, &bad, &before, &guard, rules, 5000, 6000); err == nil {
				t.Fatal("mismatched terminal actor accepted")
			}
			if terminal.Outcome.Authoritative {
				t.Fatal("pure map promoted diagnostic result")
			}
		})
	}
}
