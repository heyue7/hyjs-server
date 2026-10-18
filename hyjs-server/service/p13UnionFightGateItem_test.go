package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP13UnionFightGateItemActual 两个实际阵营从采矿店购到攻门/修门，死亡城门由真实道具产生，无固定战争winner。
func TestP13UnionFightGateItemActualTwoCampMiningShopNearestRepairAttackCrashColdRaceSHA(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	a := ids[0]
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, table := range []any{&db.P13UnionFightGateItemSource{}, &db.P13UnionFightGateItemOffer{}, &db.P13UnionFightGateState{}, &db.P13UnionFightItemSource{}, &db.P13UnionFightItemOffer{}, &db.P13UnionFightItemState{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "原双阵营门物品隔离，本地initialHP10000"}).Error; err != nil {
		t.Fatal(err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"ufitem_019", "ufitem_020"} {
		percent := int64(50)
		if item == "ufitem_019" {
			percent = 25
		}
		if err = db.GetIns().Create(&db.P13UnionFightGateItemSource{Day: match.Day, ItemID: item, Enabled: true, HPPercent: percent, PublishedAt: now().UnixMilli(), Source: fmt.Sprintf("隔离明确本地门效果%d%%，非原数额", percent)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetIns().Create(&db.P13UnionFightItemSource{Day: match.Day, ItemID: "ufitem_016", Enabled: true, HPPercent: 20, PublishedAt: now().UnixMilli(), Source: "本地20%普通瓶，共CD验证"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"ufitem_019", "ufitem_020", "ufitem_016"} {
		if err = db.GetIns().Create(&db.P13UnionFightShopSource{Day: match.Day, ShopItemID: item, ItemID: item, Enabled: true, Price: 10, Quantity: 1, BuyCount: 3, RefreshMillis: 60000, PublishedAt: now().UnixMilli(), Source: "隔离当地10战争贡献报价3次，真实矿来源"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for camp, npc := range map[int]string{0: "2", 1: "5"} {
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", npc)["Position"])
		if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: fmt.Sprintf("gate-mine-%d", camp), Enabled: true, ResourceID: "1", X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 50, PublishedAt: now().UnixMilli(), Source: "隔离原矿1/回收NPC位置/当地50矿"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ownerGuild := p13GuildID(t, f, a, now())
	enemy := match.RightGuildID
	if enemy == ownerGuild {
		enemy = match.LeftGuildID
	}
	var member db.P13GuildMember
	if err = db.GetIns().First(&member, "guild_id=?", enemy).Error; err != nil {
		t.Fatal(err)
	}
	b := member.PlayerID
	for _, id := range []int64{a, b} {
		if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id int64) *pb.InstUFPlayer {
		t.Helper()
		var row db.P13UnionFightFieldPlayer
		if err := db.GetIns().First(&row, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
			t.Fatal(err)
		}
		frame, err := p13UFFieldReadActor(&row)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	move := func(id int64, point *pb.PointNew) {
		t.Helper()
		if _, err := f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now()); err != nil {
			t.Fatal(err)
		}
	}
	req := func(item string) *pb.CSUFPlayerUseItem { return &pb.CSUFPlayerUseItem{ItemId: proto.String(item)} }
	// 两阵营各实际50矿→战争贡献150；道具全部原店购，无人工库存或受损门初始化。
	for _, id := range []int64{a, b} {
		camp := read(id).GetCamp()
		recycler, shop := "2", "3"
		if camp == 1 {
			recycler, shop = "5", "6"
		}
		point, _ := fieldPoint(original.table("UnionFightNpc", "UFNId", recycler)["Position"])
		clock.Add(80000)
		move(id, point)
		guid := p13UFFieldGUID(match.ID, "resource", fmt.Sprintf("gate-mine-%d", camp))
		if _, err = f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(guid)}, now()); err != nil {
			t.Fatal(err)
		}
		clock.Add(3000)
		if err = f.s.TickP13UnionFightResource(f.ctx, now()); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String(recycler)}, 100, now()); err != nil {
			t.Fatal(err)
		}
		if read(id).GetOwnToken() != 150 {
			t.Fatal("actual50 mineral to150 contribution")
		}
		clock.Add(10000)
		point, _ = fieldPoint(original.table("UnionFightNpc", "UFNId", shop)["Position"])
		move(id, point)
		if _, err = f.s.p13UFShopList(f.ctx, id, &pb.CSUFShopList{NpcId: proto.String(shop)}, now()); err != nil {
			t.Fatal(err)
		}
		items := []string{"ufitem_019", "ufitem_020", "ufitem_016"}
		if id == b {
			items = []string{"ufitem_019"}
		}
		for i, item := range items {
			count := 3
			if item == "ufitem_016" {
				count = 1
			}
			for n := 0; n < count; n++ {
				if _, err = f.s.p13UFShopBuy(f.ctx, id, &pb.CSUFPlayerBuyItem{ShopItemId: proto.String(item), NpcId: proto.String(shop)}, 200+int64(i*3+n), now()); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if read(a).GetOwnToken() != 80 || read(b).GetOwnToken() != 120 {
		t.Fatal("actual inventory currencies")
	}
	// 未到门边不能消费；独立门禁默认0也拒绝当前范围正确的请求。
	clock.Store(first + 140000)
	var gateRows []db.P13UnionFightGateState
	if err = db.GetIns().Where("match_id=?", match.ID).Order("gate_id").Find(&gateRows).Error; err != nil || len(gateRows) != 8 {
		t.Fatal("original8 dynamic gates", err)
	}
	var target db.P13UnionFightGateState
	for _, row := range gateRows {
		if row.Camp == int64(read(b).GetCamp()) {
			target = row
			break
		}
	}
	point, _ := fieldPoint(original.table("UnionFightGate", "UFGId", fmt.Sprint(target.GateID))["Position"])
	move(a, point)
	move(b, point)
	if _, err = f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), 1000, now()); err == nil {
		t.Fatal("closed actual gate items")
	}
	db.GetIns().Model(&db.P13UnionFightItemPolicy{}).Where("id=1").Update("enabled", true)
	if _, err = f.s.p13UFItemUse(f.ctx, a, req("ufitem_019"), 1001, now()); err == nil {
		t.Fatal("repair targeted enemy gate")
	}
	if _, err = f.s.p13UFItemUse(f.ctx, b, req("ufitem_020"), 1002, now()); err == nil {
		t.Fatal("unowned enemy hammer")
	}
	if _, err = f.s.p13UFItemUse(f.ctx, a, req("ufitem_016"), 1003, now()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), 1000, now()); err == nil {
		t.Fatal("gate item bypassed same normal potion CD")
	}
	clock.Add(5000)
	hit, err := f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), 1000, now())
	if err != nil {
		t.Fatal(err)
	}
	if len(hit) != 3 || hit[2].(*pb.SCUFGate).Gate[0].GetHp() != 5000 {
		t.Fatal("actual first half gate HP", hit)
	}
	if hit[1].(*pb.SCUFElementTickExport).HpDelta[0] != -5000 || hit[1].(*pb.SCUFElementTickExport).TargetGuid[0] != target.GUID {
		t.Fatal("original actual target HP tick")
	}
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), 1000, now())
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
	fixed, err := f.s.p13UFItemUse(f.ctx, b, req("ufitem_019"), 1000, now())
	if err != nil || fixed[2].(*pb.SCUFGate).Gate[0].GetHp() != 7500 {
		t.Fatal("actual own repair of enemy hammer damage", err)
	}
	db.GetIns().Model(&db.P13UnionFightGateItemSource{}).Where("day=?", match.Day).Updates(map[string]any{"enabled": false, "hp_percent": 1})
	clock.Add(5000)
	hit, err = f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), 1004, now())
	if err != nil || hit[2].(*pb.SCUFGate).Gate[0].GetHp() != 2500 {
		t.Fatal("frozen50 damage drifted to current1", err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFFieldFrames(f.ctx, b, &pb.CSUFInit{}, now())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, gate := range frames[0].(*pb.SCUnionFightTransfer).GateInfos {
		if gate.GetGuid() == target.GUID {
			found = true
			if gate.GetHp() != 2500 {
				t.Fatal("cold gate HP reset")
			}
		}
	}
	if !found {
		t.Fatal("cold gate omitted")
	}
	clock.Add(5000)
	if f.mysql.Database != "" {
		var wg sync.WaitGroup
		wins := make(chan []proto.Message, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(stamp int64) {
				defer wg.Done()
				replies, err := f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), stamp, now())
				if err == nil {
					wins <- replies
				}
			}(1100 + int64(i))
		}
		wg.Wait()
		close(wins)
		count := 0
		for replies := range wins {
			count++
			hit = replies
		}
		if count != 1 {
			t.Fatal("eight distinct last-hammer actions", count)
		}
	} else {
		hit, err = f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), 1100, now())
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(hit) != 4 || hit[2].(*pb.SCUFGate).Gate[0].GetHp() != 0 || hit[3].(*pb.SCUFGateCrash).GetGateId() != target.GateID {
		t.Fatal("original gate crash rawID/actual zero HP", hit)
	}
	if hit[1].(*pb.SCUFElementTickExport).HpDelta[0] != -2500 {
		t.Fatal("clamped actual zeroHP tick was overkill5000")
	}
	if _, err = f.s.p13UFItemUse(f.ctx, b, req("ufitem_019"), 1200, now()); err == nil {
		t.Fatal("repair revived destroyed gate without original rule")
	}
	dynamic, err := f.s.p13UFFieldElements(f.ctx, b, []int64{target.GUID}, now())
	if err != nil {
		t.Fatal(err)
	}
	if dynamic[0].(*pb.SCUFGate).Gate[0].GetHp() != 0 {
		t.Fatal("element still initial gate HP")
	}
	// 冻结原8门只有这个实际目标变动，其余7保持原10000；损坏拒绝不消耗真实库存。
	if err = db.GetIns().Where("match_id=?", match.ID).Find(&gateRows).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range gateRows {
		frame, err := p13UFGateRead(&row)
		if err != nil {
			t.Fatal(err)
		}
		if row.GUID != target.GUID && frame.GetHp() != 10000 {
			t.Fatal("cross gate damage")
		}
	}
	prior := gateRows[0]
	db.GetIns().Model(&db.P13UnionFightGateState{}).Where("match_id=? AND guid=?", match.ID, prior.GUID).Update("frame_hash", "bad")
	if _, err = f.s.p13UFFieldElements(f.ctx, b, []int64{target.GUID}, now()); err == nil {
		t.Fatal("dynamic gate badSHA displayed")
	}
	db.GetIns().Model(&db.P13UnionFightGateState{}).Where("match_id=? AND guid=?", match.ID, prior.GUID).Update("frame_hash", prior.FrameHash)
	clock.Store(match.EndAt)
	if _, err = f.s.p13UFItemUse(f.ctx, a, req("ufitem_020"), 1000, now()); err == nil {
		t.Fatal("old gate receipt at End kept access")
	}
}
