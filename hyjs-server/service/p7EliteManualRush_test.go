package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

// SQLite tests exercise the real paid source/transaction; the client record is
// a separately computed finite control fixture, not a claimed physical phone run.
func TestP7EliteManualNewPermissionWholeEndColdOnce(t *testing.T) {
	f, _, request := p7ScopedFixture(t, constUtil.P7RushElite)
	t.Cleanup(func() { db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 2).Update("enabled", false) })
	if err := db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 2).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	start, err := f.s.p7Start(f.ctx, f.id, request, 17001, false)
	if err != nil {
		t.Fatal(err)
	}
	var row db.P7RushManualCalculation
	if err = db.GetIns().First(&row, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var original db.P6BattleRecord
	if err = db.GetIns().First(&original, "id=?", row.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	family, err := p7ManualRushFamilyForReceipt(constUtil.P7RushElite, row.DriverPolicy, row.ScopeProof)
	if err != nil {
		t.Fatal("pending elite has no recognized frozen family", err)
	}
	if row.Status != constUtil.P7RushManualPending || len(row.Terminal) != 0 || original.Authoritative || original.Status != constUtil.P6BattleCaptured {
		t.Fatal("preview became a real result")
	}
	raw, err := decodeP6StoredInput(row.Series)
	if err != nil {
		t.Fatal(err)
	}
	var series model.FrozenP6RushSeries
	if err = json.Unmarshal(raw, &series); err != nil {
		t.Fatal(err)
	}
	if err = family.ValidateScope(&series); err != nil {
		t.Fatal("pending elite family does not admit its frozen source", err)
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		t.Fatal(err)
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(30.100000000000158)
	calculated, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if calculated.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: calculated.OriginalOperates}
	for _, variant := range []string{"clock", "hp", "winner", "missing-record"} {
		altered := proto.Clone(end).(*pb.CSRushFightEnd)
		switch variant {
		case "clock":
			altered.OptRecord.SkillOpts[0].RoundTime = proto.Float64(altered.OptRecord.SkillOpts[0].GetRoundTime() + 0.1)
		case "hp":
			altered.OptRecord.AttackerTotalLeftHp = proto.Float64(altered.OptRecord.GetAttackerTotalLeftHp() + 1)
		case "winner":
			altered.Result = proto.Int32(1 - result)
		case "missing-record":
			altered.OptRecord = nil
		}
		if _, err = f.s.p7End(f.ctx, f.id, altered); err == nil {
			t.Fatal("incomplete End accepted", variant)
		}
		var pending db.P7RushManualCalculation
		if err = db.GetIns().First(&pending, "fight_id=?", row.FightID).Error; err != nil || pending.Status != constUtil.P7RushManualPending || len(pending.Terminal) != 0 {
			t.Fatal("failed comparison persisted terminal", variant, err)
		}
		var still db.P6BattleRecord
		if err = db.GetIns().First(&still, "id=?", original.ID).Error; err != nil || still.Status != constUtil.P6BattleCaptured || still.Authoritative || string(still.Input) != string(original.Input) {
			t.Fatal("failed comparison changed frozen record", variant, err)
		}
	}
	retry, err := f.s.p7Start(f.ctx, f.id, request, 17001, false)
	if err != nil || !proto.Equal(retry, start) {
		t.Fatal("pending retry drift", err)
	}
	// Policy closure blocks new admission but honors this earlier explicit permission.
	if err = db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 2).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	response, err := f.s.p7End(f.ctx, f.id, end)
	if err != nil {
		t.Fatal(err)
	}
	accepted := response[0].(*pb.SCRushFightEnd)
	var final db.P7RushManualCalculation
	if err = db.GetIns().First(&final, "fight_id=?", row.FightID).Error; err != nil || final.Status != constUtil.P7RushManualAccepted || final.DerivedCalculationSourceSHA256 == "" || final.EndRequestHash == "" {
		t.Fatal("accepted evidence missing", err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", original.ID).Error; err != nil || !record.Authoritative || record.Status != constUtil.P6BattleComputed || string(record.Input) != string(original.Input) {
		t.Fatal("real result or original source incorrect", err)
	}
	after, game := f.profile(t)
	expected := before.GetPoint()
	if result == 1 {
		expected += 10
	}
	if after.GetPoint() != expected {
		t.Fatal("frozen award mismatch")
	}
	f.restart(t)
	repeated, err := f.s.p7End(f.ctx, f.id, end)
	if err != nil || !proto.Equal(repeated[0], accepted) {
		t.Fatal("cold accepted receipt drift", err)
	}
	again, gameAgain := f.profile(t)
	if !proto.Equal(after, again) || !p6CopySeriesJSONEqual(t, game, gameAgain) {
		t.Fatal("cold duplicated reward")
	}
	if p1Count(t, &db.P7RushScopedCalculation{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P7RushManualCalculation{}, "player_id=?", f.id) != 1 {
		t.Fatal("manual run mixed old auto rows")
	}
}

// Each new policy grants only its named driver; existing Trial rows and default
// disabled seeds keep their original identity and state across initialization.
func TestP7EliteManualPolicyIsolationAndInitialization(t *testing.T) {
	f, _, _ := p7ScopedFixture(t, constUtil.P7RushTrial)
	for _, id := range []int64{1, 2} {
		if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", id).Update("enabled", false).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id IN ?", []int64{1, 2}).Update("enabled", false)
	})
	if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 2).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		kind    int32
		allowed bool
	}{{constUtil.P7RushElite, true}, {constUtil.P7RushTrial, false}} {
		got, err := p7ManualRushStageAllowed(db.GetIns(), item.kind)
		if err != nil || got != item.allowed {
			t.Fatal("Elite policy leaked", item, got, err)
		}
	}
	if err := initializeP7ManualRush(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	var trial, elite db.P7RushManualPolicy
	if db.GetIns().First(&trial, 1).Error != nil || db.GetIns().First(&elite, 2).Error != nil || trial.Enabled || !elite.Enabled || trial.Source != constUtil.P7RushManualDriverSource || elite.Source != constUtil.P7RushEliteManualDriverSource {
		t.Fatal("initialization overwrote independent policy")
	}
	if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 2).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		kind    int32
		allowed bool
	}{{constUtil.P7RushTrial, true}, {constUtil.P7RushElite, false}} {
		got, err := p7ManualRushStageAllowed(db.GetIns(), item.kind)
		if err != nil || got != item.allowed {
			t.Fatal("Trial policy leaked", item, got, err)
		}
	}
	_ = f
}
