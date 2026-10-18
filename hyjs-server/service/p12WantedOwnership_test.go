package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This supplies one original instance as an isolated ownership prerequisite.
// It does not seed a winner or claim natural discovery or a real PK result.
func p12WantedOwnershipInstance(t *testing.T, f *p1Fixture, id string, now time.Time) *db.P12WantedInstance {
	t.Helper()
	var policy db.P12WantedPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	var def p12WantedDefinition
	if json.Unmarshal([]byte(policy.DefinitionJSON), &def) != nil {
		t.Fatal("original definition")
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
	row := &db.P12WantedInstance{ID: id, ServerID: f.s.OriginalServerID(), FinderID: f.id, FinderName: "ownership prerequisite", WantedID: "1", Level: 1, Difficulty: 1, State: constUtil.P12WantedActive, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(30 * time.Minute).UnixMilli(), CurrentHP: maximum, MaxHP: maximum, MonsterHPJSON: string(rawHP), DefinitionJSON: policy.DefinitionJSON, DefinitionHash: policy.DefinitionHash}
	if err = db.GetIns().Create(row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func TestP12WantedOwnershipAwakePKFieldBusyRejectWithoutCost(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	for _, load := range []func(string) error{f.s.LoadP7AwakeTables, f.s.LoadP12FieldPVPTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP7Awake, f.s.InitializeP12FieldPVP} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P12FieldPVPReady = true
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, "ownership-wanted-original", now)
	request := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	reject := func(part string) {
		t.Helper()
		before, game := f.profile(t)
		_, err := f.s.p12WantedStart(ctx, f.id, request, 913000, now)
		if err == nil || !strings.Contains(err.Error(), part) {
			t.Fatal("wanted new start bypassed owner", part, err)
		}
		after, next := f.profile(t)
		if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, game, next) || p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != 0 {
			t.Fatal("rejected start changed attempt/wallet/game/receipt", part)
		}
	}
	// Actual original Awake Enter owns a live exploration before Wanted is asked.
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeExplorePolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", constUtil.P7AwakeExploreCopy).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now)
	reject("觉醒")
	p7AwakeExploreCall(t, f, &pb.CSAwakeCopySettle{}, now)
	// Persisted PK presentation lease is a declared negative precondition only.
	busy := &db.P12FieldPVPState{PlayerID: f.id, BusyUntil: now.Add(time.Minute).UnixMilli(), Source: constUtil.P12FieldPVPSource}
	if err := db.GetIns().Create(busy).Error; err != nil {
		t.Fatal(err)
	}
	reject(constUtil.P12FieldPVPBusy)
	if err := db.GetIns().Delete(busy).Error; err != nil {
		t.Fatal(err)
	}
	// An existing original field encounter also owns its player without a PK row.
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.Field = &model.FieldState{Fight: &model.FieldFight{StartedAt: now.UnixMilli(), Settled: false}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reject(constUtil.P12WantedUnavailable)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error { game.Field = nil; return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestP12WantedOwnershipBlocksNewModesKeepsPaidRecoveryAndReleasesExpired(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, "ownership-paid-original", now)
	request := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	replies, err := f.s.p12WantedStart(ctx, f.id, request, 914000, now)
	if err != nil {
		t.Fatal(err)
	}
	start := replies[0].(*pb.SCRebelFightStart)
	spent, _ := f.profile(t)
	if err = db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	// An operator disabling new admissions cannot erase the already paid frame.
	retry, err := f.s.p12WantedStart(ctx, f.id, request, 914000, now)
	if err != nil || !proto.Equal(start, retry[0]) {
		t.Fatal("paid same-stamp recovery blocked by current policy", err)
	}
	if _, err = f.s.copyEnter(f.ctx, f.id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err == nil || !strings.Contains(err.Error(), constUtil.P12WantedBusy) {
		t.Fatal("new original copy bypassed pending Wanted", err)
	}
	if err = f.s.p12WantedProfile(f.ctx, f.id, func(tx *gorm.DB, _ *db.Profile, player *pb.Player, game *model.GameState) error {
		chapters := f.s.Tables["DailyCopyChapter"]
		if len(chapters) == 0 {
			t.Fatal("original resource-copy catalogue missing")
		}
		_, _, err := f.s.dailyCopyStart(tx, player, game, &pb.CSDailyCopyFightStart{ChapterId: proto.String(chapters[0]["ChapterId"])}, 914001, now)
		return err
	}); err == nil || !strings.Contains(err.Error(), constUtil.P12WantedBusy) {
		t.Fatal("new original resource copy bypassed pending Wanted", err)
	}
	afterReject, _ := f.profile(t)
	if !proto.Equal(spent, afterReject) {
		t.Fatal("recovery or rejected other-mode start consumed profile resources")
	}
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("certified terminal")
	}
	actual := int32(0)
	if terminal.Outcome.Winner == 0 {
		actual = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(actual), OptRecord: terminal.OriginalOperates}
	if _, err = f.s.p12WantedEnd(context.WithValue(f.ctx, constUtil.GameRequestContextKey, end), f.id, end, now); err != nil {
		t.Fatal("operator policy closure blocked accepted actual End", err)
	}
	if err = db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(instance, "id=?", instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.State != constUtil.P12WantedActive || instance.CurrentHP <= 0 {
		instance = p12WantedOwnershipInstance(t, f, "ownership-next-original", now)
		request.RebelId = proto.String(instance.ID)
	}
	if _, err = f.s.p12WantedStart(ctx, f.id, request, 914002, now); err != nil {
		t.Fatal("next actual encounter", err)
	}
	charged, _ := f.profile(t)
	// Only this helper's explicit domain clock crosses expiry; no system/calendar
	// or original operating window is edited, and no damage/result is supplied.
	expiredAt := time.UnixMilli(instance.ExpiresAt + 1)
	if err = f.s.p12WantedProfile(f.ctx, f.id, func(tx *gorm.DB, _ *db.Profile, _ *pb.Player, _ *model.GameState) error {
		return P12WantedRequireIdle(tx, f.id, expiredAt)
	}); err != nil {
		t.Fatal("expired presentation lease remained held", err)
	}
	last, _ := f.profile(t)
	if last.GetWantedAttTimes() != charged.GetWantedAttTimes() || last.GetScore() != charged.GetScore() {
		t.Fatal("expiry refunded attempts or minted reward")
	}
	if p1Count(t, &db.P12WantedFight{}, "player_id=? AND committed=? AND abandoned_at=0", f.id, false) != 0 {
		t.Fatal("expired pending lease was not persisted as abandoned")
	}
}
