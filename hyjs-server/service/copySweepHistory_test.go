package service

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This fixture runs on a private SQLite database by default. Setting the
// established HYJS_P1_MYSQL_* lease runs the same assertions on InnoDB.
func TestCopySweepHistoryRollbackAndExactlyOnce(t *testing.T) {
	f := p1Setup(t)
	for _, init := range []func(context.Context) error{f.s.InitCooking, f.s.InitializeDailyCopy, f.s.InitializeCopyDrops, f.s.InitializeDailyTasks, f.s.InitializeP9CopyIntimacy, f.s.InitializeCopySweepWitness} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P1Ready = true
	f.s.P9PetsReady = true
	now := time.Now()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Stamina, p.Exp = proto.Int32(100), proto.Int32(1000), proto.Int64(0)
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{
			{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5)},
			{ChapterId: proto.String("1"), MissionId: proto.String("2"), Star: proto.Int32(5)},
		}
		refreshCopySweepDay(g, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	other := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("2")}
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, req)
	var account db.Profile
	if err := db.GetIns().First(&account, f.id).Error; err != nil {
		t.Fatal(err)
	}
	oldSession := append([]byte(nil), account.Session...)
	if _, err := AuthorizeAccount(f.ctx, f.id, oldSession); err != nil {
		t.Fatal("old session preflight", err)
	}
	newAccount, err := LoginAccount(f.ctx, account.Uid, "isolated-test-password")
	if err != nil {
		t.Fatal("rotate session", err)
	}
	stale := context.WithValue(ctx, ggoNativeSessionKey{}, oldSession)
	if _, err := f.s.copySweepAt(stale, f.id, req, 8000, now); err == nil {
		t.Fatal("session rotated after preflight still settled")
	}
	ctx = context.WithValue(ctx, ggoNativeSessionKey{}, append([]byte(nil), newAccount.Session...))
	call := func(request *pb.CSCopySweep, stamp int64) ([]proto.Message, error) {
		return f.s.copySweepAt(context.WithValue(ctx, constUtil.GameRequestContextKey, request), f.id, request, stamp, now)
	}
	counts := func() (int64, int64) {
		t.Helper()
		return p1Count(t, &db.P9PetCopyGrant{}, "player_id=? AND source_key LIKE ?", f.id, "sweep:%"),
			p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P1EventCopyWin)
	}
	state := func() (*pb.Player, *model.GameState) { return f.profile(t) }
	witness := func() db.CopySweepWitness {
		t.Helper()
		var row db.CopySweepWitness
		if err := db.GetIns().First(&row, "player_id=?", f.id).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	for _, stamp := range []int64{9000, 10000} {
		if _, err := call(req, stamp); err != nil {
			t.Fatal("legacy receipt setup", stamp, err)
		}
	}
	if grants, events := counts(); grants != 2 || events != 2 {
		t.Fatal("initial committed sources", grants, events)
	}
	// Model an old-format profile from a fully covered server. The retained P9
	// rows were produced by actual successful sweeps above.
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.CopySweepWitnessInitialized = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("player_id=?", f.id).Delete(&db.CopySweepWitness{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := call(req, 1000); err == nil {
		t.Fatal("default policy accepted an unproven lower timestamp")
	}
	f.s.CopySweepHistoryComplete = true
	beforeLow, beforeLowGame := state()
	first, err := call(req, 1000)
	if err != nil {
		t.Fatal("covered lower timestamp", err)
	}
	paid, saved := state()
	result := first[0].(*pb.SCCopySweep)
	if paid.GetStamina() != beforeLow.GetStamina()-10 || len(saved.Warriors) != len(beforeLowGame.Warriors)+len(result.GetGain().GetWars()) {
		t.Fatal("lower timestamp did not pay and receive its actual drops once")
	}
	if current := witness(); saved.CopySweepReceipt.Timestamp != 10000 || current.MaxStamp != 10000 || current.LastStamp != 1000 {
		t.Fatal("legacy watermark or latest receipt changed", saved.CopySweepReceipt, current)
	}
	if grants, events := counts(); grants != 3 || events != 3 {
		t.Fatal("lower timestamp not counted once", grants, events)
	}
	replayed, err := call(req, 1000)
	if err != nil || !proto.Equal(first[0], replayed[0]) {
		t.Fatal("latest lower receipt", err)
	}
	for _, test := range []struct {
		request *pb.CSCopySweep
		stamp   int64
	}{{req, 9000}, {req, 10000}, {other, 9000}, {other, 1000}} {
		if _, err := call(test.request, test.stamp); err == nil {
			t.Fatal("historical or cross-mission collision accepted", test.stamp)
		}
	}
	afterReplay, replayState := state()
	if !proto.Equal(paid, afterReplay) || !reflect.DeepEqual(saved, replayState) {
		t.Fatal("replay or collision mutated the profile")
	}
	// Concurrent retries have one settlement and one event, even below the old watermark.
	var wg sync.WaitGroup
	replies := make([][]proto.Message, 6)
	errs := make([]error, len(replies))
	for i := range replies {
		wg.Add(1)
		go func(i int) { defer wg.Done(); replies[i], errs[i] = call(req, 1001) }(i)
	}
	wg.Wait()
	for i := range replies {
		if errs[i] != nil || len(replies[i]) == 0 || !proto.Equal(replies[0][0], replies[i][0]) {
			t.Fatal("concurrent replay", i, errs[i])
		}
	}
	if grants, events := counts(); grants != 4 || events != 4 {
		t.Fatal("concurrent settlement duplicated", grants, events)
	}
	var day db.DailyTaskState
	if err := db.GetIns().Where("player_id=?", f.id).First(&day).Error; err != nil {
		t.Fatal(err)
	}
	var daily dailyTaskSnapshot
	if err := json.Unmarshal([]byte(day.Snapshot), &daily); err != nil {
		t.Fatal(err)
	}
	for _, task := range daily.Tasks {
		if task.GetType() == constUtil.DailyTaskCopy && task.GetCount() != 3 {
			t.Fatal("daily copy progress", task)
		}
	}
	// An error after changing in-memory stamina and coin rolls back all records.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level, p.Exp = proto.Int32(14), proto.Int64(279)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	level := f.s.table("UserExp", "level", "14")
	oldExp := level["exp"]
	level["exp"] = "0"
	beforeP, beforeG := state()
	if _, err := call(req, 1002); err == nil {
		t.Fatal("corrupt experience table accepted")
	}
	level["exp"] = oldExp
	afterP, afterG := state()
	if !proto.Equal(beforeP, afterP) || !reflect.DeepEqual(beforeG, afterG) {
		t.Fatal("failed settlement persisted")
	}
	if grants, events := counts(); grants != 4 || events != 4 {
		t.Fatal("failed settlement retained a grant or event", grants, events)
	}
	if _, err := call(req, 1002); err != nil {
		t.Fatal("retry after rollback", err)
	}
	// Reopening retains the newest result and the old high watermark.
	f.restart(t)
	_, saved = state()
	if current := witness(); saved.CopySweepReceipt.Timestamp != 10000 || current.LastStamp != 1002 {
		t.Fatal("restart lost receipt identities")
	}
	if _, err := call(req, 1002); err != nil {
		t.Fatal("restart latest retry", err)
	}
	// The initialized marker prevents treating a deleted witness as a legacy
	// profile. Restoring this isolated fixture does not touch another role.
	validWitness := witness()
	if err := db.GetIns().Delete(&validWitness).Error; err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG = state()
	if _, err := call(req, 1002); err == nil {
		t.Fatal("missing initialized witness accepted")
	}
	afterP, afterG = state()
	if !proto.Equal(beforeP, afterP) || !reflect.DeepEqual(beforeG, afterG) {
		t.Fatal("missing witness mutated the profile")
	}
	if err := db.GetIns().Create(&validWitness).Error; err != nil {
		t.Fatal(err)
	}
	f.s.P9PetsReady = false
	if _, err := call(req, 1003); err == nil {
		t.Fatal("history assertion accepted an unready grant store")
	}
	if _, err := call(req, 1002); err == nil {
		t.Fatal("history assertion replayed while the grant store was unready")
	}
	f.s.P9PetsReady = true
	f.s.CopySweepHistoryComplete = false
	if _, err := call(req, 1003); err == nil {
		t.Fatal("disabled policy lost the legacy watermark")
	}
	f.s.CopySweepHistoryComplete = true
	// An old binary can rewrite the profile without touching the witness.
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.CopySweepWitnessInitialized = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(req, 1002); err != nil {
		t.Fatal("old profile rewrite lost latest witness", err)
	}
	_, rewritten := state()
	if !rewritten.CopySweepWitnessInitialized {
		t.Fatal("new binary did not restore the witness marker")
	}
	if _, err := call(req, 10000); err == nil {
		t.Fatal("stale high watermark was replayed as the latest receipt")
	}
	// Simulate an old binary accepting a newer stamp while the new binary is
	// stopped; it updates the legacy profile and P9 but knows no witness table.
	f.s.CopySweepHistoryComplete, f.s.CopySweepWitnessReady = false, false
	oldResponse, err := call(req, 20000)
	if err != nil {
		t.Fatal("old binary higher watermark", err)
	}
	f.s.CopySweepHistoryComplete, f.s.CopySweepWitnessReady = true, true
	recovered, err := call(req, 20000)
	if err != nil || !proto.Equal(oldResponse[0], recovered[0]) || witness().LastStamp != 20000 {
		t.Fatal("old binary higher watermark not reconciled", err)
	}
	if _, err := call(req, 1002); err == nil {
		t.Fatal("older witness receipt replayed after old-binary settlement")
	}
	// Unknown protobuf fields are part of the exact request retained by the
	// witness, even though the legacy profile stores its visible JSON shape.
	extended := proto.Clone(req).(*pb.CSCopySweep)
	extended.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
	extendedResponse, err := call(extended, 21000)
	if err != nil {
		t.Fatal("new max with protobuf extension", err)
	}
	if replay, err := call(extended, 21000); err != nil || !proto.Equal(extendedResponse[0], replay[0]) {
		t.Fatal("protobuf extension retry", err)
	}
	changed := proto.Clone(extended).(*pb.CSCopySweep)
	changed.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x02})
	if _, err := call(changed, 21000); err == nil {
		t.Fatal("different protobuf extension was treated as the same request")
	}
	// A rollback or alteration of the legacy watermark never silently resets
	// the witness. Restore the test row only after checking the failure.
	valid := witness()
	if err := db.GetIns().Model(&db.CopySweepWitness{}).Where("player_id=?", f.id).Update("max_stamp", valid.MaxStamp+1).Error; err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG = state()
	if _, err := call(req, 30000); err == nil {
		t.Fatal("witness ahead of profile accepted")
	}
	afterP, afterG = state()
	if !proto.Equal(beforeP, afterP) || !reflect.DeepEqual(beforeG, afterG) {
		t.Fatal("inconsistent witness mutated the profile")
	}
	if err := db.GetIns().Save(&valid).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.CopySweepWitness{}).Where("player_id=?", f.id).Update("max_digest", make([]byte, 32)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := call(extended, 21000); err == nil {
		t.Fatal("same-stamp watermark digest mismatch accepted")
	}
	if err := db.GetIns().Save(&valid).Error; err != nil {
		t.Fatal(err)
	}
	// Missing known history must reject the latest replay under the asserted
	// coverage policy; only this isolated test database is modified.
	var grant db.P9PetCopyGrant
	if err := db.GetIns().First(&grant, "player_id=? AND source_key=?", f.id, "sweep:21000:1/1").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Delete(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := call(extended, 21000); err == nil {
		t.Fatal("missing latest grant accepted")
	}
	if err := db.GetIns().Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Migrator().DropTable(&db.P9PetCopyGrant{}); err != nil {
		t.Fatal(err)
	}
	beforeP, beforeG = state()
	if _, err := call(extended, 21000); err == nil {
		t.Fatal("unavailable grant table accepted a replay")
	}
	afterP, afterG = state()
	if !proto.Equal(beforeP, afterP) || !reflect.DeepEqual(beforeG, afterG) {
		t.Fatal("unavailable grant table mutated the profile")
	}
}

func TestCopySweepHistoryPolicyCopiesIntoRuntime(t *testing.T) {
	base := &GameService{CopySweepHistoryComplete: true, CopySweepWitnessReady: true, P9PetsReady: true}
	clone := base.withRuntimeTables(map[string][]map[string]string{})
	if !clone.CopySweepHistoryComplete || !clone.CopySweepWitnessReady || !clone.P9PetsReady {
		t.Fatal("runtime overlay lost sweep history policy")
	}
}

type copySweepSessionBarrierKey struct{}

// MySQL permits the old request to pause before its profile lock while a real
// login rotates the same role's session in a separate transaction.
func TestCopySweepHistoryMySQLSessionRotationBeforeProfileLock(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an independently leased InnoDB database")
	}
	f := p1Setup(t)
	for _, init := range []func(context.Context) error{f.s.InitCooking, f.s.InitializeDailyCopy, f.s.InitializeCopyDrops, f.s.InitializeP9CopyIntimacy, f.s.InitializeCopySweepWitness} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P9PetsReady = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Stamina = proto.Int32(100), proto.Int32(1000)
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("3"), MissionId: proto.String("12"), Star: proto.Int32(5)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var account db.Profile
	if err := db.GetIns().First(&account, f.id).Error; err != nil {
		t.Fatal(err)
	}
	oldSession := append([]byte(nil), account.Session...)
	if _, err := AuthorizeAccount(f.ctx, f.id, oldSession); err != nil {
		t.Fatal("old packet preflight", err)
	}
	beforeP, beforeG := f.profile(t)
	reached, release := make(chan struct{}), make(chan struct{})
	var signal, unblock sync.Once
	defer unblock.Do(func() { close(release) })
	callback := "copy-sweep:pause-before-profile-lock"
	if err := db.GetIns().Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(copySweepSessionBarrierKey{}) == true && tx.Statement.Table == "profiles" {
			if _, locked := tx.Statement.Clauses["FOR"]; locked {
				signal.Do(func() { close(reached); <-release })
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(callback)
	oldCtx := context.WithValue(f.ctx, copySweepSessionBarrierKey{}, true)
	oldCtx = context.WithValue(oldCtx, ggoNativeSessionKey{}, oldSession)
	request := &pb.CSCopySweep{ChapterId: proto.String("3"), MissionId: proto.String("12")}
	result := make(chan error, 1)
	go func() {
		_, err := f.s.copySweepAt(oldCtx, f.id, request, 7000, time.Now())
		result <- err
	}()
	select {
	case <-reached:
	case <-time.After(15 * time.Second):
		t.Fatal("old request did not reach the profile lock")
	}
	newAccount, err := LoginAccount(f.ctx, account.Uid, "isolated-test-password")
	if err != nil {
		t.Fatal("concurrent login", err)
	}
	unblock.Do(func() { close(release) })
	select {
	case err := <-result:
		if err == nil || err.Error() != constUtil.CopySweepSessionError {
			t.Fatal("old request survived session rotation", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("old request did not leave profile lock")
	}
	afterP, afterG := f.profile(t)
	if !proto.Equal(beforeP, afterP) || !reflect.DeepEqual(beforeG, afterG) || p1Count(t, &db.P9PetCopyGrant{}, "player_id=?", f.id) != 0 {
		t.Fatal("old request changed settlement state")
	}
	newCtx := context.WithValue(f.ctx, ggoNativeSessionKey{}, append([]byte(nil), newAccount.Session...))
	if _, err := f.s.copySweepAt(newCtx, f.id, request, 7000, time.Now()); err != nil {
		t.Fatal("new session could not settle", err)
	}
}
