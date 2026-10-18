package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12WantedPersistenceFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7PartyBusinessFixture(t)
	// Reuse the complete actual equipped roster prerequisite, then initialize
	// the real saveProfile consumers before installing the service context.
	for name, rows := range p6OriginalTables(t).Tables {
		f.s.Tables[name] = rows
	}
	if err := f.s.LoadP12WantedTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP4, f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitializeP12Wanted} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P12WantedReady = true
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	return f
}

// This domain fixture supplies an original instance, not a natural-discovery claim.
// The HTTP runner separately obtains instances through real copy/sweep transactions.
func TestP12WantedPersistenceActualCostsRewardsAndColdReceipt(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	before, _ := f.profile(t)
	listCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, &pb.CSGetRebels{})
	if _, err := f.s.p12WantedList(listCtx, f.id, now); err != nil {
		t.Fatal(err)
	}
	ready, _ := f.profile(t)
	if before.GetWantedAttTime() != 0 || ready.GetWantedAttTimes() != 10 || ready.GetWantedAttTime() <= 0 {
		t.Fatal("original list must persist initial natural attempts, not merely return a projection")
	}
	var policy db.P12WantedPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	var def p12WantedDefinition
	if json.Unmarshal([]byte(policy.DefinitionJSON), &def) != nil {
		t.Fatal("frozen original definition")
	}
	_, bodies, maximum, err := f.s.p12WantedMonster(def, "1", nil)
	if err != nil {
		t.Fatal(err)
	}
	hp := map[string]float64{}
	for _, body := range bodies {
		hp[body.GetWantedEnemyId()] = body.GetHp()
	}
	rawHP, _ := json.Marshal(hp)
	instance := db.P12WantedInstance{ID: "domain-original-wanted1-persistence", ServerID: f.s.OriginalServerID(), FinderID: f.id, FinderName: ready.GetName(), WantedID: "1", Level: 1, Difficulty: 1, State: constUtil.P12WantedActive, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(30 * time.Minute).UnixMilli(), CurrentHP: maximum, MaxHP: maximum, MonsterHPJSON: string(rawHP), DefinitionJSON: policy.DefinitionJSON, DefinitionHash: policy.DefinitionHash}
	if err = db.GetIns().Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	replies, err := f.s.p12WantedStart(ctx, f.id, request, 912000, now)
	if err != nil {
		t.Fatal(err)
	}
	start := replies[0].(*pb.SCRebelFightStart)
	spent, _ := f.profile(t)
	if spent.GetWantedAttTimes() != ready.GetWantedAttTimes()-1 || spent.GetScore() != ready.GetScore() {
		t.Fatal("Start did not persist exactly one original normal attempt")
	}
	f.restart(t)
	cold, _ := f.profile(t)
	if !proto.Equal(spent, cold) {
		t.Fatal("cold profile differs from committed Start cost")
	}
	retry, err := f.s.p12WantedStart(ctx, f.id, request, 912000, now)
	if err != nil || !proto.Equal(start, retry[0]) {
		t.Fatal("cold Start retry lost exact receipt", err)
	}
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("actual certified immutable terminal missing")
	}
	actual := int32(0)
	if terminal.Outcome.Winner == 0 {
		actual = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(actual), OptRecord: terminal.OriginalOperates}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	finished, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil {
		t.Fatal(err)
	}
	result := finished[0].(*pb.SCRebelFightEnd)
	after, _ := f.profile(t)
	if result.GetDamage() < 0 || result.GetResult() != actual || after.GetWantedAttTimes() != spent.GetWantedAttTimes() || after.GetScore()-spent.GetScore() != result.GetScoreAdd() {
		t.Fatal("actual End award/cost did not persist exactly; zero actual damage remains legal")
	}
	f.restart(t)
	cold, _ = f.profile(t)
	if !proto.Equal(after, cold) {
		t.Fatal("cold profile lost actual End award")
	}
	replayed, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil || !proto.Equal(replayed[0], result) {
		t.Fatal("cold End exact receipt changed", err)
	}
	last, _ := f.profile(t)
	if !proto.Equal(after, last) {
		t.Fatal("replayed End changed original profile balance")
	}
}
