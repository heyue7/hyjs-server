package service

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// Calendar mutations use the same real-to-game conversion as original queries.
// The finite spent-quota precondition exercises archival conservation, not battle proof.
func TestP7ReRushClock75OriginalQueryMutationSingleOffsetAndQuota(t *testing.T) {
	f := p7Setup(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	move := func(delta int64) context.Context {
		t.Helper()
		view, err := f.s.AdminServerGameClock(f.ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.AdminChangeServerGameClock(f.ctx, actor, model.ServerGameClockChangeRequest{DeltaDays: delta, Revision: view.Revision, RequestID: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
		var owner db.Profile
		if err = db.GetIns().First(&owner, f.id).Error; err != nil {
			t.Fatal(err)
		}
		ctx, err := f.s.BindServerGameClockContext(f.ctx, &owner)
		if err != nil {
			t.Fatal(err)
		}
		return ctx
	}
	read := func(ctx context.Context) db.P7RushState {
		t.Helper()
		if _, err := f.s.P7RushQuery(ctx, f.id, &pb.CSGetRushEliteBossInfo{}, time.Now()); err != nil {
			t.Fatal("original query", err)
		}
		var state db.P7RushState
		if err := db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil {
			t.Fatal(err)
		}
		return state
	}
	var stamp int64 = 7508000
	for i, delta := range []int64{6, -15, 15, -15} {
		ctx := move(delta)
		day := p1Day(LocalPlayNow(ctx))
		state := read(ctx)
		if state.Day != day {
			t.Fatalf("query day=%s want=%s", state.Day, day)
		}
		if i < 2 {
			if state.FreeResetUsed != 0 || state.PaidResetUsed != 0 || state.EliteUsed != 0 || state.EliteBought != 0 {
				t.Fatal("unvisited day inherited spent quota")
			}
			if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				current, err := p7State(tx, f.id, time.Now())
				if err != nil {
					return err
				}
				current.FreeResetUsed, current.PaidResetUsed, current.EliteUsed, current.EliteBought = 1, 1, 1, 1
				return saveP7RushCalendar(tx, current, time.Now())
			}); err != nil {
				t.Fatal(err)
			}
		}
		beforeP, beforeG := f.profile(t)
		stamp++
		out, handled, err := f.s.p7ReRushMutation(ctx, f.id, &pb.CSRush3Hit{}, stamp)
		if err != nil || !handled || out[0].(*pb.SCRush3Hit).GetIsValid() != 0 {
			t.Fatal("actual uncleared sweep must be a non-rewarding native mutation", err)
		}
		if err := db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil {
			t.Fatal(err)
		}
		if state.Day != day {
			t.Fatalf("native mutation applied calendar offset twice: day=%s want=%s", state.Day, day)
		}
		state = read(ctx)
		if state.Day != day || state.FreeResetUsed != 1 || state.PaidResetUsed != 1 || state.EliteUsed != 1 || state.EliteBought != 1 {
			t.Fatalf("mutation/query split the day or reopened quota: %+v", state)
		}
		afterP, afterG := f.profile(t)
		if afterP.GetCoin() != beforeP.GetCoin() || afterP.GetPoint() != beforeP.GetPoint() || afterP.GetDiamond() != beforeP.GetDiamond() || len(afterG.Warriors) != len(beforeG.Warriors) {
			t.Fatal("no-qualification sweep granted assets")
		}
		if accidental, err := readLocalPlayRushQuota(db.GetIns(), f.id, p1Day(time.Now().AddDate(0, 0, int(GameCalendarOffsetDays(ctx)*2)))); err != nil || accidental != nil {
			t.Fatal("double-offset shadow quota was created", err)
		}
	}
}
