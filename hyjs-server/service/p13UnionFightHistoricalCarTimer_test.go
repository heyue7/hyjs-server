package service

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"strconv"
	"testing"
	"time"
)

// Real field/factory producers supply cars and quotes. The directory fixture
// exercises only historical timer compatibility; it grants no combat outcome.
func TestP13MixedHistoricalNoncausalCarTimersRealProducerColdAndReject(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	for _, model := range []any{&db.P13UnionFightFactorySource{}, &db.P13UnionFightCarMotionSource{}, &db.P13UnionFightCarAttackSource{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "guard_hp_source": "", "map_projection": "", "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated real-factory historical timer fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 4; id++ {
		for _, source := range []any{
			&db.P13UnionFightFactorySource{Day: match.Day, FactoryID: id, Enabled: true, ChariotHP: 1000, PublishedAt: clock.Load(), Source: "isolated actual120s car"},
			&db.P13UnionFightCarMotionSource{Day: match.Day, FactoryID: id, Enabled: true, Speed: 20, PublishedAt: clock.Load(), Source: "isolated local20mps"},
			&db.P13UnionFightCarAttackSource{Day: match.Day, FactoryID: id, Enabled: true, HPPercent: 4, Radius: 3.5, PublishedAt: clock.Load(), Source: "isolated local4percent"},
		} {
			if err := db.GetIns().Create(source).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	clock.Store(first)
	if err := f.s.TickP13UnionFightFactory(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 120000)
	if err := f.s.TickP13UnionFightFactory(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var factories []db.P13UnionFightFactoryState
	if err := db.GetIns().Where("match_id=?", match.ID).Order("guid").Find(&factories).Error; err != nil {
		t.Fatal(err)
	}
	if len(factories) != 4 {
		t.Fatal("real factory roster", len(factories))
	}
	// Original factories 1/2 start open; 3/4 require the real paid opening
	// action. Keep all four entities, but authenticate only actual produced cars.
	produced := make([]db.P13UnionFightFactoryState, 0, 2)
	for _, factory := range factories {
		if factory.FactoryID <= 2 {
			if factory.CompletedAt != first+120000 {
				t.Fatal("original open factory did not finish actual120s", factory.FactoryID, factory.CompletedAt)
			}
			produced = append(produced, factory)
		} else if factory.CompletedAt != 0 || factory.DueAt != 0 {
			t.Fatal("unopened paid field factory produced a car", factory.FactoryID)
		}
	}
	if len(produced) != 2 {
		t.Fatal("actual initially open cars", len(produced))
	}
	factories = produced
	var offers []db.P13UnionFightCarAttackOffer
	if err := db.GetIns().Where("match_id=?", match.ID).Find(&offers).Error; err != nil {
		t.Fatal(err)
	}
	byFactory := map[int64]db.P13UnionFightCarAttackOffer{}
	frozen := []p13UFMixedFrozenMeta{}
	for _, offer := range offers {
		byFactory[offer.FactoryGUID] = offer
		frozen = append(frozen, p13UFMixedFrozenMeta{"car-attack-offer", strconv.FormatInt(offer.FactoryGUID, 10), offer.SourceHash})
	}
	for i := range factories {
		car, err := p13UFCarItemRead(&factories[i])
		if err != nil {
			t.Fatal(err)
		}
		frozen = append(frozen, p13UFMixedFrozenMeta{"car", strconv.FormatInt(car.GetGuid(), 10), p13UFMixedCarSeal(&factories[i], nil)})
	}
	makeTimer := func(factory db.P13UnionFightFactoryState) db.P13UnionFightCarAttackState {
		car, err := p13UFCarItemRead(&factory)
		if err != nil {
			t.Fatal(err)
		}
		row := db.P13UnionFightCarAttackState{MatchID: match.ID, FactoryGUID: factory.GUID, CarGUID: car.GetGuid(), NextAttackAt: factory.CompletedAt, Completed: true, OfferHash: byFactory[factory.GUID].SourceHash}
		row.StateHash = p13UFCarAttackStateHash(&row)
		return row
	}
	wanted := makeTimer(factories[0])
	if err := db.GetIns().Create(&wanted).Error; err != nil {
		t.Fatal(err)
	}
	frozen = append(frozen, p13UFMixedFrozenMeta{"car-attack-state", strconv.FormatInt(wanted.FactoryGUID, 10), p13UFMixedCarAttackSeal(&wanted)})
	if err := p13UFMixedFrozenVerify(db.GetIns(), match.ID, frozen); err != nil {
		t.Fatal("original exact directory", err)
	}
	extra := makeTimer(factories[1])
	if err := db.GetIns().Create(&extra).Error; err != nil {
		t.Fatal(err)
	}
	// A new verifier instance has no remembered admission state or cached proof.
	if err := p13UFMixedFrozenVerify(db.GetIns().Session(&gorm.Session{NewDB: true}), match.ID, frozen); err != nil {
		t.Fatal("cold extra noncausal timer", err)
	}
	for _, mutation := range []string{"sha", "hits", "pending", "offer", "unfinished"} {
		changed := extra
		switch mutation {
		case "hits":
			changed.Hits = 1
		case "pending":
			changed.PendingReleaseAt = match.EndAt
		case "offer":
			changed.OfferHash = wanted.StateHash
		case "unfinished":
			changed.Completed = false
		}
		changed.StateHash = p13UFCarAttackStateHash(&changed)
		if mutation == "sha" {
			changed.StateHash = "bad"
		}
		if err := db.GetIns().Save(&changed).Error; err != nil {
			t.Fatal(err)
		}
		if err := p13UFMixedFrozenVerify(db.GetIns(), match.ID, frozen); err == nil {
			t.Fatal("accepted extra", mutation)
		}
	}
	if err := db.GetIns().Save(&extra).Error; err != nil {
		t.Fatal(err)
	}
	changed := wanted
	changed.Completed = false
	changed.StateHash = p13UFCarAttackStateHash(&changed)
	if err := db.GetIns().Save(&changed).Error; err != nil {
		t.Fatal(err)
	}
	if err := p13UFMixedFrozenVerify(db.GetIns(), match.ID, frozen); err == nil {
		t.Fatal("changed original wanted seal accepted")
	}
}

// Identity-only result rows test the writer fence, not a battle/reward proof.
func TestP13CarAttackWorldSealedCurrentReadAnyResult(t *testing.T) {
	f, _, _, _, match := p13UFFieldSetup(t)
	_ = f
	for _, kind := range []string{"win", "draw", constUtil.P13UFTechnicalAbortKind} {
		if err := db.GetIns().Where("match_id=?", match.ID).Delete(&db.P13UnionFightSettlementResult{}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			sealed, err := p13UFWorldSealed(tx, match.ID)
			if err != nil || sealed {
				t.Fatalf("unsealed scope: %v %v", sealed, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		row := db.P13UnionFightSettlementResult{MatchID: match.ID, ResultKind: kind, Awards: []byte("[]"), Source: "identity-only writer-fence fixture"}
		if err := db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			sealed, err := p13UFWorldSealed(tx, match.ID)
			if err != nil || !sealed {
				t.Fatalf("sealed %s: %v %v", kind, sealed, err)
			}
			abortOnly, err := p13UFWorldClosed(tx, match.ID)
			if err != nil || abortOnly != (kind == constUtil.P13UFTechnicalAbortKind) {
				t.Fatalf("abort-only cleanup changed %s", kind)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
