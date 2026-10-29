package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP12Wanted84LateRejectedEndKeepsRecoveredLease(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	start, end, recovery := p12Wanted84RejectedFight(t, f, "wanted84-late-rejection", 984401, now)
	paid, _ := f.profile(t)
	var rejected db.P12WantedFight
	if err := db.GetIns().First(&rejected, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.RejectedAt <= 0 || rejected.RejectedReason != constUtil.P6ReplayConflict || rejected.RecoveryLeaseActive {
		t.Fatal("initial rejection did not release the paid fight")
	}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, recovery)
	recovered, err := f.s.p12WantedStart(startCtx, f.id, recovery, 984402, now)
	if err != nil || len(recovered) != 1 || !proto.Equal(recovered[0], start) {
		t.Fatal("paid recovery did not return the original Start", err)
	}

	// Repeat the same failed End after reacquisition, modeling a late request.
	bad := proto.Clone(end).(*pb.CSRebelFightEnd)
	bad.OptRecord.DefenderTotalLeftHp = proto.Float64(bad.OptRecord.GetDefenderTotalLeftHp() + 1)
	badCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, bad)
	_, err = f.s.p12WantedEnd(badCtx, f.id, bad, now.Add(time.Second))
	var retry *P12WantedRetryableEndError
	if !errors.As(err, &retry) {
		t.Fatal("late rejected End lost its recoverable classification", err)
	}
	var afterLate db.P12WantedFight
	if err = db.GetIns().First(&afterLate, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if afterLate.Committed || afterLate.AbandonedAt != 0 || !afterLate.RecoveryLeaseActive ||
		afterLate.RejectedAt != rejected.RejectedAt || afterLate.RejectedReason != rejected.RejectedReason {
		t.Fatal("late rejected End released the lease or replaced the original rejection")
	}
	idle := func(at time.Time) error {
		return f.s.p12WantedProfile(f.ctx, f.id, func(tx *gorm.DB, _ *db.Profile, _ *pb.Player, _ *model.GameState) error {
			return P12WantedRequireIdle(tx, f.id, at)
		})
	}
	if err = idle(now.Add(time.Second)); err == nil || err.Error() != constUtil.P12WantedBusy {
		t.Fatal("recovered fight failed to exclude other modes", err)
	}
	recovered, err = f.s.p12WantedStart(startCtx, f.id, recovery, 984403, now.Add(time.Second))
	if err != nil || len(recovered) != 1 || !proto.Equal(recovered[0], start) {
		t.Fatal("recovered lease did not retain the original paid Start", err)
	}
	afterRetry, _ := f.profile(t)
	if afterRetry.GetWantedAttTimes() != paid.GetWantedAttTimes() || p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != 1 {
		t.Fatal("late rejection or repeated recovery charged another attempt")
	}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	replies, err := f.s.p12WantedEnd(endCtx, f.id, end, now.Add(2*time.Second))
	if err != nil || len(replies) != 1 {
		t.Fatal("valid End did not settle the recovered fight", err)
	}
	if err = idle(now.Add(2 * time.Second)); err != nil {
		t.Fatal("settled fight still blocked other modes", err)
	}
	afterEnd, _ := f.profile(t)
	accepted := replies[0].(*pb.SCRebelFightEnd)
	if afterEnd.GetWantedAttTimes() != paid.GetWantedAttTimes() || afterEnd.GetScore()-paid.GetScore() != accepted.GetScoreAdd() {
		t.Fatal("settlement changed attempts or score more than once")
	}
}
