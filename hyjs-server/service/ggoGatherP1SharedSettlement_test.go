package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// The public claim, deadline recovery and a late cancellation all enter the
// same settlement. Test the actual transaction, including a first freeze that
// must roll back the node, attempt and fact before an exclusive retry.
func TestGGOGatherP1SharedSettlementAndFirstFreeze(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	for _, action := range []string{"claim", "worker", "late-cancel"} {
		for _, alreadyFrozen := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/frozen=%t", action, alreadyFrozen), func(t *testing.T) {
				f := ggoPeriodLockSetup(t)
				f.setDue(t, 0)
				end := time.Now().Add(time.Hour)
				period := p1Instance(t, nil, "ggo-gather-shared-settlement", constUtil.P8EventMineGather, "cumulative", 100, end, 0)
				if alreadyFrozen {
					if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
						_, err := f.s.freezeP1Period(tx, period, true)
						return err
					}); err != nil {
						t.Fatal(err)
					}
				}
				var before db.GGOFieldLine
				if err := db.GetIns().First(&before, "server_id=? AND line=?", f.s.OriginalServerID(), 1).Error; err != nil {
					t.Fatal(err)
				}
				var beforeState ggoLineState
				if err := json.Unmarshal(before.Payload, &beforeState); err != nil {
					t.Fatal(err)
				}
				var beforeNode ggoResourceNode
				for _, node := range beforeState.Resources {
					if node.PointID == f.starts[0].PointID {
						beforeNode = node
					}
				}
				if beforeNode.ReservedBy != f.ids[0] || beforeNode.AttemptToken != f.starts[0].AttemptToken {
					t.Fatal("fixture lost the accepted reservation")
				}
				var modes []bool
				const callback = "test:ggo-gather-shared-settlement"
				database := db.GetIns()
				if err := database.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					fact, ok := tx.Statement.Dest.(*db.EventFact)
					if ok && fact.PlayerID == f.ids[0] && fact.SourceKey == "ggo:gather:"+f.starts[0].AttemptToken {
						modes = append(modes, p1SharedRead(tx))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = database.Callback().Create().Remove(callback) })
				// Even an inherited shared marker must be cleared during retry.
				ctx := context.WithValue(f.ctx, constUtil.P1SharedReadContextKey{}, true)
				var claim *model.GGOFieldGatherClaimResponse
				var cancelled *model.GGOFieldGatherCancelResponse
				var err error
				switch action {
				case "claim":
					claim, err = f.s.GGOFieldGatherClaim(ctx, f.claimRequest(0), f.sessions[0])
				case "worker":
					err = f.s.ggoFinishDue(ctx, f.ids[0], f.starts[0].AttemptToken)
				case "late-cancel":
					cancelled, err = f.s.GGOFieldGatherCancel(ctx, f.cancelRequest(0), f.sessions[0])
				}
				if err != nil {
					t.Fatalf("settlement: %v", err)
				}
				wantModes := []bool{true}
				if !alreadyFrozen {
					wantModes = []bool{true, false}
				}
				if !reflect.DeepEqual(modes, wantModes) {
					t.Fatalf("settlement modes %v, want %v", modes, wantModes)
				}
				var after db.GGOFieldLine
				var attempt db.P8GatherAttempt
				var current db.PeriodInstance
				var progress db.PeriodProgress
				for _, query := range []func() error{
					func() error { return database.First(&after, "server_id=? AND line=?", f.s.OriginalServerID(), 1).Error },
					func() error { return database.First(&attempt, "player_id=?", f.ids[0]).Error },
					func() error { return database.First(&current, "id=?", period.ID).Error },
					func() error {
						return database.First(&progress, "instance_id=? AND player_id=?", period.ID, f.ids[0]).Error
					},
				} {
					if err := query(); err != nil {
						t.Fatal(err)
					}
				}
				var afterState ggoLineState
				if err := json.Unmarshal(after.Payload, &afterState); err != nil {
					t.Fatal(err)
				}
				var afterNode ggoResourceNode
				for _, node := range afterState.Resources {
					if node.PointID == beforeNode.PointID {
						afterNode = node
					}
				}
				if attempt.Active || after.Revision != before.Revision+1 || afterNode.Remaining != beforeNode.Remaining-1 ||
					afterNode.ReservedBy != 0 || afterNode.AttemptToken != "" || len(current.Frozen) == 0 || progress.Value != 1 ||
					p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "ggo:gather:"+attempt.Token) != 1 ||
					p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND operation=? AND business_key=?", f.ids[0], constUtil.GGOFieldOpGatherClaim, attempt.Token) != 1 {
					t.Fatal("settlement did not commit exactly one node consumption, activity fact and claim")
				}
				switch action {
				case "claim":
					replay, e := f.s.GGOFieldGatherClaim(ctx, f.claimRequest(0), f.sessions[0])
					if e != nil || claim == nil || !reflect.DeepEqual(claim, replay) {
						t.Fatalf("claim replay: %+v %+v %v", claim, replay, e)
					}
				case "worker":
					if e := f.s.ggoFinishDue(ctx, f.ids[0], f.starts[0].AttemptToken); e != nil {
						t.Fatalf("worker replay: %v", e)
					}
				case "late-cancel":
					replay, e := f.s.GGOFieldGatherCancel(ctx, f.cancelRequest(0), f.sessions[0])
					if e != nil || cancelled == nil || cancelled.Status != "settled" || !reflect.DeepEqual(cancelled, replay) {
						t.Fatalf("late cancellation replay: %+v %+v %v", cancelled, replay, e)
					}
				}
				if !reflect.DeepEqual(modes, wantModes) || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "ggo:gather:"+attempt.Token) != 1 {
					t.Fatal("replay produced another gather fact")
				}
			})
		}
	}
}

func TestGGOGatherWithoutP1ReadyKeepsOriginalFactPath(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoPeriodLockSetup(t)
	f.setDue(t, 0)
	// The legacy partial-service configuration may retain event tables without
	// having installed the new activity catalog guard.
	f.s.P1Ready = false
	database := db.GetIns()
	const callback = "test:ggo-no-uninitialized-p1-catalog"
	if err := database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p1_gather_catalog_guards" {
			tx.AddError(fmt.Errorf("uninitialized P1 catalog was queried"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(callback) })
	first, err := f.s.GGOFieldGatherClaim(f.ctx, f.claimRequest(0), f.sessions[0])
	if err != nil || first == nil {
		t.Fatalf("partial-service claim: %+v %v", first, err)
	}
	again, err := f.s.GGOFieldGatherClaim(f.ctx, f.claimRequest(0), f.sessions[0])
	if err != nil || !reflect.DeepEqual(first, again) ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "ggo:gather:"+f.starts[0].AttemptToken) != 1 {
		t.Fatalf("partial-service claim lost its original fact or replay: %+v %v", again, err)
	}
}
