package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// TestP13UnionFightShop 使用真实采矿/上交来源消费OwnToken，并验证原5格/角色/逐次恢复/Cold/报价不可漂移。
func TestP13UnionFightShopActualMiningTokenSourceOfferStockRoleFiveSlotsAndCold(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "Shop真实来源消费隔离；明确本地出生HP"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for camp, npc := range map[int]string{0: "2", 1: "5"} {
		point, err := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("shop-mining-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "隔离具名本地矿型1/位置原回收NPC2或5/单次50，不声称原产量"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, price := range []int64{10, 10, 20, 20, 30} {
		role := int64(0)
		if i == 2 || i == 3 {
			role = 1
		}
		if i == 4 {
			role = 2
		}
		if err = db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: fmt.Sprintf("catalog-%d", i), Enabled: true, ItemID: fmt.Sprintf("ufitem_%03d", 16+i), Price: price, Quantity: 1, BuyCount: 2, BuyCondition: role, Priority: int64(i), RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "明确本地隔离报价10/10/20/20/30、数量1、次数2、逐60s恢复1；原UF16..20/角色规则，不声称原价"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var camp int64
	for _, id := range []int64{ids[0], ids[10], ids[11]} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id int64) *pb.InstUFPlayer {
		t.Helper()
		var actor db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		camp = actor.Camp
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	read(ids[0])
	recycler := "2"
	shop := "3"
	if camp == 1 {
		recycler = "5"
		shop = "6"
	}
	recyclePoint, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
	shopPoint, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
	guid := p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("shop-mining-%d", camp))
	move := func(id int64, point *pb.PointNew) {
		t.Helper()
		if _, err := f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
			t.Fatal(err)
		}
	}
	mine := func(id, stamp int64) {
		t.Helper()
		move(id, recyclePoint)
		if _, err := f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(guid)}, now()); err != nil {
			t.Fatal(err)
		}
		clock.Add(3000)
		if err := f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, stamp, now()); err != nil {
			t.Fatal(err)
		}
		if read(id).GetOwnToken() != 150 {
			t.Fatal("real50 mine to150 war currency")
		}
	}
	list := func(id int64, npc string) *pb.SCUFShopList {
		t.Helper()
		replies, err := f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(npc)}, now())
		if err != nil {
			t.Fatal(err)
		}
		return replies[0].(*pb.SCUFShopList)
	}
	buy := func(id int64, index int, stamp int64) *pb.SCUFPlayerBuyItemDetail {
		t.Helper()
		replies, err := f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(fmt.Sprintf("catalog-%d", index)), NpcId: proto.String(shop)}, stamp, now())
		if err != nil {
			t.Fatal(err)
		}
		return replies[0].(*pb.SCUFPlayerBuyItemDetail)
	}
	if _, err = f.s.p13UFShopList(f.ctx, ids[0], &pb.CSUFShopList{}, now()); err == nil {
		t.Fatal("empty NPC without authorized visit")
	}
	clock.Store(first + 20000)
	mine(ids[0], 500)
	clock.Add(10000)
	move(ids[0], shopPoint)
	catalog := list(ids[0], shop)
	if len(catalog.Items) != 5 {
		t.Fatal("all original normal5 published")
	}
	if catalog.Items[0].GetLeftBuyCount() != 2 || catalog.Items[0].GetRefreshIntervalTime() != 60000 {
		t.Fatal("original stock/time projection")
	}
	firstBuy := buy(ids[0], 0, 800)
	if firstBuy.Player.GetOwnToken() != 140 || len(firstBuy.Player.Items) != 1 || firstBuy.Player.Items[0].GetLeftUseNum() != 1 {
		t.Fatal("war token not consumed into actual original item")
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.s.p13UFShopBuy(f.ctx, ids[0], &pb.CSUFPlayerBuyItem{ShopItemId: proto.String("catalog-0"), NpcId: proto.String(shop)}, 800, now())
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if !proto.Equal(firstBuy, buy(ids[0], 0, 800)) {
		t.Fatal("exact persistent purchase receipt")
	}
	buy(ids[0], 0, 801)
	if list(ids[0], "").Items[0].GetLeftBuyCount() != 0 {
		t.Fatal("empty NPC refresh not bound to actual last shop")
	}
	if _, err = f.s.p13UFShopBuy(f.ctx, ids[0], &pb.CSUFPlayerBuyItem{ShopItemId: proto.String("catalog-0"), NpcId: proto.String(shop)}, 802, now()); err == nil {
		t.Fatal("empty purchase stock")
	}
	for i := 0; i < 3; i++ {
		clock.Add(60000)
		if list(ids[0], "").Items[0].GetLeftBuyCount() != 1 {
			t.Fatal("Countdown restored whole stock instead of one")
		}
		buy(ids[0], 0, 802+int64(i))
	}
	clock.Add(60000)
	if _, err = f.s.p13UFShopBuy(f.ctx, ids[0], &pb.CSUFPlayerBuyItem{ShopItemId: proto.String("catalog-0"), NpcId: proto.String(shop)}, 805, now()); err == nil {
		t.Fatal("original carried cap5 overflow")
	}
	if read(ids[0]).GetOwnToken() != 100 || read(ids[0]).Items[0].GetLeftUseNum() != 5 {
		t.Fatal("rejected item overflow consumed war currency")
	}
	// 两个真实不同职位也从同点顺序取得50矿和150战争贡献，再验证原购买权限。
	mine(ids[10], 501)
	clock.Add(10000)
	move(ids[10], shopPoint)
	list(ids[10], shop)
	if f.mysql.Database != "" {
		// 八个不同真实请求争两个个人库存，行锁保证只有两次货币消费和道具增加。
		var wg sync.WaitGroup
		accepted := make(chan bool, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(stamp int64) {
				defer wg.Done()
				_, err := f.s.p13UFShopBuy(f.ctx, ids[10], &pb.CSUFPlayerBuyItem{ShopItemId: proto.String("catalog-0"), NpcId: proto.String(shop)}, stamp, now())
				accepted <- err == nil
			}(1000 + int64(i))
		}
		wg.Wait()
		close(accepted)
		count := 0
		for ok := range accepted {
			if ok {
				count++
			}
		}
		member := read(ids[10])
		if count != 2 || member.GetOwnToken() != 130 || len(member.Items) != 1 || member.Items[0].GetLeftUseNum() != 2 {
			t.Fatal("different requests overspent original personal stock/token", count, member)
		}
	}
	if _, err = f.s.p13UFShopBuy(f.ctx, ids[10], &pb.CSUFPlayerBuyItem{ShopItemId: proto.String("catalog-2"), NpcId: proto.String(shop)}, 900, now()); err == nil {
		t.Fatal("ordinary bought officer item")
	}
	mine(ids[11], 502)
	clock.Add(10000)
	move(ids[11], shopPoint)
	list(ids[11], shop)
	buy(ids[11], 2, 901)
	if _, err = f.s.p13UFShopBuy(f.ctx, ids[11], &pb.CSUFPlayerBuyItem{ShopItemId: proto.String("catalog-4"), NpcId: proto.String(shop)}, 902, now()); err == nil {
		t.Fatal("vice bought chairman item")
	}
	p13GuildCall(t, f, ids[0], &pb.CSTransferChairman{PlayerId: proto.Int64(ids[10])}, 710, now())
	if _, err = f.s.p13UFShopBuy(f.ctx, ids[0], &pb.CSUFPlayerBuyItem{ShopItemId: proto.String("catalog-4"), NpcId: proto.String(shop)}, 903, now()); err == nil {
		t.Fatal("old chairman retained item permission")
	}
	p13GuildCall(t, f, ids[10], &pb.CSTransferChairman{PlayerId: proto.Int64(ids[0])}, 711, now())
	for i := 1; i < 5; i++ {
		buy(ids[0], i, 910+int64(i))
	}
	final := read(ids[0])
	if len(final.Items) != 5 || final.GetOwnToken() != 20 {
		t.Fatal("original all5 slots/source currency", final)
	}
	for index, item := range final.Items {
		if item.GetIndex() != int32(index) || item.GetPlayerGuid() != ids[0] {
			t.Fatal("original index out of bounds")
		}
	}
	// 改发布价不改冻结报价；冷库和空NPC刷新继续原余量/物品/WarToken。
	db.GetIns().Model(&db.P13UnionFightShopSource{}).Where("day=?", match.Day).Update("price", 999)
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	restored := list(ids[0], "")
	if restored.Items[0].GetPrice() != 10 || read(ids[0]).GetOwnToken() != 20 || len(read(ids[0]).Items) != 5 {
		t.Fatal("cold source/price/currency drift")
	}
	// 实际退出立即失去访问；原个人道具仅此比赛存档，不注入普通背包。
	if _, err = f.s.p13UFFieldLeave(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_main")}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFShopList(f.ctx, ids[0], &pb.CSUFShopList{}, now()); err == nil {
		t.Fatal("left actor could use old store")
	}
}
