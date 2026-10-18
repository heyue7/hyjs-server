package service

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The P11 clock seam exercises 37 past days through the existing empty-slot
// occupation and time-claim APIs. No star material or source coin is inserted.
// Live phone elapsed-time evidence is kept separate from this accelerated test.
func TestP5StarNaturalGrab37DaysHighestRefundAndCold(t *testing.T) {
	f, _, _ := p11ArenaSetup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitEquipmentMutations, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	account, err := LoginAccount(f.ctx, "natural-star-"+t.Name(), "isolated-test-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "原计时材料五星", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	var guid string
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Energy = proto.Int32(1000)
		p.Coin = proto.Int64(0)
		p.Diamond = proto.Int64(0)
		// The owned red sword is a declared finite prerequisite. Its star inputs
		// remain empty and must all come from actual occupation reward calls.
		gain, e := f.s.GrantFixedReward(p, g, "equip_037,1")
		if e == nil {
			guid = gain.GetEquips()[0].GetGuid()
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P11GrabPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "time_reward_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	initialP, initialG := f.profile(t)
	for _, key := range []string{"item_459", "item_460", "item_461", "item_462", "item_463"} {
		if simpleOwned(initialP, initialG, key) != 0 {
			t.Fatal("source material was prepared", key)
		}
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600)).AddDate(0, 0, -37)
	start := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, local.Location())
	var stamp int64 = 700000
	for day := 0; day < 37; day++ {
		now := start.AddDate(0, 0, day)
		stamp++
		occupation := p11GrabCall(t, f, f.id, &pb.CSGrab{Index: proto.Int32(1)}, stamp, now).(*pb.SCGrab)
		if occupation.GetFightId() != "" || occupation.AttPlayer != nil || occupation.GetWinPlayerId() != 0 || occupation.GetGrabData().GetPlayerId() != f.id {
			t.Fatal("empty slot fabricated combat", occupation)
		}
		for hour := 1; hour <= 3; hour++ {
			stamp++
			claim := p11GrabCall(t, f, f.id, &pb.CSGetTimeReward{Time: proto.Int32(int32(hour))}, stamp, now.Add(time.Duration(hour)*time.Hour)).(*pb.SCGetTimeReward)
			if len(claim.GetAdd().GetSimps()) != 4 {
				t.Fatal("original four-resource tier truncated", claim)
			}
		}
		stamp++
		duplicate := p11GrabCall(t, f, f.id, &pb.CSGetTimeReward{Time: proto.Int32(3)}, stamp, now.Add(3*time.Hour)).(*pb.SCGetTimeReward)
		if len(duplicate.GetAdd().GetSimps()) != 0 {
			t.Fatal("new-stamp duplicate timer reward")
		}
	}
	sourceP, sourceG := f.profile(t)
	amounts := map[string]int64{"item_459": 555, "item_460": 37, "item_461": 111, "item_462": 111, "item_463": 111}
	if sourceP.GetCoin() != 22200000 || sourceP.GetEnergy() != 630 || sourceP.GetDiamond() != 0 {
		t.Fatal("actual 37 occupations wallet/energy", sourceP)
	}
	for key, want := range amounts {
		if simpleOwned(sourceP, sourceG, key) != want {
			t.Fatal("actual natural source quantity", key, want, simpleOwned(sourceP, sourceG, key))
		}
	}
	var sessions, claims, days int64
	for _, query := range []struct {
		table any
		count *int64
	}{{&db.P11GrabSession{}, &sessions}, {&db.P11GrabRewardClaim{}, &claims}, {&db.P11GrabDaily{}, &days}} {
		if err = db.GetIns().Model(query.table).Where("player_id=?", f.id).Count(query.count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if sessions != 37 || claims != 111 || days != 37 {
		t.Fatal("natural timer receipts", sessions, claims, days)
	}
	request := &pb.CSEquipStar{Guid: proto.String(guid)}
	requestCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	var starReply proto.Message
	var lastStamp int64
	for level := int32(1); level <= 5; level++ {
		stamp++
		lastStamp = stamp
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		replies := make(chan proto.Message, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, e := f.s.equipmentStar(requestCtx, f.id, request, lastStamp)
				errs <- e
				if e == nil {
					replies <- out[0]
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(replies)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		starReply = nil
		for reply := range replies {
			if starReply == nil {
				starReply = reply
			} else if !proto.Equal(starReply, reply) {
				t.Fatal("star same-stamp divergence")
			}
		}
		if starReply.(*pb.SCEquipStar).GetEquip().GetStar() != level {
			t.Fatal("original highest star", level, starReply)
		}
	}
	spent := map[string]int64{"item_459": 550, "item_460": 36, "item_461": 20, "item_462": 46, "item_463": 54}
	grownP, grownG := f.profile(t)
	if sourceP.GetCoin()-grownP.GetCoin() != 1500000 {
		t.Fatal("five-star actual coin cost")
	}
	for key, want := range spent {
		if simpleOwned(sourceP, sourceG, key)-simpleOwned(grownP, grownG, key) != want {
			t.Fatal("five-star actual material cost", key)
		}
	}
	if _, err = f.s.equipmentStar(requestCtx, f.id, request, lastStamp+1); err == nil {
		t.Fatal("sixth star accepted")
	}
	gear, err := f.s.equipmentRecord(grownG, guid)
	if err != nil {
		t.Fatal(err)
	}
	slot := &pb.FormationItem{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Id: grownG.Warriors[0].Id, Guid: grownG.Warriors[0].Guid, Pos: proto.Int32(0)}, {Id: gear.EquipId, Guid: gear.Guid, Pos: proto.Int32(1)}}}
	beforeActor, err := f.s.battleWarrior(sourceP, sourceG.Warriors[0], sourceG, slot)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.battleWarrior(grownP, grownG.Warriors[0], grownG, slot)
	if err != nil || actor.GetProp().GetAtt() <= beforeActor.GetProp().GetAtt() {
		t.Fatal("five-star actual actor attack", err)
	}
	// Close/reopen actual storage before checking wholeWar-equivalent equipment receipts.
	f.restart(t)
	if out, e := f.s.equipmentStar(requestCtx, f.id, request, lastStamp); e != nil || !proto.Equal(out[0], starReply) {
		t.Fatal("star5 cold receipt", e)
	}
	// Original reset needs 50 diamonds. This finite fee prerequisite is separate
	// from the natural star material/coin source under verification.
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Diamond = proto.Int64(50); return nil }); err != nil {
		t.Fatal(err)
	}
	reset := &pb.CSEquipReBuild{Guid: proto.String(guid)}
	resetCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, reset)
	resetStamp := lastStamp + 2
	if _, err = f.s.p5EquipmentReset(resetCtx, f.id, reset, resetStamp); err != nil {
		t.Fatal("natural investment reset", err)
	}
	resetP, resetG := f.profile(t)
	if resetP.GetCoin() != sourceP.GetCoin() || resetP.GetDiamond() != 0 {
		t.Fatal("natural star coin refund/reset fee")
	}
	for key, want := range amounts {
		if simpleOwned(resetP, resetG, key) != want {
			t.Fatal("natural star exact material refund", key)
		}
	}
	resetGear, err := f.s.equipmentRecord(resetG, guid)
	if err != nil || resetGear.GetStar() != 0 {
		t.Fatal("same equipment baseline", err)
	}
	if _, err = f.s.equipmentStar(requestCtx, f.id, request, lastStamp); err == nil {
		t.Fatal("old star5 resurrected after reset")
	}
	beforeRaw, _ := json.Marshal(resetG)
	f.restart(t)
	if _, err = f.s.p5EquipmentReset(resetCtx, f.id, reset, resetStamp); err != nil {
		t.Fatal("reset cold exact once", err)
	}
	afterP, afterG := f.profile(t)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(resetP, afterP) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("cold reset repeated refund")
	}
	if output := os.Getenv("HYJS_P5_STAR_GRAB_REPORT"); output != "" {
		report := map[string]any{"status": "PASS_ORIGINAL_API_INNODB_PHONE_TIMER_PENDING", "sessions": sessions, "claims": claims, "days": days, "sourceCoin": sourceP.GetCoin(), "sourceMaterials": amounts, "consumedCoin": 1500000, "consumedMaterials": spent, "energyPaid": 370, "highestStar": 5, "actorAttackBefore": beforeActor.GetProp().GetAtt(), "actorAttackAfter": actor.GetProp().GetAtt(), "eightOnceEveryStar": true, "coldStarAndReset": true, "resetMaterialAndCoinConservation": true, "oldStarAfterResetRejected": true, "clockBoundary": "37 past Beijing days through existing injectable p11GrabAt clock; no live37days/HTTP/PVPwinner claim", "finitePrerequisites": "original equip037 instance, mainhero043, level100/energy1000, reset50diamond; zero sourcecoin/diamond and no five star materials insertion"}
		raw, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(output, append(raw, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
