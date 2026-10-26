package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func wantedClock75Setup(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f := p12WantedPersistenceFixture(t)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
}
func wantedClock75Move(t *testing.T, f *p1Fixture, a *model.AdminPrincipal, delta int64) context.Context {
	t.Helper()
	view, err := f.s.AdminServerGameClock(f.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminChangeServerGameClock(f.ctx, a, model.ServerGameClockChangeRequest{DeltaDays: delta, Revision: view.Revision, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	return wantedClock75Bind(t, f, f.id)
}
func wantedClock75Bind(t *testing.T, f *p1Fixture, id int64) context.Context {
	t.Helper()
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	ctx, err := f.s.BindServerGameClockContext(f.ctx, &owner)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
func wantedClock75Read(t *testing.T, id string) db.P12WantedFight {
	t.Helper()
	var f db.P12WantedFight
	if err := db.GetIns().First(&f, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	return f
}
func wantedClock75Start(t *testing.T, f *p1Fixture, ctx context.Context, id int64, at time.Time, tag string, stamp int64) db.P12WantedFight {
	t.Helper()
	own := *f
	own.id = id
	instance := p12WantedOwnershipInstance(t, &own, "wanted-clock75-"+tag, at)
	request := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	out, err := f.s.p12WantedStart(context.WithValue(ctx, constUtil.GameRequestContextKey, request), id, request, stamp, at)
	if err != nil {
		t.Fatal("actual original Start", tag, err)
	}
	return wantedClock75Read(t, out[0].(*pb.SCRebelFightStart).GetFightId())
}
func wantedClock75End(t *testing.T, f *p1Fixture, ctx context.Context, fight db.P12WantedFight, at time.Time) (*pb.CSRebelFightEnd, proto.Message) {
	t.Helper()
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("original certified terminal missing")
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	req := &pb.CSRebelFightEnd{FightId: proto.String(fight.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	out, err := f.s.p12WantedEnd(context.WithValue(ctx, constUtil.GameRequestContextKey, req), fight.PlayerID, req, at)
	if err != nil {
		t.Fatal("actual original End", err)
	}
	return req, out[0]
}

func TestWantedClock75ExpiredNativeLeaseDoesNotReviveAndReceiptStaysExact(t *testing.T) {
	f, a := wantedClock75Setup(t)
	ctx := wantedClock75Move(t, f, a, 6)
	forward := LocalPlayNow(ctx)
	// Native domain calls receive bounded historical times. No fight/receipt,
	// expiry or payment ledger is manually rewritten to manufacture expiration.
	committed := wantedClock75Start(t, f, ctx, f.id, forward.Add(-4*time.Hour), "committed", 7509101)
	end, reply := wantedClock75End(t, f, ctx, committed, forward.Add(-4*time.Hour))
	committed = wantedClock75Read(t, committed.ID)
	expired := wantedClock75Start(t, f, ctx, f.id, forward.Add(-2*time.Hour), "expired", 7509102)
	var instance db.P12WantedInstance
	if err := db.GetIns().First(&instance, "id=?", expired.InstanceID).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	ctx = wantedClock75Move(t, f, a, -15)
	after := wantedClock75Read(t, expired.ID)
	if after.AbandonedAt < forward.UnixMilli() || after.AbandonedAt > LocalPlayNow(ctx).AddDate(0, 0, 15).UnixMilli()+1000 {
		t.Fatal("expired presentation lease not sealed in old game date")
	}
	expected := expired
	expected.AbandonedAt = after.AbandonedAt
	if !reflect.DeepEqual(expected, after) {
		t.Fatal("expiration changed Start/Cost/hash or another accepted fight field")
	}
	var kept db.P12WantedInstance
	if err := db.GetIns().First(&kept, "id=?", instance.ID).Error; err != nil || !reflect.DeepEqual(instance, kept) {
		t.Fatal("instance HP, definition or original expiry changed", err)
	}
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error { return P12WantedRequireIdle(tx, f.id, LocalPlayNow(ctx)) }); err != nil {
		t.Fatal("expired lease revived after rewind", err)
	}
	if !reflect.DeepEqual(committed, wantedClock75Read(t, committed.ID)) {
		t.Fatal("committed receipt changed while sealing expiry")
	}
	out, err := f.s.p12WantedEnd(context.WithValue(ctx, constUtil.GameRequestContextKey, end), f.id, end, LocalPlayNow(ctx))
	if err != nil || !proto.Equal(reply, out[0]) {
		t.Fatal("committed original End lost exact replay after rewind", err)
	}
	saved, _ := f.profile(t)
	if saved.GetWantedAttTimes() != before.GetWantedAttTimes() || saved.GetScore() != before.GetScore() || saved.GetCoin() != before.GetCoin() || saved.GetDiamond() != before.GetDiamond() {
		t.Fatal("expiration/replay refunded attempt or granted assets")
	}
}

func TestWantedClock75ActiveNativeLeaseBlocksAndExpiryWriteRollsBack(t *testing.T) {
	f, a := wantedClock75Setup(t)
	ctx := wantedClock75Move(t, f, a, 6)
	forward := LocalPlayNow(ctx)
	expired := wantedClock75Start(t, f, ctx, f.id, forward.Add(-2*time.Hour), "rollback-expired", 7509201)
	// A second ordinary account owns a real independent Start, without copied
	// battle receipts or outcomes. Its first native roster is sufficient for admission.
	account, err := LoginAccount(f.ctx, "wanted-clock75-other-"+uuid.NewString(), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "独立悬赏计时", "hero_001"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, account.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(35); return nil }); err != nil {
		t.Fatal(err)
	}
	otherCtx := wantedClock75Bind(t, f, account.Id)
	active := wantedClock75Start(t, f, otherCtx, account.Id, forward, "rollback-active", 7509202)
	view, err := f.s.AdminServerGameClock(f.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	auditCount := p1Count(t, &db.ServerGameClockOperation{}, "server_id=?", int64(1))
	requestID := uuid.NewString()
	if _, err = f.s.AdminChangeServerGameClock(f.ctx, a, model.ServerGameClockChangeRequest{DeltaDays: -15, Revision: view.Revision, RequestID: requestID}); err == nil {
		t.Fatal("still-valid native fight allowed GM rewind")
	}
	next, err := f.s.AdminServerGameClock(f.ctx, a)
	if err != nil || next.OffsetDays != view.OffsetDays || next.Revision != view.Revision {
		t.Fatal("rejected adjustment committed calendar", err)
	}
	if !reflect.DeepEqual(expired, wantedClock75Read(t, expired.ID)) || !reflect.DeepEqual(active, wantedClock75Read(t, active.ID)) {
		t.Fatal("rejected adjustment failed to roll back early expiry sealing")
	}
	var after db.Profile
	if err = db.GetIns().First(&after, f.id).Error; err != nil || !reflect.DeepEqual(profile, after) {
		t.Fatal("rejected adjustment changed profile", err)
	}
	if p1Count(t, &db.ServerGameClockOperation{}, "server_id=?", int64(1)) != auditCount || p1Count(t, &db.ServerGameClockOperation{}, "request_id=?", requestID) != 0 {
		t.Fatal("rejected adjustment wrote a committed operation")
	}
}

func TestWantedClock75ExpiryRespectsInstanceAndProfileRealm(t *testing.T) {
	for _, foreign := range []string{"profile", "instance"} {
		t.Run(foreign, func(t *testing.T) {
			f, a := wantedClock75Setup(t)
			ctx := wantedClock75Move(t, f, a, 6)
			at := LocalPlayNow(ctx)
			original := wantedClock75Start(t, f, ctx, f.id, at.Add(-2*time.Hour), "foreign-"+foreign, 7509301)
			// Isolated negative realm-header prerequisite; the accepted fight/receipt
			// bytes and costs are untouched, and must remain untouched by realm1 repair.
			if foreign == "profile" {
				if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("server_id", 2).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := db.GetIns().Model(&db.P12WantedInstance{}).Where("id=?", original.InstanceID).Update("server_id", 2).Error; err != nil {
					t.Fatal(err)
				}
			}
			wantedClock75Move(t, f, a, -15)
			if !reflect.DeepEqual(original, wantedClock75Read(t, original.ID)) {
				t.Fatal("another realm's presentation receipt changed")
			}
		})
	}
}

func TestWantedClock75LegacyFuturePendingIsInertWithoutEarlySettlement(t *testing.T) {
	f, a := wantedClock75Setup(t)
	ctx := wantedClock75Move(t, f, a, 6)
	at := LocalPlayNow(ctx)
	committed := wantedClock75Start(t, f, ctx, f.id, at.Add(-2*time.Hour), "legacy-committed", 7509401)
	end, reply := wantedClock75End(t, f, ctx, committed, at.Add(-2*time.Hour))
	committed = wantedClock75Read(t, committed.ID)
	future := wantedClock75Start(t, f, ctx, f.id, at, "legacy-future", 7509402)
	// Legacy deployment prerequisite: only the old calendar offset/revision is
	// restored. The accepted pending fight is never backdated, abandoned or edited.
	if err := db.GetIns().Model(&db.ServerGameClock{}).Where("server_id=?", int64(1)).Updates(map[string]any{"offset_days": int64(-9), "revision": int64(2)}).Error; err != nil {
		t.Fatal(err)
	}
	ctx = wantedClock75Bind(t, f, f.id)
	now := LocalPlayNow(ctx)
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error { return P12WantedRequireIdle(tx, f.id, now) }); err != nil {
		t.Fatal("legacy future lease blocks current normal gameplay", err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(future.TerminalJSON), &terminal) != nil || terminal.Outcome == nil {
		t.Fatal("accepted future terminal")
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	early := &pb.CSRebelFightEnd{FightId: proto.String(future.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	if _, err := f.s.p12WantedEnd(context.WithValue(ctx, constUtil.GameRequestContextKey, early), f.id, early, now); err == nil {
		t.Fatal("legacy future pending settled before its accepted date")
	}
	if !reflect.DeepEqual(future, wantedClock75Read(t, future.ID)) {
		t.Fatal("inert future receipt was rewritten")
	}
	out, err := f.s.p12WantedEnd(context.WithValue(ctx, constUtil.GameRequestContextKey, end), f.id, end, now)
	if err != nil || !proto.Equal(reply, out[0]) || !reflect.DeepEqual(committed, wantedClock75Read(t, committed.ID)) {
		t.Fatal("legacy calendar blocked or changed committed replay", err)
	}
}

func TestWantedClock75LegacyForwardRecoveryAnchorRepairNoGrantAndIdempotent(t *testing.T) {
	f, a := wantedClock75Setup(t)
	forwardCtx := wantedClock75Move(t, f, a, 6)
	if _, err := f.s.p12WantedList(forwardCtx, f.id, LocalPlayNow(forwardCtx)); err != nil {
		t.Fatal("original forward list initializes recovery anchor", err)
	}
	// Spend one real original normal attempt so an accidental refill is visible.
	wantedClock75Start(t, f, forwardCtx, f.id, LocalPlayNow(forwardCtx), "legacy-anchor-paid", 7509501)
	forward, _ := f.profile(t)
	if forward.GetWantedAttTimes() != 9 || forward.GetWantedAttTime() <= time.Now().AddDate(0, 0, 5).UnixMilli() {
		t.Fatal("actual forward recovery precondition missing")
	}
	// Represent the original deployment rewind by restoring only its clock state.
	// No accepted wanted attempt, recovery cursor, fight or audit is rewritten.
	if err := db.GetIns().Model(&db.ServerGameClock{}).Where("server_id=?", int64(1)).Updates(map[string]any{"offset_days": int64(-9), "revision": int64(2)}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := wantedClock75Bind(t, f, f.id)
	target := LocalPlayNow(ctx)
	if forward.GetWantedAttTime()-9*86400000 <= target.UnixMilli() {
		t.Fatal("fixture does not exercise still-future legacy anchor after offset-only repair")
	}
	if p1Count(t, &db.AdminAudit{}, "action=? AND target=?", "calendar75.anchors", "1") != 0 {
		t.Fatal("new isolated fixture already repaired")
	}
	if err := f.s.RepairGameCalendar75(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	repaired, _ := f.profile(t)
	if repaired.GetWantedAttTimes() != forward.GetWantedAttTimes() || repaired.GetWantedAttTime() > LocalPlayNow(ctx).UnixMilli() || repaired.GetWantedAttTime() < target.UnixMilli()-1000 {
		t.Fatal("repair granted attempts or retained future last-recovery watermark")
	}
	if _, err := f.s.p12WantedList(ctx, f.id, LocalPlayNow(ctx)); err != nil {
		t.Fatal("original list remains blocked after legacy anchor repair", err)
	}
	before, beforeGame := f.profile(t)
	if before.GetWantedAttTimes() != forward.GetWantedAttTimes() {
		t.Fatal("repair/list minted historical recovery")
	}
	if err := f.s.RepairGameCalendar75(f.ctx, a); err != nil {
		t.Fatal("idempotent repair", err)
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, beforeGame, afterGame) || p1Count(t, &db.AdminAudit{}, "action=? AND target=?", "calendar75.anchors", "1") != 1 {
		t.Fatal("repeat repair changed profile, granted resources or duplicated repair audit")
	}
}
