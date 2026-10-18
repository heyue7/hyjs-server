package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// One original first-floor transaction only; no multi-floor/actor matrix. The
// test client independently executes the original driver from the actual Start.
func TestP7GenericMechanismMySQLFirstFloorEndRewardCold(t *testing.T) {
	p7GenericMechanismFirstFloorTransaction(t, false)
}

func TestP7RoundOneAutoFormationMySQLFirstFloorEndRewardCold(t *testing.T) {
	p7GenericMechanismFirstFloorTransaction(t, true)
}

func p7GenericMechanismFirstFloorTransaction(t *testing.T, formationAuto bool) {
	t.Helper()
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("owned isolated MySQL P7 database required")
	}
	f := p7PartyBusinessFixture(t)
	query, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	if err != nil {
		t.Fatal("actual first-floor query", err)
	}
	var task *pb.RushTaskInfo
	for _, candidate := range query[0].(*pb.SCRushCurrentInfo).TaskInfoList {
		if candidate.GetStar() == 1 {
			task = candidate
		}
	}
	if task == nil || task.GetPoint() != 10 {
		t.Fatal("original first-floor reward preview unavailable")
	}
	request := &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: task.TaskId, Star: proto.Int32(1)}
	start, err := f.s.p7Start(f.ctx, f.id, request, 710001, false)
	if err != nil {
		t.Fatal("actual public first-floor Start", err)
	}
	var pending db.P7RushManualCalculation
	if err = db.GetIns().First(&pending, "fight_id=?", start.GetFightId()).Error; err != nil || pending.DriverPolicy != constUtil.P7GenericMechanismDriverSource || pending.ScopeProof != constUtil.P7GenericMechanismCompatibilityProof || pending.Status != constUtil.P7RushManualPending {
		t.Fatal("new named Generic pending transaction", err)
	}
	series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	clientInput, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		t.Fatal(err)
	}
	var client *model.CombatFinalHPOutcome
	if formationAuto {
		clientInput, err = cloneP6CalculationInput(series.Template)
		if err != nil {
			t.Fatal(err)
		}
		control := &pb.BattleTransOperate{RoundNum: proto.Int32(1)}
		if len(clientInput.Attacker.Formation) != 2 {
			t.Fatal("finite original two-actor source changed")
		}
		for i, actor := range clientInput.Attacker.Formation {
			control.ActorUids = append(control.ActorUids, actor.GetGuid())
			control.Fpids = append(control.Fpids, []int32{4, 8}[i])
		}
		// Finite client controls generate a fresh complete record; the actual
		// server End must independently choose and close the production driver.
		client, err = f.s.computeFrozenBattleFinalHPControlled(f.ctx, clientInput, nil, true, control)
	} else {
		client, err = f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, clientInput, nil)
	}
	if err != nil || client.Outcome.Authoritative {
		t.Fatal("independent original test-client End", err)
	}
	result := int32(0)
	if client.Outcome.Winner == 0 {
		result = 1
	}
	if result != 1 {
		t.Fatal("actual original first-floor source lost; preserve it", client.Outcome.Winner)
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: client.OriginalOperates}
	before, _ := f.profile(t)
	bad := proto.Clone(end).(*pb.CSRushFightEnd)
	bad.OptRecord.AttackerTotalLeftHp = proto.Float64(bad.OptRecord.GetAttackerTotalLeftHp() + 1)
	if _, err = f.s.p7End(f.ctx, f.id, bad); err == nil {
		t.Fatal("forged actual public End minted rewards")
	}
	unchanged, _ := f.profile(t)
	if !proto.Equal(before, unchanged) {
		t.Fatal("rejected End changed wallet")
	}
	accepted, err := f.s.p7End(f.ctx, f.id, end)
	if err != nil {
		t.Fatal("actual public original End/reward", err)
	}
	if formationAuto {
		if err = db.GetIns().First(&pending, "fight_id=?", start.GetFightId()).Error; err != nil {
			t.Fatal(err)
		}
		var terminal model.CombatFinalHPOutcome
		if json.Unmarshal(pending.Terminal, &terminal) != nil || terminal.Outcome == nil || terminal.Outcome.DiagnosticAIDriverPolicy != constUtil.P6RoundOneLiveAutoFormationDriver {
			t.Fatal("actual End did not freeze the independent formation/Auto driver")
		}
	}
	after, game := f.profile(t)
	if after.GetPoint() != before.GetPoint()+task.GetPoint() || after.GetCoin() != before.GetCoin() || after.GetDiamond() != before.GetDiamond() {
		t.Fatal("actual first-floor settlement did not grant exact displayed reward")
	}
	if p1Count(t, &db.P7RushManualCalculation{}, "player_id=? AND status=?", f.id, constUtil.P7RushManualAccepted) != 1 || p1Count(t, &db.P6BattleRecord{}, "player_id=? AND authoritative=?", f.id, true) != 1 {
		t.Fatal("actual transaction accepted more than one fight")
	}
	f.restart(t)
	repeated, err := f.s.p7End(f.ctx, f.id, end)
	if err != nil || len(repeated) != len(accepted) || !proto.Equal(repeated[0], accepted[0]) {
		t.Fatal("cold actual End receipt changed", err)
	}
	current, currentGame := f.profile(t)
	if !proto.Equal(after, current) || !p6CopySeriesJSONEqual(t, game, currentGame) {
		t.Fatal("cold receipt minted or changed profile")
	}
	t.Logf("MySQL first original floor actual Start→End→reward point+%d; Generic proof=%s; cold receipt exact, one fight only", task.GetPoint(), pending.ScopeProof)
}
