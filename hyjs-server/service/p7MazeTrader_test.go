package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Reuse the already original-observed, prepublished local127 complete enemy.
// This is an explicit finite operational stage, not an alleged private-server floor.
func p7MazeFixture(t *testing.T) (*p1Fixture, *pb.CSRushFightStart, time.Time) {
	t.Helper()
	f, cfg, request := p7ScopedFixture(t, constUtil.P7RushTrial)
	raw, err := readTestAnalysis75("../../analysis/p6/actual-local-p7-rush-terminal-inputs-v127/000.SCRushFightStart.pb")
	if err != nil {
		t.Fatal(err)
	}
	original := &pb.SCRushFightStart{}
	if err = proto.Unmarshal(raw, original); err != nil || len(original.BattleMonsterList) != 1 {
		t.Fatal("original127 complete source", err)
	}
	cfg.Enemies, err = proto.Marshal(original.BattleMonsterList[0])
	if err != nil {
		t.Fatal(err)
	}
	cfg.Source = "Existing prepublished local127 whole original-observed strong-enemy source; private-server floor unknown; sourceStartSHA=" + p6Digest(raw)
	if err = db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	t.Cleanup(func() { db.GetIns().Model(&db.P7MazeTraderPolicy{}).Where("id=?", 1).Update("enabled", false) })
	return f, request, at
}

func p7MazePublish(t *testing.T, at time.Time, appear int32) {
	t.Helper()
	err := db.GetIns().Model(&db.P7MazeTraderPolicy{}).Where("id=?", 1).Updates(map[string]any{"enabled": true, "published_at": at.Add(-time.Second).UnixMilli(), "start_at": at.Add(-time.Second).UnixMilli(), "end_at": at.Add(time.Hour).UnixMilli(), "appear_ppm": appear}).Error
	if err != nil {
		t.Fatal(err)
	}
}

func p7MazeAcceptedEnd(t *testing.T, f *p1Fixture, request *pb.CSRushFightStart, stamp int64) (*pb.SCRushFightStart, *pb.CSRushFightEnd) {
	t.Helper()
	start, err := f.s.p7Start(f.ctx, f.id, request, stamp, false)
	if err != nil {
		t.Fatal(err)
	}
	var row db.P7RushScopedCalculation
	if err = db.GetIns().First(&row, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if err = json.Unmarshal(row.Terminal, &terminal); err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	if result != 0 {
		t.Fatalf("expected an actual loss from original-observed local127 source, got real winner %d; do not rewrite outcome", terminal.Outcome.Winner)
	}
	return start, end
}

func TestP7MazeTraderRealAcceptedFailureDiamondUseResetAndCold(t *testing.T) {
	f, request, at := p7MazeFixture(t)
	p7MazePublish(t, at, 1000000)
	// Queries and purchases before an accepted failure cannot create an offer.
	if _, err := f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTrader{}, 0, at); err == nil {
		t.Fatal("unearned merchant")
	}
	start, end := p7MazeAcceptedEnd(t, f, request, 17401)
	before, _ := f.profile(t)
	if before.GetPoint() != 0 {
		t.Fatal("defeat manufactured win reward")
	}
	info, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	flags := info[0].(*pb.SCGetRushInfo)
	if flags.GetIsMustReset() != 1 || flags.GetIsMazeTraderValid() != 1 {
		t.Fatal("original failure flags", flags)
	}
	if _, err = f.s.p7Start(f.ctx, f.id, request, 17402, false); err == nil {
		t.Fatal("failed run restarted without reset")
	}
	query, err := f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTrader{}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	product := query[0].(*pb.SCRushMazeTrader)
	if product.GetResult().GetId() != "item_001" || product.GetResult().GetNum() != 2 || product.GetOriginalPrice() != 20 || product.GetDiscountPrice() != 10 {
		t.Fatal(product)
	}
	// Closing/repricing the live policy preserves this already frozen offer.
	if err = db.GetIns().Model(&db.P7MazeTraderPolicy{}).Where("id=?", 1).Updates(map[string]any{"enabled": false, "discount_price": 999}).Error; err != nil {
		t.Fatal(err)
	}
	bought, err := f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 17403, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := bought[0].(*pb.SCRushMazeTraderBuy)
	if len(body.GetCost()) != 1 || body.Cost[0].GetId() != "diamond" || body.Cost[0].GetNum() != 10 {
		t.Fatal(body)
	}
	after, game := f.profile(t)
	if after.GetDiamond() != before.GetDiamond()-10 || p5LifeItemCount(game, "item_001") != 2 {
		t.Fatal("cost/inventory", after.GetDiamond(), game.Items)
	}
	if _, err = f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 17404, time.Now()); err == nil {
		t.Fatal("second purchase")
	}
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal("accepted End retry", err)
	}
	f.restart(t)
	repeated, err := f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 17403, time.Now())
	if err != nil || !proto.Equal(repeated[0], body) {
		t.Fatal("cold original purchase receipt", err)
	}
	// Consume the actual purchased original001 items from the unchanged starting energy.
	energyBeforeUse := after.GetEnergy()
	used, err := f.s.itemUse(f.ctx, f.id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_001"), Num: proto.Int32(2)}}, 17405)
	_ = used
	if err != nil {
		t.Fatal("original item use", err)
	}
	p, g := f.profile(t)
	if p.GetEnergy() != energyBeforeUse+20 || p5LifeItemCount(g, "item_001") != 0 {
		t.Fatal("actual001 use", p.GetEnergy(), g.Items)
	}
	if _, err = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 17406); err != nil {
		t.Fatal("original free reset after real failure", err)
	}
	var state db.P7RushState
	if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.RunID != 2 || state.FreeResetUsed != 1 {
		t.Fatal("reset", state, err)
	}
	if _, err = f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTrader{}, 0, time.Now()); err == nil {
		t.Fatal("old run merchant leaked")
	}
	var fight db.P7RushFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P7MazeTraderOffer{}, "player_id=?", f.id) != 1 || p1Count(t, &db.P7RushReceipt{}, "player_id=? AND domain=?", f.id, constUtil.P7MazeTraderReceipt) != 1 {
		t.Fatal("offer/receipt duplicated")
	}
}

func TestP7MazeTraderUnpublishedStartAndMissNeverBackfill(t *testing.T) {
	for _, variant := range []string{"late-publication", "miss"} {
		t.Run(variant, func(t *testing.T) {
			f, request, at := p7MazeFixture(t)
			if variant == "miss" {
				p7MazePublish(t, at, 0)
			}
			p7MazeAcceptedEnd(t, f, request, 17411)
			p7MazePublish(t, time.Now(), 1000000)
			if _, err := f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTrader{}, 0, time.Now()); err == nil {
				t.Fatal("late query manufactured offer")
			}
			info, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			flags := info[0].(*pb.SCGetRushInfo)
			if flags.GetIsMustReset() != 1 || flags.GetIsMazeTraderValid() != 0 {
				t.Fatal(flags)
			}
		})
	}
}
