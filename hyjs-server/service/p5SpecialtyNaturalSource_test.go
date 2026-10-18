package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This explicit source integration complements the existing all49 highest
// mechanics matrix. Original random drops remain random; no seeded success,
// target material, book, crystal or source wallet is inserted.
func TestP5SpecialtyNaturalAll16ToHighest100AndRefund(t *testing.T) {
	if os.Getenv("HYJS_P5_SPECIALTY_NATURAL") != "1" {
		t.Skip("explicit natural source integration")
	}
	f := p4Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables} {
		if e := load("../../analysis/tables"); e != nil {
			t.Fatal(e)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitSkillSpecialty, f.s.InitializeCopyDrops, f.s.InitializeDailyCopy, f.s.InitCooking, f.s.InitializeP4CopyBenefits} {
		if e := init(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	f.s.P5Ready = true
	var guid string
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(0)
		p.Diamond = proto.Int64(0)
		p.Stamina = proto.Int32(100000)
		p.WarriorBagSize = proto.Int32(10000)
		p.EquipBagSize = proto.Int32(10000)
		g.Copy.PassedCopyInfo = nil
		g.Copy.PassedChapterInfo = nil
		g.Run = nil
		for _, mission := range f.s.Tables["CopyMission"] {
			g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: proto.String(mission["ChapterId"]), MissionId: proto.String(mission["MissionId"]), Star: proto.Int32(5), ChallengeTimes: proto.Int32(0)})
		}
		gain, err := f.s.GrantFixedReward(p, g, "hero_001,1")
		if err != nil {
			return err
		}
		guid = gain.GetWars()[0].GetGuid()
		w, err := p5Warrior(g, guid)
		if err != nil {
			return err
		}
		w.Level = proto.Int32(100)
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if e := f.s.skillSpecialtyPrepare(f.ctx, f.id); e != nil {
		t.Fatal(e)
	}
	// Source prerequisites are finite prior stars/level/stamina/eligible partner.
	// Every source coin still comes through actual chapter award or sweep calls.
	var starCoin int64
	for _, chapter := range f.s.Tables["CopyChapter"] {
		for box := int32(0); box < 3; box++ {
			out, e := f.s.chapterStarReward(f.ctx, f.id, &pb.CSCopyGetStarAward{ChapterId: proto.String(chapter["ChapterId"]), StarIndex: proto.Int32(box)})
			if e != nil {
				t.Fatal(e)
			}
			for _, v := range out[0].(*pb.SCCopyGetStarAward).GetGain().GetSimps() {
				if v.GetId() == "coin" {
					starCoin += int64(v.GetNum())
				}
			}
		}
	}
	needed := map[string]int64{}
	for _, target := range []struct{ table, field, ordinal string }{{"SkillSpecialty", "levelUpCost", "level"}, {"SkillSpecialtyStrength", "strengthCost", "strengthLevel"}} {
		for _, row := range f.s.Tables[target.table] {
			if row["type"] != "1" || row[target.ordinal] == "1" {
				continue
			}
			costs, e := p3Resources(row[target.field])
			if e != nil {
				t.Fatal(e)
			}
			for _, cost := range costs {
				needed[cost.GetId()] += int64(cost.GetNum())
			}
		}
	}
	prices := map[string]int64{"item_441": 50000, "item_445": 100000, "item_449": 200000, "item_453": 400000, "item_442": 500000, "item_446": 500000, "item_450": 500000, "item_454": 500000}
	var buyCoin int64
	for key, price := range prices {
		quantity := int64(10)
		if key == "item_442" || key == "item_446" || key == "item_450" || key == "item_454" {
			quantity = 3
		}
		buyCoin += ((needed[key] + quantity - 1) / quantity) * price
	}
	ordinary := []string{"item_439", "item_440", "item_443", "item_444", "item_447", "item_448", "item_451", "item_452"}
	missions := []string{"42", "72", "102", "132"}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600)).AddDate(0, 0, -60)
	start := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, local.Location())
	var stamp int64 = 800000
	var sweeps int64
	var sweepCoin int64
	var sweepStamina int64
	var soldMaterials, saleCoin, saleCalls int64
	var firstSaleRequest *pb.CSWarriorSell
	var firstSaleReply proto.Message
	var firstSaleStamp int64
	sellSurplus := func() {
		_, g := f.profile(t)
		var ids []string
		for _, w := range g.Warriors {
			row := f.s.table("Hero", "id", w.GetId())
			if row != nil && row["heroClass"] == "99" {
				ids = append(ids, w.GetGuid())
			}
			if len(ids) == 100 {
				break
			}
		}
		if len(ids) < 50 {
			return
		}
		stamp++
		request := &pb.CSWarriorSell{Guid: ids}
		saleCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
		beforeP, beforeG := f.profile(t)
		out, e := f.s.p5WarriorRecycle(saleCtx, f.id, ids, false, stamp, request)
		if e != nil {
			t.Fatal("actual surplus experience material sale", e)
		}
		afterP, afterG := f.profile(t)
		actualCoin := afterP.GetCoin() - beforeP.GetCoin()
		if actualCoin != int64(len(ids))*1000 || len(beforeG.Warriors)-len(afterG.Warriors) != len(ids) {
			t.Fatal("original experience sell price/GUID conservation", actualCoin, len(ids))
		}
		soldMaterials += int64(len(ids))
		saleCoin += actualCoin
		saleCalls++
		if firstSaleRequest == nil {
			firstSaleRequest, firstSaleReply, firstSaleStamp = request, out[0], stamp
		}
	}
	for day := 0; day < 60; day++ {
		p, g := f.profile(t)
		ready := p.GetCoin() >= buyCoin
		for _, key := range ordinary {
			if simpleOwned(p, g, key) < needed[key] {
				ready = false
			}
		}
		if ready {
			break
		}
		at := start.AddDate(0, 0, day)
		for tier, mission := range missions {
			row := f.s.table("CopyMission", "MissionId", mission)
			limit, _ := strconv.Atoi(row["ChallengeCount"])
			for n := 0; n < limit; n++ {
				sellSurplus()
				p, g = f.profile(t)
				// Stop this tier as soon as its two actual material inputs suffice.
				// Only the final tier continues for currency when every material
				// is ready; lower tiers never repeat merely to fund later ones.
				allMaterialsReady := true
				for _, material := range ordinary {
					if simpleOwned(p, g, material) < needed[material] {
						allMaterialsReady = false
					}
				}
				if simpleOwned(p, g, ordinary[tier*2]) >= needed[ordinary[tier*2]] && simpleOwned(p, g, ordinary[tier*2+1]) >= needed[ordinary[tier*2+1]] && (!allMaterialsReady || p.GetCoin() >= buyCoin || tier != len(missions)-1) {
					break
				}
				stamp++
				req := &pb.CSCopySweep{ChapterId: proto.String(row["ChapterId"]), MissionId: proto.String(mission)}
				sourceCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
				out, e := f.s.copySweepAt(sourceCtx, f.id, req, stamp, at)
				if e != nil {
					t.Fatal("natural source sweep", day, mission, n, e)
				}
				r := out[0].(*pb.SCCopySweep)
				sweeps++
				for _, v := range r.GetGain().GetSimps() {
					if v.GetId() == "coin" {
						sweepCoin += int64(v.GetNum())
					}
				}
				for _, v := range r.GetCost() {
					if v.GetId() == "stamina" {
						sweepStamina += int64(v.GetNum())
					}
				}
			}
		}
	}
	sourceP, sourceG := f.profile(t)
	if sourceP.GetCoin() != starCoin+sweepCoin+saleCoin || sourceP.GetCoin() < buyCoin || int64(100000-sourceP.GetStamina()) != sweepStamina {
		t.Fatal("actual source currency/stamina", sweeps, starCoin, sweepCoin, buyCoin, sourceP)
	}
	for _, key := range ordinary {
		if simpleOwned(sourceP, sourceG, key) < needed[key] {
			t.Fatal("finite random source budget exhausted", key, sweeps, needed[key], simpleOwned(sourceP, sourceG, key))
		}
	}
	now := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	key := p4ExchangeKey(now)
	child := constUtil.P5CrystalSupplyPrefix + now.Format("200601")
	if e := db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", child).Update("enabled", true).Error; e != nil {
		t.Fatal(e)
	}
	var paid int64
	buys := map[string]int64{}
	// Each date is a past current-month eligibility day. Actual individual
	// purchase calls retain the original daily/total limits and frozen quotes.
	for resource, price := range prices {
		quantity := int64(10)
		if resource == "item_442" || resource == "item_446" || resource == "item_450" || resource == "item_454" {
			quantity = 3
		}
		bundles := (needed[resource] + quantity - 1) / quantity
		for day := 0; bundles > 0 && day < 7; day++ {
			at := now.AddDate(0, 0, -day)
			listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, at).(*pb.SCActivityShopInfo)
			var product *pb.ActivityShopItem
			for _, entry := range listing.GetShopItemList() {
				if len(entry.GetGetResource()) == 1 && entry.GetGetResource()[0].GetId() == resource {
					product = entry
				}
			}
			if product == nil {
				t.Fatal("actual original source product", resource)
			}
			amount := bundles
			if amount > int64(product.GetLeft()) {
				amount = int64(product.GetLeft())
			}
			if amount < 1 {
				continue
			}
			stamp++
			req := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: product.Index, Num: proto.Int32(int32(amount)), CostResources: product.CostResource, GetResources: product.GetResource}
			p4Trade(t, f, req, stamp, at)
			paid += amount * price
			buys[resource] += amount * quantity
			bundles -= amount
		}
		if bundles > 0 {
			t.Fatal("source existing daily limit capacity", resource, bundles)
		}
	}
	if paid != buyCoin {
		t.Fatal("frozen source total coin", paid, buyCoin)
	}
	beforeP, beforeG := f.profile(t)
	beforeWar, e := p5Warrior(beforeG, guid)
	if e != nil {
		t.Fatal(e)
	}
	slot := &pb.FormationItem{FormatPos: proto.Int32(2), BattlePos: proto.Int32(2), Things: []*pb.FormationThing{{Id: beforeWar.Id, Guid: beforeWar.Guid, Pos: proto.Int32(0)}}}
	beforeActor, e := f.s.battleWarrior(beforeP, beforeWar, beforeG, slot)
	if e != nil {
		t.Fatal(e)
	}
	level, strength := int32(1), int32(1)
	var lastRequest proto.Message
	var lastReply proto.Message
	var lastStamp int64
	var operations int64
	requestCtx := func(req proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	}
	for level < 100 {
		row, e := f.s.specialtyTable("SkillSpecialty", "level", 1, level+1)
		if e != nil {
			t.Fatal(e)
		}
		need, _ := strconv.Atoi(row["needStrengthLevel"])
		var req proto.Message = &pb.CSUpRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(1)}
		strengthening := strength < int32(need)
		if strengthening {
			req = &pb.CSStrengthRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(1)}
		}
		stamp++
		out, e := f.s.skillSpecialtyUpgrade(requestCtx(req), f.id, req, stamp)
		if e != nil {
			t.Fatal("natural highest use", level, strength, e)
		}
		operations++
		lastRequest, lastReply, lastStamp = req, out[0], stamp
		if strengthening {
			strength++
		} else {
			level++
		}
	}
	if strength != 10 || operations != 108 {
		t.Fatal("original highest natural operations", strength, operations)
	}
	finalP, finalG := f.profile(t)
	target, e := p5Warrior(finalG, guid)
	if e != nil {
		t.Fatal(e)
	}
	for resource, cost := range needed {
		if simpleOwned(beforeP, beforeG, resource)-simpleOwned(finalP, finalG, resource) != cost {
			t.Fatal("highest exact source consumption", resource, cost)
		}
	}
	actor, e := f.s.battleWarrior(finalP, target, finalG, slot)
	if e != nil || actor.GetProp().GetHp() <= beforeActor.GetProp().GetHp() {
		t.Fatal("highest actual HP", e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := f.s.skillSpecialtyUpgrade(requestCtx(lastRequest), f.id, lastRequest, lastStamp)
			if e == nil && !proto.Equal(out[0], lastReply) {
				e = fmt.Errorf("highest retry changed projection")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	f.restart(t)
	if firstSaleRequest == nil {
		t.Fatal("source integration did not consume surplus")
	}
	saleCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, firstSaleRequest)
	beforeSalePlayer, beforeSaleGame := f.profile(t)
	beforeSaleRaw, _ := json.Marshal(beforeSaleGame)
	if out, e := f.s.p5WarriorRecycle(saleCtx, f.id, firstSaleRequest.Guid, false, firstSaleStamp, firstSaleRequest); e != nil || !proto.Equal(out[0], firstSaleReply) {
		t.Fatal("source surplus sale cold receipt", e)
	}
	afterSalePlayer, afterSaleGame := f.profile(t)
	afterSaleRaw, _ := json.Marshal(afterSaleGame)
	if !proto.Equal(beforeSalePlayer, afterSalePlayer) || string(beforeSaleRaw) != string(afterSaleRaw) {
		t.Fatal("cold source sale paid twice")
	}
	if out, e := f.s.skillSpecialtyUpgrade(requestCtx(lastRequest), f.id, lastRequest, lastStamp); e != nil || !proto.Equal(out[0], lastReply) {
		t.Fatal("highest natural cold", e)
	}
	// Original star awards already produced reset diamonds, so this fee also
	// uses an actual source. No reset currency is artificially appended here.
	reset := &pb.CSWarriorReset{Guid: proto.String(guid)}
	stamp++
	if _, e := f.s.p5WarriorReset(requestCtx(reset), f.id, reset, stamp); e != nil {
		t.Fatal("natural highest refund", e)
	}
	resetP, resetG := f.profile(t)
	for resource := range needed {
		if simpleOwned(resetP, resetG, resource) != simpleOwned(beforeP, beforeG, resource) {
			t.Fatal("all16 natural material refund", resource)
		}
	}
	if resetP.GetCoin() != finalP.GetCoin() {
		t.Fatal("unpaid specialty coin manufactured refund")
	}
	if _, e := f.s.skillSpecialtyUpgrade(requestCtx(lastRequest), f.id, lastRequest, lastStamp); e == nil {
		t.Fatal("old highest after reset accepted")
	}
	beforeResetPlayer := proto.Clone(resetP).(*pb.Player)
	beforeResetRaw, _ := json.Marshal(resetG)
	f.restart(t)
	if _, e := f.s.p5WarriorReset(requestCtx(reset), f.id, reset, stamp); e != nil {
		t.Fatal("natural highest reset cold", e)
	}
	afterResetPlayer, afterResetGame := f.profile(t)
	afterResetRaw, _ := json.Marshal(afterResetGame)
	if !proto.Equal(beforeResetPlayer, afterResetPlayer) || string(beforeResetRaw) != string(afterResetRaw) {
		t.Fatal("natural highest cold reset regranted input")
	}
	if output := os.Getenv("HYJS_P5_SPECIALTY_NATURAL_REPORT"); output != "" {
		report := map[string]any{"status": "PASS_ORIGINAL_API_NATURAL16_HIGHEST_PHONE_PENDING", "originalStarBoxes": 81, "actualStarCoin": starCoin, "actualSweeps": sweeps, "actualSweepCoin": sweepCoin, "actualSweepStamina": sweepStamina, "actualSurplusExpGUIDsSold": soldMaterials, "actualOriginalSaleCoin": saleCoin, "actualSaleRequests": saleCalls, "surplusSaleColdExactOnce": true, "actualSupplyCoin": paid, "sourcePurchases": buys, "actuallyConsumedMaterials": needed, "originalHPLevel": level, "originalHPStrength": strength, "actualUpgradeOperations": operations, "battleHPBefore": beforeActor.GetProp().GetHp(), "battleHPAfter": actor.GetProp().GetHp(), "eightHighestOnceAndCold": true, "sourceSurplusPolicy": "onlyrealoriginalHero rows class99;onceat50,max100 through CSWarriorSell;no direct GUID deletion;original1000each counted separately in sourcecoin", "exactAll16Refund": true, "coldResetExactOnce": true, "oldHighestAfterResetRejected": true, "preparation": "finite originalhero001/level100,prior161missionstar5/stamina100000/bags10000;initialcoin0diamond0;no16materials or sourcecoins inserted;realchapterawards and randomoriginalsweeps. Existing named DropOdds relativeweights unknownoriginalalgorithm retained.", "clockBoundary": "existing copySweepAt up to60 pastdays;existing p4Commerce pastcurrentmonth dailyeligibility;no livecalendar/HTTP/phone claim"}
		raw, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(output, append(raw, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
