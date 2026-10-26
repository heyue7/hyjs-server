package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestP13CarImpactPureStockIdentityAndMovingFlags(t *testing.T) {
	before := &pb.InstUFCariot{Guid: proto.Int64(11), FactoryGuid: proto.Int64(22), CariotId: proto.Int32(1), Camp: proto.Int32(1), Hp: proto.Float64(1000), MaxHp: proto.Float64(1000), IsSlowDown: proto.Int32(0)}
	after := proto.Clone(before).(*pb.InstUFCariot)
	after.Hp = proto.Float64(800)
	row := &db.P13UnionFightCarImpact{MatchID: strings.Repeat("a", 64), CarGUID: 11, Ordinal: 1, FactoryGUID: 22, FactoryID: 1, Generation: 1, BornAt: 1, CarCamp: 1, FactorySourceHash: strings.Repeat("b", 64), GenerationSourceHash: strings.Repeat("b", 64), PlayerID: 900, Camp: 0, ItemID: "ufitem_017", Stamp: 0, OfferHash: strings.Repeat("c", 64), HPPercent: 20, InventoryBefore: 5, InventoryAfter: 4, LogicalAt: 1000, AppliedAt: 1000, HPBefore: 1000, HPAfter: 800, IdentityHash: p13UFCarHPIdentity(before)}
	row.ID = p6Digest([]byte(row.MatchID + "/11/1/car-impact"))
	row.Before, _ = proto.Marshal(before)
	row.After, _ = proto.Marshal(after)
	row.BeforeHash = p6Digest(row.Before)
	row.AfterHash = p6Digest(row.After)
	row.SourceHash = p13UFCarImpactHash(row)
	if _, _, err := p13UFCarImpactVerify(row); err != nil {
		t.Fatal("stamp0 immutable effect", err)
	}
	moving := proto.Clone(after).(*pb.InstUFCariot)
	moving.IsSlowDown = proto.Int32(1)
	if p13UFCarHPIdentity(moving) != row.IdentityHash {
		t.Fatal("movement polluted HP chain identity")
	}
	moving.Guid = proto.Int64(33)
	if p13UFCarHPIdentity(moving) == row.IdentityHash {
		t.Fatal("generation GUID was not bound")
	}
	after.IsSlowDown = proto.Int32(1)
	row.After, _ = proto.Marshal(after)
	row.AfterHash = p6Digest(row.After)
	row.SourceHash = p13UFCarImpactHash(row)
	if _, _, err := p13UFCarImpactVerify(row); err == nil {
		t.Fatal("bomb altered movement inside its own HP effect")
	}
}

func TestP13CarImpactRealTimerStampZeroAndMovementLedger(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	for _, table := range []any{&db.P13UnionFightFactorySource{}, &db.P13UnionFightCarItemSource{}, &db.P13UnionFightCarImpact{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "guard_hp_source": "", "map_projection": "", "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "explicit car impact source fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 4; id++ {
		if err := db.GetIns().Create(&db.P13UnionFightFactorySource{Day: match.Day, FactoryID: id, Enabled: true, ChariotHP: 1000, PublishedAt: clock.Load(), Source: "explicit real-timer car HP1000 fixture"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Create(&db.P13UnionFightCarItemSource{Day: match.Day, ItemID: "ufitem_017", Enabled: true, HPPercent: 20, PublishedAt: clock.Load(), Source: "explicit isolated bomb20percent"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	clock.Store(first + 120000)
	if err := f.s.TickP13UnionFightFactory(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var actor db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	var car db.P13UnionFightFactoryState
	if err := db.GetIns().Where("match_id=? AND camp<>? AND completed_at>0", match.ID, actor.Camp).Order("guid").First(&car).Error; err != nil {
		t.Fatal("real enemy first-car timer", err)
	}
	var quote db.P13UnionFightCarItemOffer
	if err := db.GetIns().First(&quote, "match_id=? AND item_id=?", match.ID, "ufitem_017").Error; err != nil {
		t.Fatal(err)
	}
	// The existing full item suite retains actual mine/shop/stock consumption.
	// This lower-level fixture supplies stock headers solely to test irreversible
	// stamp0 append and legal movement between two real produced-car effects.
	for i := int64(0); i < 2; i++ {
		before, err := p13UFCarItemRead(&car)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			before.IsSlowDown = proto.Int32(1)
			car.CarFrame, _ = proto.Marshal(before)
			car.CarHash = p6Digest(car.CarFrame)
			if err := p13UFCarSave(db.GetIns(), &car); err != nil {
				t.Fatal(err)
			}
		}
		after := proto.Clone(before).(*pb.InstUFCariot)
		after.Hp = proto.Float64(before.GetHp() - 200)
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			if err := p13UFCarImpactRecord(tx, &car, after, db.P13UnionFightCarImpact{PlayerID: ids[0], Camp: actor.Camp, ItemID: quote.ItemID, OfferHash: quote.SourceHash, HPPercent: 20, InventoryBefore: 5 - i, InventoryAfter: 4 - i, LogicalAt: clock.Load() + i, AppliedAt: clock.Load() + i}); err != nil {
				return err
			}
			car.CarFrame, _ = proto.Marshal(after)
			car.CarHash = p6Digest(car.CarFrame)
			return p13UFCarSave(tx, &car)
		}); err != nil {
			t.Fatal(err)
		}
	}
	ledger, err := p13UFCarImpactLedger(db.GetIns(), match)
	if err != nil || len(ledger.Impacts) != 2 {
		t.Fatal("car ledger falsely rejected moving PB", err)
	}
	if ledger.Impacts[0].Stamp != 0 || ledger.Impacts[1].Stamp != 0 || ledger.Impacts[0].ID == ledger.Impacts[1].ID || ledger.Impacts[0].Ordinal != 1 || ledger.Impacts[1].Ordinal != 2 || ledger.Impacts[0].AfterHash == ledger.Impacts[1].BeforeHash {
		t.Fatal("stamp0 journal or independent movement evidence missing")
	}
}
