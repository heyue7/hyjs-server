package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This closes an ordinary source chain, not another maximum-equipment matrix.
// Only level18 and historical original chapter1..8 stars are qualifications;
// currencies, boxes, equipment GUIDs, ore, gems and rewards are not injected.
func TestP5GrowthEquipmentOriginal400DiamondBoxToActualGemPropertyAndCold(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitGems, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		if p.GetCoin() != 0 || p.GetDiamond() != 0 {
			t.Fatal("source account must start with no currencies")
		}
		p.Level = proto.Int32(18)
		g.Copy = &pb.SCCopyPassedInfo{}
		for _, mission := range f.s.Tables["CopyMission"] {
			chapter, err := strconv.Atoi(mission["ChapterId"])
			if err == nil && chapter >= 1 && chapter <= 8 {
				g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: proto.String(mission["ChapterId"]), MissionId: proto.String(mission["MissionId"]), Star: proto.Int32(5)})
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for chapter := 1; chapter <= 8; chapter++ {
		for index := int32(0); index < 3; index++ {
			if _, err := f.s.chapterStarReward(f.ctx, id, &pb.CSCopyGetStarAward{ChapterId: proto.String(strconv.Itoa(chapter)), StarIndex: proto.Int32(index)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	p, _ := f.profile(t)
	beforeDiamond := p.GetDiamond()
	if p.GetCoin() < 1010000 || beforeDiamond < 400 {
		t.Fatal("actual original star rewards insufficient")
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "OpenHappy", "91400000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	purchase := &pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}
	result, err := f.s.openServerActivity(f.ctx, id, purchase, 9301)
	if err != nil {
		t.Fatal("original day1 400diamond blue equipment source", err)
	}
	replay, err := f.s.openServerActivity(f.ctx, id, purchase, 9301)
	if err != nil || len(result) != len(replay) {
		t.Fatal("actual source receipt", err)
	}
	for i := range result {
		if !proto.Equal(result[i], replay[i]) {
			t.Fatal("source replay changed")
		}
	}
	p, g := f.profile(t)
	if p.GetDiamond() != beforeDiamond-400 || p5LifeItemCount(g, "item_055") != 1 {
		t.Fatal("original price/box inventory")
	}
	opened, err := f.s.p5BoxUse(f.ctx, id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_055"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(0)}, 9302)
	if err != nil {
		t.Fatal("original fixed four-equipment box", err)
	}
	box := opened[0].(*pb.SCItemUse)
	if len(box.GetGet().GetEquips()) != 4 {
		t.Fatal("original four ordinary equipment bodies")
	}
	gearByColour := map[int32]*pb.Equipment{}
	seen := map[string]bool{}
	for _, gear := range box.GetGet().GetEquips() {
		if gear.GetGuid() == "" || seen[gear.GetGuid()] {
			t.Fatal("actual unique equipment GUID")
		}
		seen[gear.GetGuid()] = true
		switch gear.GetEquipId() {
		case "equip_005", "equip_006", "equip_007", "equip_008":
		default:
			t.Fatal("original item055 changed equipment", gear.GetEquipId())
		}
		holes, e := f.s.gemHoles(gear)
		if e != nil {
			t.Fatal(e)
		}
		if len(holes) != 4 || holes[0] < 1 || holes[0] > 4 {
			t.Fatal("original ordinary coloured hole")
		}
		gearByColour[holes[0]] = gear
		if _, e = f.s.equipmentFormationOn(f.ctx, id, &pb.CSFormationOn{Id: gear.EquipId, Guid: gear.Guid, FormatPos: proto.Int32(1)}); e != nil {
			t.Fatal(e)
		}
	}
	if len(gearByColour) != 4 {
		t.Fatal("full four original ordinary hole colours")
	}
	now := LocalPlayNow(f.ctx)
	if err = f.s.seedP5GrowthResourceSupply(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	global := constUtil.P5GrowthResourceSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	if err = db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", global).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitExchange", "91400000-0000-4000-8000-000000000002"); err != nil {
		t.Fatal(err)
	}
	now = LocalPlayNow(f.ctx)
	key, err := localPlayExchangeKey(db.GetIns().WithContext(f.ctx), now)
	if err != nil {
		t.Fatal(err)
	}
	_, bindings, err := f.s.p4ExchangeDirectory(db.GetIns().WithContext(f.ctx), key, now)
	if err != nil {
		t.Fatal(err)
	}
	var index int32
	for _, b := range bindings {
		if strings.HasPrefix(b.ChildID, constUtil.P5GrowthResourceSupplyPrefix) && b.ProductID == "MineItem_007" {
			index = b.Index
		}
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	var quote *pb.ActivityShopItem
	for _, item := range listing.GetShopItemList() {
		if item.GetIndex() == index {
			quote = item
		}
	}
	if quote == nil {
		t.Fatal("personal source original ore quote missing")
	}
	p4Trade(t, f, &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: quote.Index, Num: proto.Int32(20), CostResources: quote.CostResource, GetResources: quote.GetResource}, 9303, LocalPlayNow(f.ctx))
	if _, err = f.s.gemRefine(f.ctx, id, &pb.CSMineRefine{MineId: proto.String("MineItem_007"), Num: proto.Int32(100)}, 9304); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	var recipe map[string]string
	for _, r := range f.s.Tables["MineSkill"] {
		colour := int32(tableNumber(r, "Color"))
		if r["Type"] != "magicGem" || r["Level"] != "1" || gearByColour[colour] == nil {
			continue
		}
		enough := true
		for _, cost := range strings.Split(r["Material"], ";") {
			parts := strings.Split(cost, ",")
			if len(parts) != 2 {
				enough = false
				break
			}
			n, e := strconv.ParseInt(parts[1], 10, 64)
			if e != nil {
				enough = false
				break
			}
			if (parts[0] == "coin" && p.GetCoin() < n) || (parts[0] != "coin" && p5GrowthSourceOwned(g, parts[0]) < n) {
				enough = false
			}
		}
		if enough {
			recipe = r
			break
		}
	}
	if recipe == nil {
		t.Fatal("actual bounded random refinery yielded no affordable matching original recipe; do not alter RNG")
	}
	gem := recipe["MineItemID"]
	if _, err = f.s.gemMake(f.ctx, id, &pb.CSGemMake{GemId: proto.String(gem), Num: proto.Int32(1)}, 9305); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	before, err := f.s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	gear := gearByColour[int32(tableNumber(recipe, "Color"))]
	mounted, err := f.s.gemMount(f.ctx, id, &pb.CSGemMount{Guid: gear.Guid, Pos: proto.Int32(0), GemId: proto.String(gem)}, 9306)
	if err != nil {
		t.Fatal(err)
	}
	if mounted[0].(*pb.SCGemMount).GetEquip().GetGems()[0] != gem {
		t.Fatal("original coloured-hole mount")
	}
	p, g = f.profile(t)
	after, err := f.s.battlePlayer(p, g)
	if err != nil {
		t.Fatal(err)
	}
	values := strings.Split(recipe["Value"], ",")
	if len(values) != 2 {
		t.Fatal("original flat gem property")
	}
	delta, err := strconv.ParseFloat(values[1], 64)
	if err != nil {
		t.Fatal(err)
	}
	beforeProp, afterProp := before.GetFormation()[0].GetProp().ProtoReflect(), after.GetFormation()[0].GetProp().ProtoReflect()
	field := beforeProp.Descriptor().Fields().ByName(protoreflect.Name(values[0]))
	if field == nil || afterProp.Get(field).Float()-beforeProp.Get(field).Float() != delta {
		t.Fatal("actual complete battle panel did not receive original gem value")
	}
	unmountReq := &pb.CSGemUnmount{Guid: gear.Guid, Pos: proto.Int32(0)}
	unmounted, err := f.s.gemUnmount(f.ctx, id, unmountReq, 9307)
	if err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	restored, err := f.s.battlePlayer(p, g)
	if err != nil || !proto.Equal(before, restored) || p5GrowthSourceOwned(g, gem) != 1 {
		t.Fatal("actual gem return/property restoration", err)
	}
	savedPlayer, savedGame := proto.Clone(p).(*pb.Player), g
	savedRaw, err := json.Marshal(savedGame)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	coldPlayer, coldGame := f.profile(t)
	coldRaw, err := json.Marshal(coldGame)
	if err != nil || !proto.Equal(savedPlayer, coldPlayer) || string(savedRaw) != string(coldRaw) {
		t.Fatal("source/equipment/gem cold state changed", err)
	}
	coldReplay, err := f.s.gemUnmount(f.ctx, id, unmountReq, 9307)
	if err != nil || !proto.Equal(unmounted[0], coldReplay[0]) {
		t.Fatal("cold latest actual unmount receipt", err)
	}
	t.Log("bounded historical star eligibility -> actual400diamond originalday1 source -> actualfour blueGUIDs -> newpersonal paidore -> genuine random recipe -> complete battle panel +", delta, " ->unmount/cold; no business fight/win or phone claim")
}
