package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestP1GatherCatalogTransactionProofAndUpgrade(t *testing.T) {
	f := p1GatherFixture(t)
	ctx := p1GatherContext(f)
	var stale context.Context
	rollback := errors.New("discard test transaction")
	err := db.GetIns().WithContext(p1GatherCatalogContext(ctx)).Transaction(func(tx *gorm.DB) error {
		if err := p1RequireGatherCatalog(tx, constUtil.P8EventMineGather, false); err == nil {
			t.Fatal("marker alone authorized a gather")
		}
		guarded, err := p1AcquireGatherCatalog(tx)
		if err != nil {
			return err
		}
		for _, kind := range p1GatherCatalogKinds() {
			if err = p1RequireGatherCatalog(guarded, kind, false); err != nil {
				return err
			}
			if !errors.Is(p1RequireGatherCatalog(guarded, kind, true), constUtil.ErrP1GatherCatalogWriteRetry) {
				t.Fatal("read lease authorized publication")
			}
		}
		if _, err = p1AcquireGatherCatalogWriter(guarded); !errors.Is(err, constUtil.ErrP1GatherCatalogWriteRetry) {
			t.Fatalf("read-to-write upgrade did not request rollback: %v", err)
		}
		stale = guarded.Statement.Context
		return rollback
	})
	if !errors.Is(err, rollback) || stale == nil {
		t.Fatalf("read lease transaction failed: %v", err)
	}
	err = db.GetIns().WithContext(stale).Transaction(func(tx *gorm.DB) error {
		if err := p1RequireGatherCatalog(tx, constUtil.P8EventMineGather, false); err == nil {
			t.Fatal("a prior transaction's lease was accepted")
		}
		guarded, err := p1AcquireGatherCatalogWriter(tx)
		if err != nil {
			return err
		}
		for _, kind := range p1GatherCatalogKinds() {
			if err := p1RequireGatherCatalog(guarded, kind, true); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestP1GatherCatalogPartialServiceRetainsOriginalFact(t *testing.T) {
	f := p1GatherFixture(t)
	attempt := &db.P8GatherAttempt{PlayerID: f.id, Token: "catalog-partial-service"}
	ctx := p1GatherContext(f)
	write := func(tx *gorm.DB) error {
		return f.s.recordP1GatherEvent(tx, f.id, attempt, constUtil.P8EventMineGather, true, time.Now())
	}
	if err := db.GetIns().WithContext(ctx).Transaction(write); err == nil {
		t.Fatal("enabled P1 accepted an unguarded gather")
	}
	f.s.P1Ready = false
	shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
	if err := db.GetIns().WithContext(shared).Transaction(write); !errors.Is(err, constUtil.ErrP1ExclusiveRetry) {
		t.Fatalf("partial service did not preserve the exclusive event path: %v", err)
	}
	if err := db.GetIns().WithContext(ctx).Transaction(write); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "ggo:gather:"+attempt.Token) != 1 {
		t.Fatal("partial service lost its original gather fact")
	}
}

func TestP1GatherCatalogTypedChangeProtectsOldAndNewKind(t *testing.T) {
	f := p1GatherFixture(t)
	end := time.Now().Add(time.Hour)
	mine := p1Instance(t, f, "p1_catalog_old_mine", constUtil.P8EventMineGather, "cumulative", 1, end, 0)
	other := p1Instance(t, f, "p1_catalog_new_fish", constUtil.P1EventLoginDay, "cumulative", 1, end, 0)
	form := model.AdminActivityForm{Mode: "isolated", Name: "目录种类测试", Source: "隔离测试", StartAt: end.Add(-time.Hour).UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.Add(time.Hour).UnixMilli(), Milestones: []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 1}}}}}
	for _, test := range []struct {
		id, oldKind, newKind string
	}{
		{mine.ID, constUtil.P8EventMineGather, constUtil.P1EventLoginDay},
		{other.ID, constUtil.P1EventLoginDay, constUtil.P8EventFishGather},
	} {
		definition := adminActivityDefinition{mode: "isolated", domain: "period", kind: test.newKind}
		persist := func(tx *gorm.DB) error {
			return f.s.adminPersistTypedActivity(tx, definition, test.id, form, true, true)
		}
		err := db.GetIns().Transaction(persist)
		if !errors.Is(err, constUtil.ErrP1GatherCatalogWriteRetry) {
			t.Fatalf("kind change %s -> %s lacked a write retry: %v", test.oldKind, test.newKind, err)
		}
		var unchanged db.PeriodInstance
		if err := db.GetIns().First(&unchanged, "id=?", test.id).Error; err != nil || unchanged.Kind != test.oldKind {
			t.Fatalf("failed kind change persisted: %+v %v", unchanged, err)
		}
		err = db.GetIns().Transaction(func(tx *gorm.DB) error {
			guarded, err := p1AcquireGatherCatalogWriter(tx)
			if err != nil {
				return err
			}
			return persist(guarded)
		})
		if err != nil {
			t.Fatalf("guarded kind change %s -> %s failed: %v", test.oldKind, test.newKind, err)
		}
		var changed db.PeriodInstance
		if err := db.GetIns().First(&changed, "id=?", test.id).Error; err != nil || changed.Kind != test.newKind {
			t.Fatalf("guarded kind change was not committed: %+v %v", changed, err)
		}
	}
}

func TestP1GatherCatalogPersonalPeriodFullRetry(t *testing.T) {
	f, playerID := localPlayActivitiesFixture(t)
	now := time.Now()
	// The personal cycle copies the latest matching source, including a kind
	// supplied through the generic period editor. Keep this source newest.
	source := db.PeriodInstance{ID: "mainCopyWinTimes_catalog_guard", Kind: constUtil.P8EventMineGather, Mode: "cumulative", Enabled: true, Title: "采集目录复制测试", Goal: 1, Rewards: "coin,1", StartAt: now.AddDate(5, 0, 0).UnixMilli(), EndAt: now.AddDate(5, 0, 1).UnixMilli(), RewardEndAt: now.AddDate(5, 0, 2).UnixMilli(), Source: "隔离测试"}
	if err := db.GetIns().Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	var def localPlayActivityDefinition
	for _, candidate := range localPlayActivityDefinitions {
		if candidate.ID == "MainCopy" {
			def = candidate
		}
	}
	cycleID := "lp-00000000000000000000000001"
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		_, err := f.s.localPlayCreateScopedCycle(tx, playerID, def, &db.LocalPlayActivityCycle{ID: cycleID}, now)
		return err
	})
	if !errors.Is(err, constUtil.ErrP1GatherCatalogWriteRetry) {
		t.Fatalf("unguarded personal mine clone was not rolled back: %v", err)
	}
	if p1Count(t, &db.PeriodInstance{}, "id LIKE ?", "%"+cycleID+"%") != 0 {
		t.Fatal("failed personal clone left an instance behind")
	}
	const requestID = "cc000000-0000-4000-8000-000000000001"
	first, err := f.s.LocalPlayActivitiesApply(f.ctx, playerID, "refresh", "MainCopy", requestID)
	if err != nil || len(first.Results) != 1 || first.Results[0].Status != "open" {
		t.Fatalf("personal operation did not retry with the directory write guard: %+v %v", first, err)
	}
	replay, err := f.s.LocalPlayActivitiesApply(f.ctx, playerID, "refresh", "MainCopy", requestID)
	if err != nil || len(replay.Results) != 1 || replay.Results[0].Status != "open" {
		t.Fatalf("personal operation replay changed: %+v %v", replay, err)
	}
	cycleID = localPlayCycleID(t, f, playerID, "MainCopy")
	var copied db.PeriodInstance
	if err := db.GetIns().Where("id LIKE ? AND kind=?", "%"+cycleID+"%", constUtil.P8EventMineGather).First(&copied).Error; err != nil {
		t.Fatal(err)
	}
	if len(copied.Frozen) != 0 || p1Count(t, &db.PeriodInstance{}, "id LIKE ? AND kind=?", "%"+cycleID+"%", constUtil.P8EventMineGather) != 1 ||
		p1Count(t, &db.LocalPlayActivityOperation{}, "player_id=? AND request_id=?", playerID, requestID) != 1 {
		t.Fatal("retry or replay duplicated the personal period or receipt")
	}
}
