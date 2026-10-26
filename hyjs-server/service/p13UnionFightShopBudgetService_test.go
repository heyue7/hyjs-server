package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFShopBudgetLegacyOfferDoesNotReadFrames(t *testing.T) {
	f, _ := p13GuildSetup(t)
	scope := p6Digest([]byte(t.Name()))
	offer := db.P13UnionFightGateItemOffer{MatchID: scope, ItemID: "ufitem_020", HPPercent: 25, Enemy: true, Radius: 5, CooldownMillis: 5000, Source: "explicit legacy no-budget offer fixture"}
	offer.SourceHash = p13UFGateItemOfferHash(&offer)
	if err := db.GetIns().Create(&offer).Error; err != nil {
		t.Fatal(err)
	}
	match := &db.P13UnionFightMatch{ID: scope}
	actor := &db.P13UnionFightFieldPlayer{MatchID: scope, PlayerID: f.id, Camp: 0}
	// No field, roster or participant frame exists. An unrestricted legacy offer
	// must not invent a new dependency on current or departed inventories.
	remaining, enabled, err := p13UFShopBudgetRemaining(db.GetIns(), match, actor, "ufitem_020")
	if err != nil || enabled || remaining != 0 {
		t.Fatal(remaining, enabled, err)
	}
	if p13UFShopBudgetCheck(p13UFShopBudgetQuota{}, 100) != nil {
		t.Fatal("legacy purchase newly restricted")
	}
	orphan := db.P13UnionFightItemTeamUse{MatchID: scope, Camp: 0, ItemID: "ufitem_017", Limit: 4, Rule: constUtil.P13UFItemBudgetRule, OfferHash: p6Digest([]byte("missing offer"))}
	orphan.SourceHash = p13UFItemTeamUseHash(&orphan)
	if err = db.GetIns().Create(&orphan).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = p13UFShopBudgetRemaining(db.GetIns(), match, actor, "ufitem_017"); err == nil {
		t.Fatal("orphan budget counter became unrestricted")
	}
}

func TestP13UFShopBudgetRealMineTwoBuyersLastQuotaConsumeColdAndLeft(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t, 600000)
	a, b := ids[0], ids[11]
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, model := range []any{&db.P13UnionFightGateItemSource{}, &db.P13UnionFightCarItemSource{}, &db.P13UnionFightShopSource{}, &db.P13UnionFightResourceSource{}} {
		if err := db.GetIns().Where("1=1").Delete(model).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "guard_hp_source": "", "map_projection": "", "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "explicit real mine/shop shared-budget fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightGateItemSource{Day: match.Day, ItemID: "ufitem_020", Enabled: true, HPPercent: 25, BudgetRule: constUtil.P13UFItemBudgetRule, TeamUseLimit: 4, TargetCooldownMillis: 10000, EffectFamily: constUtil.P13UFItemBudgetHammer, PublishedAt: clock.Load(), Source: "isolated hammer25percent four team uses fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: "ufitem_020", ItemID: "ufitem_020", Enabled: true, Price: 10, Quantity: 1, BuyCount: 5, RefreshMillis: 60000, PublishedAt: clock.Load(), Source: "isolated token10 real shop quote fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	guild := p13GuildID(t, f, a, now())
	camp := int64(0)
	recycler, shop := "2", "3"
	if guild == match.RightGuildID {
		camp = 1
		recycler, shop = "5", "6"
	}
	recyclePoint, err := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a, b} {
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("budget-mine-%d", id), Enabled: true, ResourceID: "1", X: recyclePoint.GetX(), Y: recyclePoint.GetY(), Z: recyclePoint.GetZ(), RewardRes: 50, PublishedAt: clock.Load(), Source: "isolated actual50 mineral deposit source"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{a, b} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id int64) (db.P13UnionFightFieldPlayer, *pb.InstUFPlayer) {
		t.Helper()
		var actor db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			t.Fatal(err)
		}
		return actor, frame
	}
	move := func(id int64, point *pb.PointNew) {
		t.Helper()
		if _, err := f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
			t.Fatal(err)
		}
	}
	shopPoint, err := fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
	if err != nil {
		t.Fatal(err)
	}
	for index, id := range []int64{a, b} {
		clock.Store(first + int64(index)*60000)
		move(id, recyclePoint)
		if _, err = f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("budget-mine-%d", id)))}, now()); err != nil {
			t.Fatal(err)
		}
		clock.Add(3000)
		if err = f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, 100+int64(index), now()); err != nil {
			t.Fatal(err)
		}
		clock.Add(10000)
		move(id, shopPoint)
	}
	actorA, _ := read(a)
	actorB, _ := read(b)
	if actorA.Camp != camp || actorB.Camp != camp {
		t.Fatal("fixture not same real camp")
	}
	quotesA := p13UFTestShopQuotes(t, f, a, shop, now())
	quotesB := p13UFTestShopQuotes(t, f, b, shop, now())
	reqA := &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(p13UFTestShopQuote(t, quotesA, "ufitem_020")), NpcId: proto.String(shop)}
	reqB := &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(p13UFTestShopQuote(t, quotesB, "ufitem_020")), NpcId: proto.String(shop)}
	quota := func(actor db.P13UnionFightFieldPlayer) p13UFShopBudgetQuota {
		t.Helper()
		var q p13UFShopBudgetQuota
		err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			all, err := p13UFShopBudgetBatch(tx, match, &actor, []string{"ufitem_017", "ufitem_018", "ufitem_019", "ufitem_020"})
			if err == nil {
				q = all["ufitem_020"]
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	if q := quota(actorA); !q.Enabled || q.Remaining != 4 || q.Unspent != 0 || q.Used != 0 {
		t.Fatal(q)
	}
	for i := int64(0); i < 3; i++ {
		if _, err = f.s.p13UFShopBuy(f.ctx, a, reqA, 200+i, now()); err != nil {
			t.Fatal(err)
		}
	}
	if q := quota(actorA); q.Remaining != 1 || q.Unspent != 3 {
		t.Fatal(q)
	}
	_, beforeA := read(a)
	_, beforeB := read(b)
	type buyResult struct {
		id, stamp int64
		err       error
	}
	results := make(chan buyResult, 2)
	buy := func(id, stamp int64, request *pb.CSUFPlayerBuyItem) {
		_, err := f.s.p13UFShopBuy(f.ctx, id, request, stamp, now())
		results <- buyResult{id, stamp, err}
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); buy(a, 300, reqA) }()
		go func() { defer wg.Done(); buy(b, 301, reqB) }()
		wg.Wait()
	} else {
		buy(a, 300, reqA)
		buy(b, 301, reqB)
	}
	var winner buyResult
	success, failed := 0, 0
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			winner = r
			success++
		} else {
			failed++
		}
	}
	if success != 1 || failed != 1 {
		t.Fatal("last team quota must have one purchaser", success, failed)
	}
	if q := quota(actorA); q.Remaining != 0 || q.Unspent != 4 || q.Used != 0 {
		t.Fatal(q)
	}
	loser := a
	loserBefore := beforeA
	winnerReq := reqB
	if winner.id == a {
		loser = b
		loserBefore = beforeB
		winnerReq = reqA
	}
	_, loserAfter := read(loser)
	if !proto.Equal(loserBefore, loserAfter) {
		t.Fatal("failed buyer lost wallet or inventory")
	}
	_, winnerBeforeRetry := read(winner.id)
	if _, err = f.s.p13UFShopBuy(f.ctx, winner.id, winnerReq, winner.stamp, now()); err != nil {
		t.Fatal(err)
	}
	_, winnerAfterRetry := read(winner.id)
	if !proto.Equal(winnerBeforeRetry, winnerAfterRetry) || quota(actorA).Unspent != 4 {
		t.Fatal("positive receipt reserved twice")
	}
	for index, id := range []int64{a, b} {
		_, before := read(id)
		req := reqA
		if id == b {
			req = reqB
		}
		if _, err = f.s.p13UFShopBuy(f.ctx, id, req, 400+int64(index), now()); err == nil {
			t.Fatal("exhausted team sold another item")
		}
		_, after := read(id)
		if !proto.Equal(before, after) {
			t.Fatal("exhausted purchase changed actual currency/bag")
		}
	}
	// Real hammer consumption moves one unit from unspent to used in the same TX.
	if err = db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var target db.P13UnionFightGateState
	if err = db.GetIns().Where("match_id=? AND camp<>?", match.ID, camp).Order("gate_id").First(&target).Error; err != nil {
		t.Fatal(err)
	}
	gatePoint, err := fieldPoint(original.table("UnionFightGate", "UFGId", fmt.Sprint(target.GateID))["Position"])
	if err != nil {
		t.Fatal(err)
	}
	clock.Add(80000)
	move(a, gatePoint)
	if _, err = f.s.p13UFItemUse(f.ctx, a, &pb.CSUFPlayerUseItem{ItemId: proto.String("ufitem_020")}, 500, now()); err != nil {
		t.Fatal(err)
	}
	if q := quota(actorB); q.Remaining != 0 || q.Unspent != 3 || q.Used != 1 {
		t.Fatal("use freed a spent team quota", q)
	}
	if _, _, err = f.s.p13UFFieldLeaveExpected(f.ctx, a, &pb.CSTransferScene{Target: proto.String("town_main")}, now(), match.ID); err != nil {
		t.Fatal(err)
	}
	if q := quota(actorB); q.Unspent != 3 || q.Used != 1 || q.Remaining != 0 {
		t.Fatal("departed bag reservation disappeared", q)
	}
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if q := quota(actorB); q.Unspent != 3 || q.Used != 1 || q.Remaining != 0 {
		t.Fatal("cold login lost camp reservations", q)
	}
	// A departed frame remains part of the reservation source and must retain SHA.
	retired, _ := read(a)
	originalFrameHash := retired.FrameHash
	if err = db.GetIns().Model(&retired).Update("frame_hash", "damaged").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = p13UFShopBudgetRemaining(db.GetIns(), match, &actorB, "ufitem_020"); err == nil {
		t.Fatal("damaged departed stock silently ignored")
	}
	if err = db.GetIns().Model(&retired).Update("frame_hash", originalFrameHash).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&retired).Update("camp", 1-camp).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = p13UFShopBudgetRemaining(db.GetIns(), match, &actorB, "ufitem_020"); err == nil {
		t.Fatal("departed roster inventory hidden by damaged camp")
	}
	if err = db.GetIns().Model(&retired).Update("camp", camp).Error; err != nil {
		t.Fatal(err)
	}
	var used db.P13UnionFightItemTeamUse
	if err = db.GetIns().First(&used, "match_id=? AND camp=? AND item_id=?", match.ID, camp, "ufitem_020").Error; err != nil {
		t.Fatal(err)
	}
	originalHash := used.SourceHash
	if err = db.GetIns().Model(&db.P13UnionFightItemTeamUse{}).Where("match_id=? AND camp=? AND item_id=?", match.ID, camp, "ufitem_020").Update("source_hash", "damaged").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = p13UFShopBudgetRemaining(db.GetIns(), match, &actorB, "ufitem_020"); err == nil {
		t.Fatal("damaged team use source silently ignored")
	}
	if err = db.GetIns().Model(&db.P13UnionFightItemTeamUse{}).Where("match_id=? AND camp=? AND item_id=?", match.ID, camp, "ufitem_020").Update("source_hash", originalHash).Error; err != nil {
		t.Fatal(err)
	}
	if q := quota(actorB); q.Used+q.Unspent != 4 {
		t.Fatal(q)
	}
}
