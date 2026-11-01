package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The fractional HP is an isolated four-body fixture; no production HP or
// player data is copied. Both totals use the same validated body values.
func p12WantedLegacyOrderFixture(t *testing.T, s *GameService, instance *db.P12WantedInstance) (map[string]float64, float64, float64) {
	t.Helper()
	var def p12WantedDefinition
	if err := json.Unmarshal([]byte(instance.DefinitionJSON), &def); err != nil {
		t.Fatal(err)
	}
	_, bodies, _, err := s.p12WantedMonster(def, instance.WantedID, nil)
	if err != nil || len(bodies) != 4 {
		t.Fatal("original four-body definition", err)
	}
	values := []float64{618.79, 795.08, 270.45, 1890.63}
	hp := make(map[string]float64, len(values))
	for i, body := range bodies {
		if values[i] > body.GetHp() {
			t.Fatal("fixture exceeds original maximum", i)
		}
		hp[body.GetWantedEnemyId()] = values[i]
	}
	ordered := float64(0)
	for _, i := range []int{0, 1, 2, 3} {
		ordered += values[i]
	}
	legacy := float64(0)
	for _, i := range []int{0, 2, 1, 3} {
		legacy += values[i]
	}
	if ordered == legacy {
		t.Fatal("fixture must demonstrate distinct binary floating-point sums")
	}
	raw, err := json.Marshal(hp)
	if err != nil {
		t.Fatal(err)
	}
	instance.MonsterHPJSON, instance.CurrentHP = string(raw), legacy
	return hp, ordered, legacy
}

func TestP12WantedSharedLegacyOrderRequiresExactValidSnapshot(t *testing.T) {
	s, def := p12WantedOriginalDefinition(t)
	_, _, maximum, err := s.p12WantedMonster(def, "1", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	instance := &db.P12WantedInstance{ID: "legacy-order", ServerID: 1, FinderID: 2, WantedID: "1", Level: 1, Difficulty: 1, MaxHP: maximum, DefinitionJSON: string(raw), DefinitionHash: p6Digest(raw)}
	hp, ordered, legacy := p12WantedLegacyOrderFixture(t, s, instance)
	view, err := s.p12WantedView(instance)
	if err != nil || view.GetHp() != ordered || instance.CurrentHP != legacy || len(view.Monsters) != 4 {
		t.Fatal("legacy map sum was not projected from ordered body HP", err)
	}

	// The next representable value above both actual sums must remain invalid,
	// even when it is only one ULP away from a reachable sum.
	bad := *instance
	bad.CurrentHP = math.Nextafter(math.Max(ordered, legacy), math.Inf(1))
	if _, err = s.p12WantedView(&bad); err == nil {
		t.Fatal("unreachable floating-point total was accepted")
	}
	for _, change := range []func(*db.P12WantedInstance){
		func(v *db.P12WantedInstance) { v.DefinitionHash = "damaged" },
		func(v *db.P12WantedInstance) { v.MaxHP++ },
		func(v *db.P12WantedInstance) { v.FinderID = 0 },
		func(v *db.P12WantedInstance) { v.Level++ },
		func(v *db.P12WantedInstance) { v.MonsterHPJSON = "null" },
		func(v *db.P12WantedInstance) {
			copied := make(map[string]float64, len(hp))
			for key, value := range hp {
				copied[key] = value
			}
			copied["wanted_1"] = maximum + 1
			encoded, _ := json.Marshal(copied)
			v.MonsterHPJSON = string(encoded)
		},
	} {
		bad = *instance
		change(&bad)
		if _, err = s.p12WantedView(&bad); err == nil {
			t.Fatal("damaged identity, definition or body HP was accepted")
		}
	}
}

func TestP12WantedSharedLegacyListAndFallbackSettlement(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().Select("id").Where("id<>?", f.id).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, owner.Id, func(player *pb.Player, _ *model.GameState) error {
		player.Level = proto.Int32(35)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	shared := p12WantedOwnershipInstance(t, f, "shared-legacy-order", now)
	hpBefore, ordered, legacy := p12WantedLegacyOrderFixture(t, f.s, shared)
	shared.FinderID = owner.Id
	if err := db.GetIns().Save(shared).Error; err != nil {
		t.Fatal(err)
	}
	// A minimal committed damage row is a controlled prerequisite for the real
	// Share operation; it carries no player reward or fabricated settlement.
	prior := db.P12WantedFight{ID: "fixture-owner-prior-damage", PlayerID: owner.Id, InstanceID: shared.ID, CreatedAt: now.UnixMilli(), Committed: true, CommittedAt: now.UnixMilli(), AppliedDamage: 1, StartProto: []byte{}, EndProto: []byte{}}
	if err := db.GetIns().Create(&prior).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p12WantedShare(f.ctx, owner.Id, &pb.CSShareRebel{RebelId: proto.String(shared.ID)}, now); err != nil {
		t.Fatal("owner could not share the historical instance", err)
	}
	own := p12WantedOwnershipInstance(t, f, "own-valid", now)
	private := p12WantedOwnershipInstance(t, f, "other-private", now)
	private.FinderID = owner.Id
	if err := db.GetIns().Save(private).Error; err != nil {
		t.Fatal(err)
	}
	foreign := p12WantedOwnershipInstance(t, f, "foreign-server", now)
	foreign.FinderID, foreign.ServerID, foreign.SharedAt = owner.Id, f.s.OriginalServerID()+1, now.UnixMilli()
	if err := db.GetIns().Save(foreign).Error; err != nil {
		t.Fatal(err)
	}
	bad := p12WantedOwnershipInstance(t, f, "corrupt-shared", now)
	bad.FinderID, bad.SharedAt, bad.DefinitionHash = owner.Id, now.UnixMilli(), "damaged"
	bad.CurrentHP--
	if err := db.GetIns().Save(bad).Error; err != nil {
		t.Fatal(err)
	}
	list, err := f.s.p12WantedList(f.ctx, f.id, now)
	if err != nil {
		t.Fatal("one corrupt instance blocked the list", err)
	}
	visible := map[string]*pb.RebelInfo{}
	for _, item := range list[0].(*pb.SCGetRebels).Rebels {
		visible[item.GetId()] = item
	}
	if len(visible) != 2 || visible[own.ID] == nil || visible[shared.ID] == nil || visible[shared.ID].GetHp() != ordered || visible[shared.ID].GetState() != 1 || ordered == legacy {
		t.Fatal("own/shared visibility or legacy projection differs", visible)
	}
	before, _ := f.profile(t)
	for _, row := range []*db.P12WantedInstance{private, foreign, bad} {
		request := &pb.CSRebelFightStart{RebelId: proto.String(row.ID), Type: proto.Int32(1)}
		if _, err = f.s.p12WantedStart(context.WithValue(f.ctx, constUtil.GameRequestContextKey, request), f.id, request, 120100, now); err == nil {
			t.Fatal("private, cross-server or corrupt instance admitted", row.ID)
		}
	}
	if _, err = f.s.p12WantedShare(f.ctx, owner.Id, &pb.CSShareRebel{RebelId: proto.String(bad.ID)}, now); err == nil {
		t.Fatal("corrupt shared instance accepted by owner share retry")
	}
	unchanged, _ := f.profile(t)
	if !proto.Equal(before, unchanged) || p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != 0 {
		t.Fatal("rejected entries changed attempts, wallet or fight records")
	}
	var savedPolicy db.P12WantedPolicy
	if err = db.GetIns().First(&savedPolicy, 1).Error; err != nil {
		t.Fatal(err)
	}
	originalHash := savedPolicy.DefinitionHash
	if err = db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=?", savedPolicy.ID).Update("definition_hash", "damaged").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p12WantedList(f.ctx, f.id, now); err == nil {
		t.Fatal("public configuration failure was hidden as an invalid instance")
	}
	if err = db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=?", savedPolicy.ID).Update("definition_hash", originalHash).Error; err != nil {
		t.Fatal(err)
	}

	request := &pb.CSRebelFightStart{RebelId: proto.String(shared.ID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	starts, err := f.s.p12WantedStart(startCtx, f.id, request, 120101, now)
	if err != nil {
		t.Fatal("participant could not start shared historical instance", err)
	}
	start := starts[0].(*pb.SCRebelFightStart)
	retry, err := f.s.p12WantedStart(startCtx, f.id, request, 120101, now)
	if err != nil || !proto.Equal(start, retry[0]) {
		t.Fatal("start retry changed the paid fight", err)
	}
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var input model.FrozenBattleInput
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("missing certified frozen fight")
	}
	expectedHP, applied, err := p12WantedApplyTerminalHP(&input, &terminal, hpBefore)
	if err != nil {
		t.Fatal(err)
	}
	var def p12WantedDefinition
	if err = json.Unmarshal([]byte(shared.DefinitionJSON), &def); err != nil {
		t.Fatal(err)
	}
	_, bodies, _, err := f.s.p12WantedMonster(def, shared.WantedID, expectedHP)
	if err != nil {
		t.Fatal(err)
	}
	expectedTotal := float64(0)
	for _, body := range bodies {
		expectedTotal += body.GetHp()
	}
	winner := int32(0)
	if terminal.Outcome.Winner == 0 {
		winner = 1
	}
	record := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	record.DefenderTotalLeftHp = proto.Float64(record.GetDefenderTotalLeftHp() + 1)
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(1 - winner), OptRecord: record}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	if _, err = f.s.p12WantedEnd(endCtx, owner.Id, end, now); err == nil {
		t.Fatal("another player submitted the participant's fight")
	}
	result, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil {
		t.Fatal("shared fight server-terminal fallback", err)
	}
	receipt := result[0].(*pb.SCRebelFightEnd)
	var after db.P12WantedInstance
	if err = db.GetIns().First(&after, "id=?", shared.ID).Error; err != nil {
		t.Fatal(err)
	}
	var gotHP map[string]float64
	if json.Unmarshal([]byte(after.MonsterHPJSON), &gotHP) != nil || len(gotHP) != len(expectedHP) {
		t.Fatal("shared HP was not persisted")
	}
	for key, value := range expectedHP {
		if gotHP[key] != value {
			t.Fatal("shared body HP diverged", key)
		}
	}
	if after.CurrentHP != expectedTotal || receipt.GetDamage() != applied || receipt.GetResult() != winner {
		t.Fatal("new total, damage or server winner diverged")
	}
	if err = db.GetIns().First(&fight, "id=?", fight.ID).Error; err != nil || !fight.Committed || fight.RecordedProof != constUtil.P12WantedAutomaticFallback {
		t.Fatal("server fallback receipt not committed", err)
	}
	paid, _ := f.profile(t)
	if paid.GetWantedAttTimes() != before.GetWantedAttTimes()-1 || paid.GetScore()-before.GetScore() != receipt.GetScoreAdd() {
		t.Fatal("cost or score did not match the single receipt")
	}
	f.restart(t)
	replayed, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil || !proto.Equal(replayed[0], receipt) {
		t.Fatal("cold retry changed the committed receipt", err)
	}
	last, _ := f.profile(t)
	var again db.P12WantedInstance
	if err = db.GetIns().First(&again, "id=?", shared.ID).Error; err != nil || !proto.Equal(last, paid) || again.CurrentHP != after.CurrentHP || again.MonsterHPJSON != after.MonsterHPJSON {
		t.Fatal("cold retry changed HP or the participant balance", err)
	}
	conflict := proto.Clone(end).(*pb.CSRebelFightEnd)
	conflict.OptRecord.DefenderTotalLeftHp = proto.Float64(record.GetDefenderTotalLeftHp() + 1)
	if _, err = f.s.p12WantedEnd(endCtx, f.id, conflict, now); err == nil {
		t.Fatal("different recording replaced a committed receipt")
	}
}
